package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
	_ "time/tzdata"
)

var moscow, _ = time.LoadLocation("Europe/Moscow")

func dayOf(t time.Time) string { return t.In(moscow).Format("2006-01-02") }

func rnd() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return float64(binary.LittleEndian.Uint64(b[:])>>11) / (1 << 53)
}

func shuffle[T any](xs []T) {
	for i := len(xs) - 1; i > 0; i-- {
		j := int(rnd() * float64(i+1))
		xs[i], xs[j] = xs[j], xs[i]
	}
}

type Lab struct {
	c  *Content
	st *Store
}

var (
	errUnavailable = errors.New("task unavailable")
	errNotFound    = errors.New("not found")
	errFinished    = errors.New("attempt finished")
)

type storedQ struct {
	Code    string `json:"code"`
	Index   int    `json:"index"`
	Partial bool   `json:"partial,omitempty"`
}
type storedA struct {
	Choice  int  `json:"choice"`
	Correct bool `json:"correct"`
}

type PublicQuestion struct {
	Position int      `json:"position"`
	Code     string   `json:"code"`
	Title    string   `json:"title"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type AttemptView struct {
	ID       int64           `json:"id"`
	Kind     string          `json:"kind"`
	Code     string          `json:"code,omitempty"`
	Title    string          `json:"title"`
	Lesson   string          `json:"lesson,omitempty"`
	Partial  bool            `json:"partial,omitempty"`
	Total    int             `json:"total"`
	Question *PublicQuestion `json:"question"`
}

type AttemptResult struct {
	Correct int  `json:"correct"`
	Total   int  `json:"total"`
	XP      int  `json:"xp"`
	Passed  bool `json:"passed"`
	Perfect bool `json:"perfect"`
	// Implicit is how many other ideas got repetition credit through FIRe.
	Implicit []ImplicitCredit `json:"implicit"`
}

type ImplicitCredit struct {
	Code  string  `json:"code"`
	Title string  `json:"title"`
	Delta float64 `json:"delta"`
}

type AnswerView struct {
	Correct      bool            `json:"correct"`
	CorrectIndex int             `json:"correct_index"`
	Explanation  string          `json:"explanation,omitempty"`
	Done         bool            `json:"done"`
	Next         *PublicQuestion `json:"next,omitempty"`
	Result       *AttemptResult  `json:"result,omitempty"`
}

func (l *Lab) public(q storedQ, pos int) *PublicQuestion {
	i := l.c.Ideas[q.Code]
	x := i.Questions[q.Index]
	return &PublicQuestion{pos, i.Code, i.Name, x.Question, x.Options}
}

func (l *Lab) quizDue(ctx context.Context, q querier, user int64, mem map[string]FIREMemory) bool {
	var since int
	_ = q.QueryRowContext(ctx, `SELECT since_quiz FROM xp_state WHERE user_id=?`, user).Scan(&since)
	return FIREQuizDue(since) && len(l.quizPool(mem, time.Now())) >= 10
}

func (l *Lab) quizPool(mem map[string]FIREMemory, now time.Time) []storedQ {
	type cand struct {
		q storedQ
		p float64
	}
	var pool []cand
	for code, st := range mem {
		if st.Status != "learned" {
			continue
		}
		idea := l.c.Ideas[code]
		if idea == nil {
			continue
		}
		for j := range idea.Questions {
			w := 0.1 + 1 - FIREMemoryNow(st, now)
			pool = append(pool, cand{storedQ{Code: code, Index: j}, -math.Log(math.Max(rnd(), 1e-12)) / w})
		}
	}
	sort.Slice(pool, func(a, b int) bool { return pool[a].p < pool[b].p })
	seen := map[string]int{}
	out := []storedQ{}
	for _, c := range pool {
		if seen[c.q.Code] >= 2 || len(out) >= 10 {
			continue
		}
		seen[c.q.Code]++
		out = append(out, c.q)
	}
	return out
}

func (l *Lab) Start(ctx context.Context, user int64, kind, code string) (*AttemptView, error) {
	tx, err := l.st.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	mem, err := loadMemory(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var qs []storedQ
	view := &AttemptView{Kind: kind}
	switch kind {
	case "quiz":
		if !l.quizDue(ctx, tx, user, mem) {
			return nil, errUnavailable
		}
		qs = l.quizPool(mem, now)
		view.Title = "Проверка знаний"
		code = ""
	case "lesson", "review":
		idea := l.c.Ideas[code]
		if idea == nil || len(idea.Questions) == 0 {
			return nil, errNotFound
		}
		st := mem[code]
		if kind == "lesson" {
			sched := FIREIdea{}
			for _, e := range idea.Edges {
				if e.Kind == "prerequisite" {
					sched.Prerequisites = append(sched.Prerequisites, FIREEdge{Code: e.Code, Encompassing: e.Encompassing})
				}
			}
			if st.Status == "learned" || !FIREFrontier(sched, mem) {
				return nil, errUnavailable
			}
			view.Lesson, view.Partial = idea.Lesson, st.Status == "partial"
		} else if st.Status != "learned" {
			return nil, errUnavailable
		}
		idx := make([]int, len(idea.Questions))
		for i := range idx {
			idx[i] = i
		}
		shuffle(idx)
		if kind == "review" && len(idx) > 2 {
			idx = idx[:2]
		}
		for _, i := range idx {
			qs = append(qs, storedQ{Code: code, Index: i, Partial: view.Partial})
		}
		view.Code, view.Title = code, idea.Name
	default:
		return nil, errUnavailable
	}
	raw, _ := json.Marshal(qs)
	res, err := tx.ExecContext(ctx, `INSERT INTO attempts(user_id, kind, code, started_at, questions) VALUES(?,?,NULLIF(?,''),?,?)`, user, kind, code, now.Unix(), string(raw))
	if err != nil {
		return nil, err
	}
	view.ID, _ = res.LastInsertId()
	view.Total = len(qs)
	view.Question = l.public(qs[0], 0)
	return view, tx.Commit()
}

func (l *Lab) Answer(ctx context.Context, user, id int64, choice int) (*AnswerView, error) {
	tx, err := l.st.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var kind, qsRaw, asRaw string
	var finished sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT kind, questions, answers, finished_at FROM attempts WHERE id=? AND user_id=?`, id, user).Scan(&kind, &qsRaw, &asRaw, &finished); err != nil {
		return nil, errNotFound
	}
	if finished.Valid {
		return nil, errFinished
	}
	var qs []storedQ
	var as []storedA
	_ = json.Unmarshal([]byte(qsRaw), &qs)
	_ = json.Unmarshal([]byte(asRaw), &as)
	q := qs[len(as)]
	full := l.c.Ideas[q.Code].Questions[q.Index]
	if choice < 0 || choice >= len(full.Options) {
		return nil, errUnavailable
	}
	as = append(as, storedA{choice, choice == full.Correct})
	out := &AnswerView{Correct: choice == full.Correct, CorrectIndex: full.Correct, Explanation: full.Explanation}

	bools := make([]bool, len(as))
	correct := 0
	for i, a := range as {
		bools[i] = a.Correct
		if a.Correct {
			correct++
		}
	}
	done, pass, perfect := false, false, false
	switch kind {
	case "lesson":
		done, pass, perfect = FIRELessonResult(q.Partial, bools)
		if !done && len(as) == len(qs) {
			done, pass = true, correct > len(as)-correct
		}
	case "review":
		done = len(as) == len(qs)
		pass, perfect = correct == len(as), correct == len(as)
	case "quiz":
		done = len(as) == len(qs)
		pass, perfect = correct >= 7, correct == len(as)
	}
	raw, _ := json.Marshal(as)
	if !done {
		out.Next = l.public(qs[len(as)], len(as))
		_, err = tx.ExecContext(ctx, `UPDATE attempts SET answers=?, correct=? WHERE id=?`, string(raw), correct, id)
		if err != nil {
			return nil, err
		}
		return out, tx.Commit()
	}
	now := time.Now().UTC()
	mem, err := loadMemory(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	before := make(map[string]FIREMemory, len(mem))
	for k, v := range mem {
		before[k] = v
	}
	counts := map[string][2]int{}
	for i, a := range as {
		code := qs[i].Code
		st, ok := mem[code]
		if !ok {
			st = FIREMemory{Status: "new", Speed: 1, Accuracy: 0.7}
		}
		mem[code] = FIREAccuracy(st, a.Correct, l.c.Ideas[code].Level)
		c := counts[code]
		c[1]++
		if a.Correct {
			c[0]++
		}
		counts[code] = c
	}
	explicit := map[string]bool{}
	for code, c := range counts {
		explicit[code] = true
		st := mem[code]
		if kind == "lesson" {
			if pass {
				mem[code] = FIRECompleteLesson(st, now)
				FIREPropagate(l.c.Graph, mem, code, 1, now)
			} else {
				st.Status = "partial"
				mem[code] = st
				FIREPropagate(l.c.Graph, mem, code, -1, now)
				FIREKeyReviews(l.c.Graph[code], mem, now)
			}
			continue
		}
		score := -1.0
		if c[0] == c[1] {
			score = 1
		} else if c[0] > 0 {
			score = 0.5
		}
		mem[code] = FIREReview(st, score, now)
		FIREPropagate(l.c.Graph, mem, code, score, now)
	}
	if kind == "quiz" {
		for code, c := range counts {
			if c[0] < c[1] {
				st := mem[code]
				st.Memory, st.LastReviewAt, st.DueAt = 0.4, now, now
				mem[code] = st
			}
		}
	}
	result := &AttemptResult{Correct: correct, Total: len(as), Passed: pass, Perfect: perfect, Implicit: []ImplicitCredit{}}
	for code, st := range mem {
		old, existed := before[code]
		if existed && old == st {
			continue
		}
		if err := saveMemory(ctx, tx, user, code, st); err != nil {
			return nil, err
		}
		if !explicit[code] && existed && st.RepNum-old.RepNum >= 0.01 {
			result.Implicit = append(result.Implicit, ImplicitCredit{code, l.c.Ideas[code].Name, math.Round((st.RepNum-old.RepNum)*100) / 100})
		}
	}
	sort.Slice(result.Implicit, func(a, b int) bool { return result.Implicit[a].Delta > result.Implicit[b].Delta })
	base := 3
	if kind == "quiz" {
		base = 15
	} else if kind == "lesson" {
		base = 3 + 3*l.c.Ideas[qs[0].Code].Level
	}
	switch {
	case perfect:
		result.XP = int(math.Round(float64(base) * 1.2))
	case pass:
		result.XP = base
	case kind == "review" && correct > 0:
		result.XP = int(math.Round(float64(base) * 0.5))
	}
	if kind == "quiz" {
		_, err = tx.ExecContext(ctx, `INSERT INTO xp_state VALUES(?,0) ON CONFLICT(user_id) DO UPDATE SET since_quiz=0`, user)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO xp_state VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET since_quiz=since_quiz+excluded.since_quiz`, user, result.XP)
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE attempts SET answers=?, correct=?, xp=?, passed=?, finished_at=?, implicit=? WHERE id=?`, string(raw), correct, result.XP, pass, now.Unix(), len(result.Implicit), id)
	if err != nil {
		return nil, err
	}
	out.Done, out.Result = true, result
	return out, tx.Commit()
}

