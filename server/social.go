package main

import (
	"context"
	"sort"
	"time"
)

type Leader struct {
	Rank        int    `json:"rank"`
	Name        string `json:"name"`
	XP          int    `json:"xp"`
	WeekXP      int    `json:"week_xp"`
	LearnedML   int    `json:"learned_ml"`
	LearnedMath int    `json:"learned_math"`
	Me          bool   `json:"me,omitempty"`
}

// Leaders ranks by XP for the period; it reads only nicknames people chose.
func (l *Lab) Leaders(ctx context.Context, viewer int64, period string) ([]Leader, error) {
	week := time.Now().Add(-7 * 24 * time.Hour).Unix()
	order := "xp"
	if period == "week" {
		order = "week_xp"
	}
	rows, err := l.st.db.QueryContext(ctx, `SELECT u.id, u.name, coalesce(sum(a.xp),0) xp, coalesce(sum(CASE WHEN a.finished_at>=? THEN a.xp END),0) week_xp
		FROM users u JOIN attempts a ON a.user_id=u.id AND a.finished_at IS NOT NULL GROUP BY u.id HAVING xp>0 ORDER BY `+order+` DESC, u.id LIMIT 200`, week)
	if err != nil {
		return nil, err
	}
	type row struct {
		id int64
		Leader
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.Name, &r.XP, &r.WeekXP); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	out := []Leader{}
	for i, r := range all {
		if i >= 15 && r.id != viewer {
			continue
		}
		mem, err := loadMemory(ctx, l.st.db, r.id)
		if err != nil {
			return nil, err
		}
		for code, st := range mem {
			if st.Status != "learned" {
				continue
			}
			switch i := l.c.Ideas[code]; {
			case i == nil:
			case i.Program == "math":
				r.LearnedMath++
			default:
				r.LearnedML++
			}
		}
		r.Rank, r.Me = i+1, r.id == viewer
		out = append(out, r.Leader)
	}
	return out, nil
}

// ---- anonymized public statistics ----

type Stats struct {
	GeneratedAt time.Time `json:"generated_at"`
	Users       int       `json:"users"`
	Active7     int       `json:"active_7d"`
	Active30    int       `json:"active_30d"`
	Attempts    int       `json:"attempts"`
	LessonsDone int       `json:"lessons_passed"`
	XP          int       `json:"xp"`
	// Daily activity over the last 60 days (Moscow time).
	Daily []struct {
		Date     string `json:"date"`
		Learners int    `json:"learners"`
		Attempts int    `json:"attempts"`
		XP       int    `json:"xp"`
	} `json:"daily"`
	// LearnedHistogram: how many learners have learned N ideas, in buckets.
	LearnedHistogram []Bucket `json:"learned_histogram"`
	// Sections: first-try pass rate per section, only with ≥ minSample attempts.
	Sections []SectionStat `json:"sections"`
	Hardest  []IdeaStat    `json:"hardest_ideas"`
	Implicit struct {
		ExplicitReviews int `json:"explicit_reviews"`
		ImplicitCredits int `json:"implicit_credits"`
	} `json:"implicit"`
	MinSample int `json:"min_sample"`
}

type Bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type SectionStat struct {
	Program  string  `json:"program"`
	Name     string  `json:"name"`
	Attempts int     `json:"attempts"`
	PassRate float64 `json:"pass_rate"`
}

type IdeaStat struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Attempts int     `json:"attempts"`
	PassRate float64 `json:"pass_rate"`
}

const minSample = 5

