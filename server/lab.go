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
	"sync"
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

	placeMu sync.Mutex
	graphs  map[string]*PlacementGraph // by track, built on first use
	places  map[int64]*placementRun    // tests in progress, by user
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

func (l *Lab) quizDue(ctx context.Context, q querier, user int64, mem map[string]Memory) bool {
	var since int
	_ = q.QueryRowContext(ctx, `SELECT since_quiz FROM xp_state WHERE user_id=?`, user).Scan(&since)
	return QuizDue(since) && len(l.quizPool(mem, time.Now())) >= 10
}

func (l *Lab) quizPool(mem map[string]Memory, now time.Time) []storedQ {
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
			w := 0.1 + 1 - st.RecallNow(now)
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
			if st.Status == "learned" || !Frontier(sched, mem) {
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
	// Content can change between deploys: an attempt over removed ideas ends.
	for _, x := range qs {
		if i := l.c.Ideas[x.Code]; i == nil || x.Index >= len(i.Questions) {
			return nil, errFinished
		}
	}
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
		done, pass, perfect = LessonResult(q.Partial, bools)
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
	before := make(map[string]Memory, len(mem))
	for k, v := range mem {
		before[k] = v
	}
	counts := map[string][2]int{}
	for i, a := range as {
		c := counts[qs[i].Code]
		c[1]++
		if a.Correct {
			c[0]++
		}
		counts[qs[i].Code] = c
	}
	gains := map[string]float64{}
	addGains := func(g map[string]float64) {
		for k, v := range g {
			gains[k] += v
		}
	}
	explicit := map[string]bool{}
	codes := make([]string, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		c := counts[code]
		explicit[code] = true
		st, ok := mem[code]
		if !ok {
			st = Memory{Status: "new"}
		}
		if kind == "lesson" {
			if pass {
				g := Good
				if !perfect {
					g = Hard
				}
				mem[code] = st.Learn(g, now)
				addGains(Propagate(l.c.Graph, l.c.Up, mem, code, true, now))
			} else {
				st.Status = "partial"
				st.Reps = math.Max(st.Reps, 0)
				mem[code] = st
				Propagate(l.c.Graph, l.c.Up, mem, code, false, now)
				KeyReviews(l.c.Graph[code], mem, now)
			}
			continue
		}
		g := RatingFrom(c[0], c[1])
		mem[code] = st.Review(g, now)
		addGains(Propagate(l.c.Graph, l.c.Up, mem, code, g > Again, now))
	}
	// A quiz error always makes its idea due, whatever the propagation did.
	if kind == "quiz" {
		for code, c := range counts {
			if c[0] < c[1] {
				mem[code] = mem[code].DueNow(now)
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
		if g := gains[code]; !explicit[code] && g >= 0.01 && l.c.Ideas[code] != nil {
			result.Implicit = append(result.Implicit, ImplicitCredit{code, l.c.Ideas[code].Name, math.Round(math.Min(g, 1)*100) / 100})
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
	MemoryNow float64    `json:"memory_now"` // FSRS recall probability now
	Stability float64    `json:"stability"`  // FSRS stability, days
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
	Progress float64      `json:"progress"`
	XP       int          `json:"xp"`
	TodayXP  int          `json:"today_xp"`
	ToQuiz   int          `json:"xp_to_quiz"`
	Days     []Day        `json:"days"`
	Ideas    []MemoryView `json:"ideas"`
	Forecast struct {
		All Forecast `json:"all"`
	} `json:"forecast"`
}

func known(st Memory) float64 {
	switch st.Status {
	case "learned":
		return 1
	case "partial":
		return math.Max(0, math.Min(1, st.Reps))
	}
	return 0
}

func (l *Lab) coverage(track string, mem map[string]Memory) float64 {
	var w, total float64
	for _, i := range l.c.Track[track] {
		x := l.c.weight(i, track)
		w += x * known(mem[i.Code])
		total += x
	}
	if total == 0 {
		return 0
	}
	return math.Max(0, math.Min(1, w/total))
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
	program := l.c.Track[track]
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
		me.Ideas = append(me.Ideas, MemoryView{i.Code, st.Status, math.Round(st.Reps*100) / 100, math.Round(st.RecallNow(now)*1000) / 1000, math.Round(st.S*10) / 10, due})
		if st.Status == "learned" {
			me.Learned++
		}
		if st.Due(now) {
			me.Due++
		}
	}
	me.Progress = l.coverage(track, mem)
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
	f := Estimate(sched, mem, rate, pace, now)
	me.Forecast.All = Forecast{math.Round(f.RemainingXP), f.Days, f.ETADate}
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
	// Goal is set on the pinned task: the idea the learner chose to study
	// next, and how many lessons are left on the way to it (itself included).
	Goal     string `json:"goal,omitempty"`
	GoalCode string `json:"goal_code,omitempty"`
	GoalLeft int    `json:"goal_left,omitempty"`
	GoalKind string `json:"goal_kind,omitempty"`
}

// Pin is the learner's chosen next step: one idea (a lesson) or a whole topic.
type Pin struct {
	Code  string `json:"code"`
	Kind  string `json:"kind"` // idea | topic
	Title string `json:"title"`
	Left  int    `json:"left"`
	Next  string `json:"next"`
}

// goalCodes resolves a pin to the ideas it asks for.
func (l *Lab) goalCodes(code string) (kind, title string, codes []string) {
	if i := l.c.Ideas[code]; i != nil {
		return "idea", i.Name, []string{code}
	}
	if t := l.c.Topics[code]; t != nil {
		return "topic", t.Name, t.Ideas
	}
	return "", "", nil
}

// goalStep finds the next lesson on the way to a goal: every not yet learned
// prerequisite (transitively, prerequisite edges only) plus the goal itself;
// of those, the earliest one in program order whose own prerequisites are met.
func (l *Lab) goalStep(goals []string, mem map[string]Memory) (*Idea, int) {
	need := map[string]bool{}
	var walk func(code string)
	walk = func(code string) {
		if need[code] || mem[code].Status == "learned" || l.c.Ideas[code] == nil {
			return
		}
		need[code] = true
		for _, e := range l.c.Ideas[code].Edges {
			if e.Kind == "prerequisite" {
				walk(e.Code)
			}
		}
	}
	for _, g := range goals {
		walk(g)
	}
	var best *Idea
	for code := range need {
		i := l.c.Ideas[code]
		ok := len(i.Questions) > 0
		for _, e := range i.Edges {
			if e.Kind == "prerequisite" && need[e.Code] {
				ok = false
			}
		}
		if !ok {
			continue
		}
		if best == nil || i.Program > best.Program || (i.Program == best.Program && i.order < best.order) {
			best = i
		}
	}
	return best, len(need)
}

func (l *Lab) GetPin(ctx context.Context, user int64) (*Pin, error) {
	var code string
	if err := l.st.db.QueryRowContext(ctx, `SELECT code FROM pins WHERE user_id=?`, user).Scan(&code); err != nil {
		return nil, nil
	}
	kind, title, codes := l.goalCodes(code)
	if kind == "" {
		return nil, nil
	}
	mem, err := loadMemory(ctx, l.st.db, user)
	if err != nil {
		return nil, err
	}
	next, left := l.goalStep(codes, mem)
	if next == nil {
		// Reached (or unreachable): the pin has done its job.
		_, _ = l.st.db.ExecContext(ctx, `DELETE FROM pins WHERE user_id=?`, user)
		return nil, nil
	}
	return &Pin{code, kind, title, left, next.Code}, nil
}

func (l *Lab) SetPin(ctx context.Context, user int64, code string) error {
	if code == "" {
		_, err := l.st.db.ExecContext(ctx, `DELETE FROM pins WHERE user_id=?`, user)
		return err
	}
	if kind, _, _ := l.goalCodes(code); kind == "" {
		return errNotFound
	}
	_, err := l.st.db.ExecContext(ctx, `INSERT INTO pins VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET code=excluded.code, created_at=excluded.created_at`, user, code, time.Now().Unix())
	return err
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
	pinned := ""
	if pin, err := l.GetPin(ctx, user); err == nil && pin != nil {
		i := l.c.Ideas[pin.Next]
		out = append(out, TaskView{Kind: "lesson", Code: i.Code, Title: i.Name, Topic: l.topicName(i), Level: i.Level, XP: 3 + 3*i.Level, Goal: pin.Title, GoalCode: pin.Code, GoalLeft: pin.Left, GoalKind: pin.Kind})
		pinned = i.Code
	}
	if l.quizDue(ctx, l.st.db, user, mem) {
		out = append(out, TaskView{Kind: "quiz", Title: "Проверка знаний", XP: 15})
	}
	for _, t := range Next(l.c.schedulingIdeas(track), mem, now, 8) {
		i := l.c.ByID[t.IdeaID]
		if i.Code == pinned {
			continue
		}
		v := TaskView{Kind: t.Kind, Code: i.Code, Title: i.Name, Topic: l.topicName(i), Level: i.Level, XP: 3}
		if t.Kind == "lesson" {
			v.XP = 3 + 3*i.Level
			for _, e := range i.Edges {
				if e.Encompassing >= 0.5 && mem[e.Code].Due(now) {
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