// ---- progress ----

type MemoryView struct {
	Code      string     `json:"code"`
	Status    string     `json:"status"`
	Rep       float64    `json:"rep"`
	MemoryNow float64    `json:"memory_now"`
	DueAt     *time.Time `json:"due_at"`
}

type Day struct {
	Date string `json:"date"`
	XP   int    `json:"xp"`
}

type Forecast struct {
	RemainingXP float64   `json:"remaining_xp"`
	Days        float64   `json:"days"`
	ETA         time.Time `json:"eta"`
}

type Me struct {
	User     string       `json:"user"`
	Track    string       `json:"track"`
	Total    int          `json:"total"`
	Learned  int          `json:"learned"`
	Due      int          `json:"due"`
	Coverage float64      `json:"coverage"`
	Offers   float64      `json:"offers"`
	AnyOffer float64      `json:"any_offer"`
	XP       int          `json:"xp"`
	TodayXP  int          `json:"today_xp"`
	ToQuiz   int          `json:"xp_to_quiz"`
	Days     []Day        `json:"days"`
	Ideas    []MemoryView `json:"ideas"`
	Forecast struct {
		All   Forecast  `json:"all"`
		Offer *Forecast `json:"offer,omitempty"`
	} `json:"forecast"`
}

func known(st FIREMemory) float64 {
	switch st.Status {
	case "learned":
		return 1
	case "partial":
		return math.Max(0, math.Min(1, st.RepNum))
	}
	return 0
}