func (l *Lab) Stats(ctx context.Context) (*Stats, error) {
	s := &Stats{GeneratedAt: time.Now().UTC(), MinSample: minSample, LearnedHistogram: []Bucket{}, Sections: []SectionStat{}, Hardest: []IdeaStat{}}
	db := l.st.db
	now := time.Now()
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&s.Users)
	_ = db.QueryRowContext(ctx, `SELECT count(DISTINCT user_id) FROM attempts WHERE finished_at>=?`, now.Add(-7*24*time.Hour).Unix()).Scan(&s.Active7)
	_ = db.QueryRowContext(ctx, `SELECT count(DISTINCT user_id) FROM attempts WHERE finished_at>=?`, now.Add(-30*24*time.Hour).Unix()).Scan(&s.Active30)
	_ = db.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(xp),0), coalesce(sum(kind='lesson' AND passed=1),0) FROM attempts WHERE finished_at IS NOT NULL`).Scan(&s.Attempts, &s.XP, &s.LessonsDone)

	type agg struct {
		users    map[int64]bool
		attempts int
		xp       int
	}
	daily := map[string]*agg{}
	rows, err := db.QueryContext(ctx, `SELECT user_id, finished_at, xp, kind, coalesce(code,''), coalesce(passed,0) FROM attempts WHERE finished_at IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	type pr struct{ n, ok int }
	firstLesson := map[[2]any]bool{}
	byIdea := map[string]*pr{}
	for rows.Next() {
		var user, at int64
		var xp int
		var kind, code string
		var passed bool
		if err := rows.Scan(&user, &at, &xp, &kind, &code, &passed); err != nil {
			rows.Close()
			return nil, err
		}
		d := dayOf(time.Unix(at, 0))
		a := daily[d]
		if a == nil {
			a = &agg{users: map[int64]bool{}}
			daily[d] = a
		}
		a.users[user], a.attempts, a.xp = true, a.attempts+1, a.xp+xp
		if kind == "review" {
			s.Implicit.ExplicitReviews++
		}
		if kind == "lesson" && !firstLesson[[2]any{user, code}] {
			firstLesson[[2]any{user, code}] = true
			p := byIdea[code]
			if p == nil {
				p = &pr{}
				byIdea[code] = p
			}
			p.n++
			if passed {
				p.ok++
			}
		}
	}
	rows.Close()
	for i := 59; i >= 0; i-- {
		d := dayOf(now.Add(-time.Duration(i) * 24 * time.Hour))
		row := struct {
			Date     string `json:"date"`
			Learners int    `json:"learners"`
			Attempts int    `json:"attempts"`
			XP       int    `json:"xp"`
		}{Date: d}
		if a := daily[d]; a != nil {
			row.Learners, row.Attempts, row.XP = len(a.users), a.attempts, a.xp
		}
		s.Daily = append(s.Daily, row)
	}
	sections := map[string]*pr{}
	for code, p := range byIdea {
		i := l.c.Ideas[code]
		if i == nil {
			continue
		}
		var sec string
		for _, x := range l.c.Sections[i.Program] {
			for _, t := range x.Topics {
				if t.ID == i.TopicID {
					sec = i.Program + "\x00" + x.Name
				}
			}
		}
		q := sections[sec]
		if q == nil {
			q = &pr{}
			sections[sec] = q
		}
		q.n += p.n
		q.ok += p.ok
		if p.n >= minSample {
			s.Hardest = append(s.Hardest, IdeaStat{code, i.Name, p.n, float64(p.ok) / float64(p.n)})
		}
	}
	for key, p := range sections {
		if p.n < minSample {
			continue
		}
		var prog, name string
		for i := range key {
			if key[i] == 0 {
				prog, name = key[:i], key[i+1:]
			}
		}
		s.Sections = append(s.Sections, SectionStat{prog, name, p.n, float64(p.ok) / float64(p.n)})
	}
	sort.Slice(s.Sections, func(a, b int) bool { return s.Sections[a].PassRate < s.Sections[b].PassRate })
	sort.Slice(s.Hardest, func(a, b int) bool { return s.Hardest[a].PassRate < s.Hardest[b].PassRate })
	if len(s.Hardest) > 10 {
		s.Hardest = s.Hardest[:10]
	}

	// Per-learner aggregates go only into buckets; no row identifies a person.
	learnedEdges := []int{0, 1, 10, 25, 50, 100, 200, 400}
	lh := make([]int, len(learnedEdges))
	urows, err := db.QueryContext(ctx, `SELECT DISTINCT user_id FROM attempts`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for urows.Next() {
		var id int64
		_ = urows.Scan(&id)
		ids = append(ids, id)
	}
	urows.Close()
	for _, id := range ids {
		mem, err := loadMemory(ctx, db, id)
		if err != nil {
			return nil, err
		}
		n := 0
		for _, st := range mem {
			if st.Status == "learned" {
				n++
			}
		}
		for i := len(learnedEdges) - 1; i >= 0; i-- {
			if n >= learnedEdges[i] {
				lh[i]++
				break
			}
		}
	}
	for i, e := range learnedEdges {
		label := ""
		switch {
		case i == len(learnedEdges)-1:
			label = itoa(e) + "+"
		case learnedEdges[i+1]-e == 1:
			label = itoa(e)
		default:
			label = itoa(e) + "–" + itoa(learnedEdges[i+1]-1)
		}
		s.LearnedHistogram = append(s.LearnedHistogram, Bucket{label, lh[i]})
	}
	_ = db.QueryRowContext(ctx, `SELECT coalesce(sum(implicit),0) FROM attempts`).Scan(&s.Implicit.ImplicitCredits)
	return s, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
