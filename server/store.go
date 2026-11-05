package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const schema = `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE, pass TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(token TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id), expires_at INTEGER NOT NULL);
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
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// One writer keeps per-user progress serialisable without row locks.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Store{db}, nil
}

var (
	errNameTaken = errors.New("name taken")
	errBadLogin  = errors.New("bad login")
)

func hashPassword(pass string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key, _ := pbkdf2.Key(sha256.New, pass, salt, 200_000, 32)
	return "pbkdf2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}

func checkPassword(stored, pass string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[1])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil {
		return false
	}
	key, _ := pbkdf2.Key(sha256.New, pass, salt, 200_000, 32)
	return subtle.ConstantTimeCompare(key, want) == 1
}

func (s *Store) Register(ctx context.Context, name, pass string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users(name, pass, created_at) VALUES(?,?,?)`, name, hashPassword(pass), time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, errNameTaken
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Login(ctx context.Context, name, pass string) (int64, error) {
	var id int64
	var stored string
	if err := s.db.QueryRowContext(ctx, `SELECT id, pass FROM users WHERE name=?`, name).Scan(&id, &stored); err != nil {
		return 0, errBadLogin
	}
	if !checkPassword(stored, pass) {
		return 0, errBadLogin
	}
	return id, nil
}

const sessionTTL = 180 * 24 * time.Hour

func (s *Store) NewSession(ctx context.Context, user int64) (string, error) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions VALUES(?,?,?)`, hex.EncodeToString(sum[:]), user, time.Now().Add(sessionTTL).Unix())
	return token, err
}

func (s *Store) SessionUser(ctx context.Context, token string) (int64, string, bool) {
	sum := sha256.Sum256([]byte(token))
	var id int64
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT u.id, u.name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND s.expires_at>?`, hex.EncodeToString(sum[:]), time.Now().Unix()).Scan(&id, &name)
	return id, name, err == nil
}

// DeleteUser removes the account and everything tied to it.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM sessions WHERE user_id=?`, `DELETE FROM memory WHERE user_id=?`, `DELETE FROM attempts WHERE user_id=?`, `DELETE FROM xp_state WHERE user_id=?`, `DELETE FROM pins WHERE user_id=?`, `DELETE FROM users WHERE id=?`} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DropSession(ctx context.Context, token string) {
	sum := sha256.Sum256([]byte(token))
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token=?`, hex.EncodeToString(sum[:]))
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

func loadMemory(ctx context.Context, q querier, user int64) (map[string]FIREMemory, error) {
	rows, err := q.QueryContext(ctx, `SELECT code, status, rep, memory, interval, speed, accuracy, last_review, due FROM memory WHERE user_id=?`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FIREMemory{}
	for rows.Next() {
		var code string
		var m FIREMemory
		var last, due sql.NullInt64
		if err := rows.Scan(&code, &m.Status, &m.RepNum, &m.Memory, &m.IntervalDays, &m.Speed, &m.Accuracy, &last, &due); err != nil {
			return nil, err
		}
		m.LastReviewAt, m.DueAt = fromUnix(last), fromUnix(due)
		out[code] = m
	}
	return out, rows.Err()
}

func saveMemory(ctx context.Context, q querier, user int64, code string, m FIREMemory) error {
	_, err := q.ExecContext(ctx, `INSERT INTO memory VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id, code) DO UPDATE SET status=excluded.status, rep=excluded.rep,
		memory=excluded.memory, interval=excluded.interval, speed=excluded.speed, accuracy=excluded.accuracy, last_review=excluded.last_review, due=excluded.due`,
		user, code, m.Status, m.RepNum, m.Memory, m.IntervalDays, m.Speed, m.Accuracy, unix(m.LastReviewAt), unix(m.DueAt))
	return err
}
