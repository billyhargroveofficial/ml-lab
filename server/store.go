package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Progress of the one local learner. Tables keep a user_id column (always
// localUser) so the schema stays compatible with older databases.
const localUser int64 = 1

const schema = `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS memory(user_id INTEGER NOT NULL, code TEXT NOT NULL, status TEXT NOT NULL, rep REAL NOT NULL, memory REAL NOT NULL,
  interval REAL NOT NULL, speed REAL NOT NULL, accuracy REAL NOT NULL, last_review INTEGER, due INTEGER, PRIMARY KEY(user_id, code));
CREATE TABLE IF NOT EXISTS attempts(id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, kind TEXT NOT NULL, code TEXT, started_at INTEGER NOT NULL,
  finished_at INTEGER, questions TEXT NOT NULL, answers TEXT NOT NULL DEFAULT '[]', correct INTEGER NOT NULL DEFAULT 0, xp INTEGER NOT NULL DEFAULT 0, passed INTEGER, implicit INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS attempts_user ON attempts(user_id, id);
CREATE TABLE IF NOT EXISTS xp_state(user_id INTEGER PRIMARY KEY, since_quiz INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS pins(user_id INTEGER PRIMARY KEY, code TEXT NOT NULL, created_at INTEGER NOT NULL);
`

type Store struct{ db *sql.DB }

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// One writer keeps per-user progress serialisable without row locks.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	if err := migrateFSRS(db); err != nil {
		return nil, err
	}
	return &Store{db}, nil
}

func unix(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

func fromUnix(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.Unix(v.Int64, 0).UTC()
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadMemory(ctx context.Context, q querier, user int64) (map[string]Memory, error) {
	rows, err := q.QueryContext(ctx, `SELECT code, status, rep, s, d, last_review, due, anchor_s, anchor_at FROM memory WHERE user_id=?`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Memory{}
	for rows.Next() {
		var code string
		var m Memory
		var last, due, anchorAt sql.NullInt64
		if err := rows.Scan(&code, &m.Status, &m.Reps, &m.S, &m.D, &last, &due, &m.AnchorS, &anchorAt); err != nil {
			return nil, err
		}
		m.LastReviewAt, m.DueAt, m.AnchorAt = fromUnix(last), fromUnix(due), fromUnix(anchorAt)
		if m.Status == "learned" && m.AnchorS <= 0 { // rows saved before the anchor existed
			m.AnchorS, m.AnchorAt = m.S, m.LastReviewAt
		}
		out[code] = m
	}
	return out, rows.Err()
}

func saveMemory(ctx context.Context, q querier, user int64, code string, m Memory) error {
	_, err := q.ExecContext(ctx, `INSERT INTO memory(user_id, code, status, rep, memory, interval, speed, accuracy, last_review, due, s, d, anchor_s, anchor_at) VALUES(?,?,?,?,0,0,0,0,?,?,?,?,?,?)
		ON CONFLICT(user_id, code) DO UPDATE SET status=excluded.status, rep=excluded.rep, last_review=excluded.last_review, due=excluded.due, s=excluded.s, d=excluded.d, anchor_s=excluded.anchor_s, anchor_at=excluded.anchor_at`,
		user, code, m.Status, m.Reps, unix(m.LastReviewAt), unix(m.DueAt), m.S, m.D, m.AnchorS, unix(m.AnchorAt))
	return err
}

// migrateFSRS adds FSRS stability/difficulty to the memory table and converts
// progress kept by the old interval model: a learned idea keeps its last
// review and gets stability equal to its old interval (the old half-life),
// with neutral difficulty 5.
func migrateFSRS(db *sql.DB) error {
	has := map[string]bool{}
	rows, err := db.Query(`SELECT name FROM pragma_table_info('memory')`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		has[n] = true
	}
	rows.Close()
	qs := []string{}
	if !has["s"] {
		qs = append(qs,
			`ALTER TABLE memory ADD COLUMN s REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE memory ADD COLUMN d REAL NOT NULL DEFAULT 5`,
			`UPDATE memory SET s = max(interval, 1), d = 5 WHERE status='learned'`,
			`UPDATE memory SET due = last_review + CAST(round(max(interval, 1) * 86400) AS INTEGER) WHERE status='learned' AND last_review IS NOT NULL`)
	}
	if !has["anchor_s"] { // the last explicit review; filled from s on load
		qs = append(qs,
			`ALTER TABLE memory ADD COLUMN anchor_s REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE memory ADD COLUMN anchor_at INTEGER`)
	}
	for _, q := range qs {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// ProgressFormat names the export file format; keep it backward compatible.
const ProgressFormat = "fire-progress-v1"

var progressTables = []string{"memory", "attempts", "xp_state", "pins"}

// Export writes the whole local progress as JSON: rows of every table.
func (s *Store) Export(ctx context.Context, w io.Writer) error {
	out := map[string]any{"format": ProgressFormat, "exported_at": time.Now().UTC().Format(time.RFC3339)}
	for _, t := range progressTables {
		rows, err := s.db.QueryContext(ctx, "SELECT * FROM "+t+" WHERE user_id=?", localUser)
		if err != nil {
			return err
		}
		cols, _ := rows.Columns()
		list := []map[string]any{}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			row := map[string]any{}
			for i, c := range cols {
				if b, ok := vals[i].([]byte); ok {
					vals[i] = string(b)
				}
				row[c] = vals[i]
			}
			list = append(list, row)
		}
		rows.Close()
		out[t] = list
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(out)
}

// Import replaces the local progress with an export, in one transaction.
func (s *Store) Import(ctx context.Context, r io.Reader) error {
	var in map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return fmt.Errorf("not JSON: %w", err)
	}
	var format string
	_ = json.Unmarshal(in["format"], &format)
	if format != ProgressFormat {
		return fmt.Errorf("format %q, want %q", format, ProgressFormat)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range progressTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
		var rows []map[string]any
		if raw, ok := in[t]; ok {
			if err := json.Unmarshal(raw, &rows); err != nil {
				return fmt.Errorf("%s: %w", t, err)
			}
		}
		for _, row := range rows {
			row["user_id"] = localUser
			cols, marks, vals := "", "", []any{}
			for k, v := range row {
				if !safeColumn(k) {
					return fmt.Errorf("%s: bad column %q", t, k)
				}
				if cols != "" {
					cols, marks = cols+",", marks+","
				}
				cols, marks = cols+k, marks+"?"
				vals = append(vals, v)
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+t+"("+cols+") VALUES("+marks+")", vals...); err != nil {
				return fmt.Errorf("%s: %w", t, err)
			}
		}
	}
	return tx.Commit()
}

func safeColumn(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}

// Reset forgets all progress.
func (s *Store) Reset(ctx context.Context) error {
	for _, t := range progressTables {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
	}
	return nil
}
