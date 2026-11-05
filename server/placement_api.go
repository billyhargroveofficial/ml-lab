package main

import (
	"context"
	"time"
)

// The placement test over HTTP. A run lives in memory: it takes a few
// minutes, and a restart only means starting over. Nothing is written until
// the run ends, and it never earns XP.

const placementTTL = 2 * time.Hour

type placementRun struct {
	track   string
	p       *Placement
	q       storedQ
	asked   map[string]int // questions used per idea
	started time.Time
}

type PlacementView struct {
	Done     bool            `json:"done"`
	Probe    int             `json:"probe"`
	Max      int             `json:"max"`
	Question *PublicQuestion `json:"question,omitempty"`
	// After an answer:
	Correct      *bool  `json:"correct,omitempty"`
	CorrectIndex *int   `json:"correct_index,omitempty"`
	Explanation  string `json:"explanation,omitempty"`
	// At the end:
	Known int `json:"known"`
	Total int `json:"total"`
}

func (l *Lab) placementGraph(track string) *PlacementGraph {
	if l.graphs == nil {
		l.graphs = map[string]*PlacementGraph{}
	}
	g := l.graphs[track]
	if g == nil {
		g = NewPlacementGraph(l.c.schedulingIdeas(track))
		l.graphs[track] = g
	}
	return g
}

func (r *placementRun) ask(l *Lab, code string) *PublicQuestion {
	i := l.c.Ideas[code]
	n := r.asked[code]
	r.asked[code] = n + 1
	idx := (int(rnd()*float64(len(i.Questions))) + n) % len(i.Questions)
	if n > 0 && idx == r.q.Index && len(i.Questions) > 1 {
		idx = (idx + 1) % len(i.Questions)
	}
	r.q = storedQ{Code: code, Index: idx}
	return l.public(r.q, r.p.Probes+1)
}

// StartPlacement begins (or restarts) a test for a track.
func (l *Lab) StartPlacement(ctx context.Context, user int64, track string) (*PlacementView, error) {
	mem, err := loadMemory(ctx, l.st.db, user)
	if err != nil {
		return nil, err
	}
	learned := map[string]bool{}
	for code, m := range mem {
		if m.Status == "learned" {
			learned[code] = true
		}
	}
	l.placeMu.Lock()
	defer l.placeMu.Unlock()
	if l.places == nil {
		l.places = map[int64]*placementRun{}
	}
	for u, r := range l.places {
		if time.Since(r.started) > placementTTL {
			delete(l.places, u)
		}
	}
	g := l.placementGraph(track)
	r := &placementRun{track: track, p: NewPlacement(g, learned), asked: map[string]int{}, started: time.Now()}
	l.places[user] = r
	code := r.p.Next()
	if code == "" {
		delete(l.places, user)
		return &PlacementView{Done: true, Max: placementMaxProbes, Total: len(g.Codes)}, nil
	}
	return &PlacementView{Probe: 1, Max: placementMaxProbes, Question: r.ask(l, code), Total: len(g.Codes)}, nil
}

// AnswerPlacement records an answer (choice −1 is "don't know") and returns
// the next question or, at the end, how many ideas were placed.
func (l *Lab) AnswerPlacement(ctx context.Context, user int64, choice int) (*PlacementView, error) {
	l.placeMu.Lock()
	r := l.places[user]
	if r == nil {
		l.placeMu.Unlock()
		return nil, errNotFound
	}
	q := l.c.Ideas[r.q.Code].Questions[r.q.Index]
	right := choice == q.Correct
	r.p.Record(r.q.Code, right)
	v := &PlacementView{Max: placementMaxProbes, Probe: r.p.Probes + 1, Correct: &right, CorrectIndex: &q.Correct, Explanation: q.Explanation, Total: len(r.p.g.Codes)}
	if code := r.p.Next(); code != "" {
		v.Question = r.ask(l, code)
		l.placeMu.Unlock()
		return v, nil
	}
	delete(l.places, user)
	l.placeMu.Unlock()

	v.Done = true
	now := time.Now().UTC()
	tx, err := l.st.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	mem, err := loadMemory(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	for code := range r.p.Known {
		if mem[code].Status == "learned" {
			continue
		}
		if err := saveMemory(ctx, tx, user, code, Placed(code, r.p.Direct[code], now)); err != nil {
			return nil, err
		}
		v.Known++
	}
	return v, tx.Commit()
}