func (l *Lab) coverage(track string, mem map[string]FIREMemory) float64 {
	var w, total float64
	for _, i := range l.c.Program[programOf(track)] {
		x := l.c.weight(i, track)
		w += x * known(mem[i.Code])
		total += x
	}
	if total == 0 {
		return 0
	}
	unseen := 0.0
	if t, ok := l.c.Market.Tracks[track]; ok {
		unseen = t.Unseen
	}
	return math.Max(0, math.Min(1, w/total*(1-unseen)))
}

func curveAt(curve []float64, coverage float64) float64 {
	if len(curve) == 0 {
		return 0
	}
	x := math.Max(0, math.Min(100, coverage*100))
	lo := int(math.Floor(x))
	hi := min(len(curve)-1, lo+1)
	lo = min(lo, len(curve)-1)
	return curve[lo] + (curve[hi]-curve[lo])*(x-float64(lo))
}

func (l *Lab) days(ctx context.Context, user int64, n int) ([]Day, error) {
	since := time.Now().Add(-time.Duration(n+1) * 24 * time.Hour).Unix()
	rows, err := l.st.db.QueryContext(ctx, `SELECT finished_at, xp FROM attempts WHERE user_id=? AND finished_at>=?`, user, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sum := map[string]int{}
	for rows.Next() {
		var at int64
		var xp int
		if err := rows.Scan(&at, &xp); err != nil {
			return nil, err
		}
		sum[dayOf(time.Unix(at, 0))] += xp
	}
	out := make([]Day, 0, n)
	for i := n - 1; i >= 0; i-- {
		d := dayOf(time.Now().Add(-time.Duration(i) * 24 * time.Hour))
		out = append(out, Day{d, sum[d]})
	}
	return out, nil
}

func (l *Lab) Me(ctx context.Context, user int64, name, track string) (*Me, error) {
	mem, err := loadMemory(ctx, l.st.db, user)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	me := &Me{User: name, Track: track, Ideas: []MemoryView{}}
	program := l.c.Program[programOf(track)]
	sched := l.c.schedulingIdeas(track)
	me.Total = len(sched)
	for _, i := range program {
		st, ok := mem[i.Code]
		if !ok {
			continue
		}
		var due *time.Time
		if !st.DueAt.IsZero() && st.Status == "learned" {
			d := st.DueAt
			due = &d
		}
		me.Ideas = append(me.Ideas, MemoryView{i.Code, st.Status, math.Round(st.RepNum*100) / 100, math.Round(FIREMemoryNow(st, now)*1000) / 1000, due})
		if st.Status == "learned" {
			me.Learned++
		}
		if FIREDue(st, now) {
			me.Due++
		}
	}
	me.Coverage = l.coverage(track, mem)
	if t, ok := l.c.Market.Tracks[track]; ok {
		me.Offers = t.Interviews * curveAt(t.Curve.PerInterview, me.Coverage)
		me.AnyOffer = curveAt(t.Curve.Job, me.Coverage)
	}
	_ = l.st.db.QueryRowContext(ctx, `SELECT coalesce(sum(xp),0) FROM attempts WHERE user_id=?`, user).Scan(&me.XP)
	var since int
	_ = l.st.db.QueryRowContext(ctx, `SELECT since_quiz FROM xp_state WHERE user_id=?`, user).Scan(&since)
	me.ToQuiz = max(0, 150-since)
	if me.Days, err = l.days(ctx, user, 120); err != nil {
		return nil, err
	}
	me.TodayXP = me.Days[len(me.Days)-1].XP
	pace := 0.0
	for _, d := range me.Days[len(me.Days)-14:] {
		pace += float64(d.XP)
	}
	pace /= 14
	var passed, total float64
	_ = l.st.db.QueryRowContext(ctx, `SELECT coalesce(sum(passed),0), count(*) FROM (SELECT passed FROM attempts WHERE user_id=? AND finished_at IS NOT NULL ORDER BY id DESC LIMIT 50)`, user).Scan(&passed, &total)
	rate := 0.85
	if total >= 5 {
		rate = math.Max(0.3, passed/total)
	}
	f := FIREEstimate(sched, mem, rate, pace, 0.75, now)
	me.Forecast.All = Forecast{math.Round(f.RemainingXP), f.Days, f.ETADate}
	if t, ok := l.c.Market.Tracks[track]; ok && t.Thresholds.O1 != nil {
		n := max(0, min(*t.Thresholds.O1, len(t.Order)))
		by := map[string]FIREIdea{}
		for _, s := range sched {
			by[s.Code] = s
		}
		subset := []FIREIdea{}
		for _, o := range t.Order[:n] {
			if s, ok := by[o.Code]; ok {
				subset = append(subset, s)
			}
		}
		o := FIREEstimate(subset, mem, rate, pace, 0.75, now)
		me.Forecast.Offer = &Forecast{math.Round(o.RemainingXP), o.Days, o.ETADate}
	}
	return me, nil
}

type TaskView struct {
	Kind  string     `json:"kind"`
	Code  string     `json:"code,omitempty"`
	Title string     `json:"title"`
	Topic string     `json:"topic,omitempty"`
	Level int        `json:"level,omitempty"`
	XP    int        `json:"xp"`
	DueAt *time.Time `json:"due_at,omitempty"`
	// Covers lists due reviews this lesson repeats implicitly.
	Covers []string `json:"covers,omitempty"`
}

func (l *Lab) topicName(i *Idea) string {
	for _, s := range l.c.Sections[i.Program] {
		for _, t := range s.Topics {
			if t.ID == i.TopicID {
				return t.Name
			}
		}
	}
	return ""
}

func (l *Lab) Next(ctx context.Context, user int64, track string) ([]TaskView, error) {
	mem, err := loadMemory(ctx, l.st.db, user)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := []TaskView{}
	if l.quizDue(ctx, l.st.db, user, mem) {
		out = append(out, TaskView{Kind: "quiz", Title: "Проверка знаний", XP: 15})
	}
	for _, t := range FIRENext(l.c.schedulingIdeas(track), mem, now, 8, false) {
		i := l.c.ByID[t.IdeaID]
		v := TaskView{Kind: t.Kind, Code: i.Code, Title: i.Name, Topic: l.topicName(i), Level: i.Level, XP: 3}
		if t.Kind == "lesson" {
			v.XP = 3 + 3*i.Level
			for _, e := range i.Edges {
				if e.Encompassing >= 0.5 && FIREDue(mem[e.Code], now) {
					v.Covers = append(v.Covers, l.c.Ideas[e.Code].Name)
				}
			}
		} else if st := mem[i.Code]; !st.DueAt.IsZero() {
			d := st.DueAt
			v.DueAt = &d
		}
		out = append(out, v)
	}
	return out, nil
}

type HistoryItem struct {
	Kind     string    `json:"kind"`
	Code     string    `json:"code,omitempty"`
	Title    string    `json:"title"`
	Finished time.Time `json:"finished_at"`
	Correct  int       `json:"correct"`
	Total    int       `json:"total"`
	XP       int       `json:"xp"`
	Passed   bool      `json:"passed"`
}

func (l *Lab) History(ctx context.Context, user int64) ([]HistoryItem, error) {
	rows, err := l.st.db.QueryContext(ctx, `SELECT kind, coalesce(code,''), finished_at, correct, answers, xp, coalesce(passed,0) FROM attempts WHERE user_id=? AND finished_at IS NOT NULL ORDER BY id DESC LIMIT 40`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryItem{}
	for rows.Next() {
		var h HistoryItem
		var at int64
		var answers string
		if err := rows.Scan(&h.Kind, &h.Code, &at, &h.Correct, &answers, &h.XP, &h.Passed); err != nil {
			return nil, err
		}
		var as []storedA
		_ = json.Unmarshal([]byte(answers), &as)
		h.Total, h.Finished = len(as), time.Unix(at, 0).UTC()
		h.Title = "Проверка знаний"
		if i := l.c.Ideas[h.Code]; i != nil {
			h.Title = i.Name
		}
		out = append(out, h)
	}
	return out, nil
}
