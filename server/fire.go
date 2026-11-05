package main

// Scheduling: FSRS-6 memory for every idea, plus FIRe (fractional implicit
// repetition) over the knowledge graph.
//
// FSRS-6 follows the open specification of open-spaced-repetition
// (https://github.com/open-spaced-repetition, MIT) with the default
// parameters of py-fsrs 6.3.2. An idea's memory is its stability S (days
// until recall drops to 90%) and difficulty D (1–10); recall decays as
// R(t) = (1 + factor·t/S)^decay. A learned idea is due when R < 0.9.
//
// FIRe: solving an idea also counts as a fraction of a Good review of the
// simpler ideas it encompasses. A target reached with path weight w (the
// product of encompassing weights along the best path) moves its stability
// and recall the fraction w of the way to what a Good review would give.

import (
	"math"
	"sort"
	"time"
)

type Rating int

const (
	Again Rating = 1
	Hard  Rating = 2
	Good  Rating = 3
	Easy  Rating = 4
)

var fsrsW = [21]float64{
	0.212, 1.2931, 2.3065, 8.2956, 6.4133, 0.8334, 3.0194, 0.001,
	1.8722, 0.1666, 0.796, 1.4835, 0.0614, 0.2629, 1.6483, 0.6014,
	1.8729, 0.5425, 0.0912, 0.0658, 0.1542,
}

const (
	DesiredRetention = 0.9
	stabilityMin     = 0.001
	maxIntervalDays  = 36500
	day              = 24 * time.Hour
)

var (
	fsrsDecay  = -fsrsW[20]
	fsrsFactor = math.Pow(0.9, 1/fsrsDecay) - 1
)

func clampS(s float64) float64 { return math.Max(s, stabilityMin) }
func clampD(d float64) float64 { return math.Min(math.Max(d, 1), 10) }

// Recall after `days` at stability s.
func Recall(s, days float64) float64 {
	if s <= 0 {
		return 0
	}
	return math.Pow(1+fsrsFactor*math.Max(0, days)/s, fsrsDecay)
}

// DaysUntil is the time at which recall falls to r.
func DaysUntil(s, r float64) float64 { return s / fsrsFactor * (math.Pow(r, 1/fsrsDecay) - 1) }

func IntervalDays(s float64) float64 {
	return math.Min(math.Max(math.Round(DaysUntil(s, DesiredRetention)), 1), maxIntervalDays)
}

func initialS(g Rating) float64 { return clampS(fsrsW[g-1]) }
func initialD(g Rating, clamp bool) float64 {
	d := fsrsW[4] - math.Exp(fsrsW[5]*float64(g-1)) + 1
	if clamp {
		return clampD(d)
	}
	return d
}

func nextD(d float64, g Rating) float64 {
	delta := -fsrsW[6] * float64(g-3)
	damped := d + (10-d)*delta/9
	return clampD(fsrsW[7]*initialD(Easy, false) + (1-fsrsW[7])*damped)
}

func recallS(d, s, r float64, g Rating) float64 {
	hard, easy := 1.0, 1.0
	if g == Hard {
		hard = fsrsW[15]
	}
	if g == Easy {
		easy = fsrsW[16]
	}
	return clampS(s * (1 + math.Exp(fsrsW[8])*(11-d)*math.Pow(s, -fsrsW[9])*(math.Exp((1-r)*fsrsW[10])-1)*hard*easy))
}

func forgetS(d, s, r float64) float64 {
	long := fsrsW[11] * math.Pow(d, -fsrsW[12]) * (math.Pow(s+1, fsrsW[13]) - 1) * math.Exp((1-r)*fsrsW[14])
	short := s / math.Exp(fsrsW[17]*fsrsW[18])
	return clampS(math.Min(long, short))
}

func shortTermS(s float64, g Rating) float64 {
	inc := math.Exp(fsrsW[17]*(float64(g)-3+fsrsW[18])) * math.Pow(s, -fsrsW[19])
	if g >= Hard {
		inc = math.Max(inc, 1)
	}
	return clampS(s * inc)
}

// Memory is the scheduling state of one idea for one learner.
type Memory struct {
	Status       string // new | partial | learned
	S, D         float64
	Reps         float64 // explicit reviews plus implicit credit
	LastReviewAt time.Time
	DueAt        time.Time
}

func (m Memory) elapsed(now time.Time) float64 {
	if m.LastReviewAt.IsZero() {
		return 0
	}
	return math.Max(0, now.Sub(m.LastReviewAt).Hours()/24)
}

// RecallNow is the probability to recall the idea now (0 if never learned).
func (m Memory) RecallNow(now time.Time) float64 {
	if m.Status != "learned" || m.S <= 0 {
		return 0
	}
	return Recall(m.S, m.elapsed(now))
}

func (m Memory) Due(now time.Time) bool {
	return m.Status == "learned" && m.RecallNow(now) < DesiredRetention
}

func (m *Memory) schedule() {
	m.DueAt = m.LastReviewAt.Add(time.Duration(IntervalDays(m.S) * float64(day)))
}

// Learn is the first successful lesson.
func (m Memory) Learn(g Rating, now time.Time) Memory {
	m.Status, m.S, m.D, m.Reps, m.LastReviewAt = "learned", initialS(g), initialD(g, true), 1, now
	m.schedule()
	return m
}

// Review applies an explicit review (lesson retake, review or quiz).
func (m Memory) Review(g Rating, now time.Time) Memory {
	if m.Status != "learned" || m.S <= 0 {
		return m.Learn(g, now)
	}
	r, days := m.RecallNow(now), m.elapsed(now)
	switch {
	case days < 1:
		m.S = shortTermS(m.S, g)
	case g == Again:
		m.S = forgetS(m.D, m.S, r)
	default:
		m.S = recallS(m.D, m.S, r, g)
	}
	m.D = nextD(m.D, g)
	m.LastReviewAt = now
	if g > Again {
		m.Reps++
	}
	m.schedule()
	return m
}

// setRecall places LastReviewAt so that recall right now equals r at the
// current stability.
func (m *Memory) setRecall(r float64, now time.Time) {
	r = math.Min(math.Max(r, 0.01), 1)
	m.LastReviewAt = now.Add(-time.Duration(DaysUntil(m.S, r) * float64(day)))
	m.schedule()
}

// Credit applies FIRe's fractional implicit review: the fraction w of a Good
// review. It returns the gain in recall, which is w·(1 − R).
func (m Memory) Credit(w float64, now time.Time) (Memory, float64) {
	if m.Status != "learned" || w <= 0 {
		return m, 0
	}
	r := m.RecallNow(now)
	target := recallS(m.D, m.S, r, Good)
	if m.elapsed(now) < 1 {
		target = shortTermS(m.S, Good)
	}
	m.S = clampS(m.S + w*(target-m.S))
	gain := w * (1 - r)
	m.Reps += w
	m.setRecall(r+gain, now)
	return m, gain
}

// Weaken applies the fraction w of a failure to an idea built on a failed one.
func (m Memory) Weaken(w float64, now time.Time) Memory {
	if m.Status != "learned" || w <= 0 {
		return m
	}
	r := m.RecallNow(now)
	m.S = clampS(m.S + w*(forgetS(m.D, m.S, r)-m.S))
	m.setRecall(r, now)
	return m
}

// DueNow makes a learned idea due immediately (key prerequisites of a failed
// lesson, ideas missed in a quiz).
func (m Memory) DueNow(now time.Time) Memory {
	if m.Status == "learned" {
		m.setRecall(DesiredRetention-0.001, now)
	}
	return m
}

// LessonResult: a lesson passes on two right answers in a row (one when it
// was started before) and fails on two wrong in a row.
func LessonResult(partial bool, answers []bool) (done, passed, perfect bool) {
	need, right, wrong := 2, 0, 0
	if partial {
		need = 1
	}
	perfect = true
	for _, a := range answers {
		if a {
			right, wrong = right+1, 0
		} else {
			wrong, right, perfect = wrong+1, 0, false
		}
		if right >= need {
			return true, true, perfect
		}
		if wrong >= 2 {
			return true, false, false
		}
	}
	return false, false, perfect
}

// RatingFrom maps answers to a rating: all right is Good, most right Hard.
func RatingFrom(correct, total int) Rating {
	switch {
	case total == 0:
		return Again
	case correct == total:
		return Good
	case float64(correct)/float64(total) >= 0.5:
		return Hard
	}
	return Again
}

// ---- the knowledge graph ----

type FIREEdge struct {
	Code         string
	Key          bool
	Encompassing float64
}

type FIREIdea struct {
	Code, ID      string
	Level         int
	Weight        float64
	Prerequisites []FIREEdge
}

// spread returns the best-path weight to every idea reached from code: down
// the prerequisites (credit) or up to the dependents (failure). Paths below
// 0.1 stop.
func spread(graph map[string]FIREIdea, dependents map[string][]FIREEdge, code string, up bool) map[string]float64 {
	weights := map[string]float64{code: 1}
	queue := []string{code}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		edges := graph[cur].Prerequisites
		if up {
			edges = dependents[cur]
		}
		for _, e := range edges {
			w := weights[cur] * e.Encompassing
			if w < 0.1 || w <= weights[e.Code] {
				continue
			}
			weights[e.Code] = w
			queue = append(queue, e.Code)
		}
	}
	delete(weights, code)
	return weights
}

// Propagate spreads a successful solve (credit down) or a failure (weakening
// up) and returns the recall gain per credited idea.
func Propagate(graph map[string]FIREIdea, dependents map[string][]FIREEdge, mem map[string]Memory, code string, success bool, now time.Time) map[string]float64 {
	gains := map[string]float64{}
	for target, w := range spread(graph, dependents, code, !success) {
		m, ok := mem[target]
		if !ok || m.Status != "learned" {
			continue
		}
		if success {
			var g float64
			m, g = m.Credit(w, now)
			gains[target] = g
		} else {
			m = m.Weaken(w, now)
		}
		mem[target] = m
	}
	return gains
}

func KeyReviews(idea FIREIdea, mem map[string]Memory, now time.Time) {
	for _, e := range idea.Prerequisites {
		if m, ok := mem[e.Code]; e.Key && ok && m.Status == "learned" {
			mem[e.Code] = m.DueNow(now)
		}
	}
}

func Frontier(idea FIREIdea, mem map[string]Memory) bool {
	for _, e := range idea.Prerequisites {
		m := mem[e.Code]
		if m.Status != "learned" && !(m.Status == "partial" && m.Reps >= 0.5) {
			return false
		}
	}
	return true
}

type FIRETask struct{ Kind, IdeaID string }

// Next produces at most limit tasks: due reviews, except those a frontier
// lesson covers with weight ≥ 0.5 (the lesson replaces the review), and at
// least a quarter new lessons, heaviest ideas first.
func Next(ideas []FIREIdea, mem map[string]Memory, now time.Time, limit int) []FIRETask {
	if limit <= 0 {
		return nil
	}
	lessons, reviews := []FIREIdea{}, []FIREIdea{}
	for _, idea := range ideas {
		m := mem[idea.Code]
		if (m.Status == "" || m.Status == "new" || m.Status == "partial") && Frontier(idea, mem) {
			lessons = append(lessons, idea)
		}
		if m.Due(now) {
			reviews = append(reviews, idea)
		}
	}
	less := func(a, b FIREIdea) bool {
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.ID < b.ID
	}
	sort.Slice(lessons, func(i, j int) bool { return less(lessons[i], lessons[j]) })
	// The most forgotten reviews first.
	sort.Slice(reviews, func(i, j int) bool {
		ri, rj := mem[reviews[i].Code].RecallNow(now), mem[reviews[j].Code].RecallNow(now)
		if ri != rj {
			return ri < rj
		}
		return less(reviews[i], reviews[j])
	})
	selected := map[string]bool{}
	tasks := []FIRETask{}
	add := func(kind string, idea FIREIdea) {
		if len(tasks) < limit && !selected[idea.ID] {
			tasks = append(tasks, FIRETask{kind, idea.ID})
			selected[idea.ID] = true
		}
	}
	covers := func(lesson FIREIdea, code string) bool {
		for _, e := range lesson.Prerequisites {
			if e.Code == code && e.Encompassing >= 0.5 {
				return true
			}
		}
		return false
	}
	for _, review := range reviews {
		covered := false
		for _, lesson := range lessons {
			if covers(lesson, review.Code) {
				add("lesson", lesson)
				covered = true
				break
			}
		}
		if !covered {
			add("review", review)
		}
	}
	minLessons := (min(limit, len(reviews)+len(lessons)) + 3) / 4
	for _, lesson := range lessons {
		count := 0
		for _, t := range tasks {
			if t.Kind == "lesson" {
				count++
			}
		}
		if count >= minLessons {
			break
		}
		if len(tasks) >= limit {
			for i := len(tasks) - 1; i >= 0; i-- {
				if tasks[i].Kind == "review" {
					delete(selected, tasks[i].IdeaID)
					tasks = append(tasks[:i], tasks[i+1:]...)
					break
				}
			}
		}
		add("lesson", lesson)
	}
	for _, lesson := range lessons {
		add("lesson", lesson)
	}
	return tasks
}

type FIREForecast struct {
	RemainingXP, Pace, Days float64
	ETADate                 time.Time
}

// Estimate is the XP left to learn every idea and to bring each one to about
// four successful reviews, at the given pass rate and daily pace.
func Estimate(ideas []FIREIdea, mem map[string]Memory, passRate, pace float64, now time.Time) FIREForecast {
	if passRate <= 0 || passRate > 1 {
		passRate = 0.85
	}
	if pace <= 0 {
		pace = 20
	}
	remaining := 0.0
	for _, idea := range ideas {
		m := mem[idea.Code]
		reps := m.Reps
		if m.Status != "learned" {
			remaining += float64(3+3*idea.Level) / passRate
			reps = 1
		}
		remaining += 3 * math.Max(0, 4-reps) / passRate
	}
	remaining += remaining / 150 * 15 * 0.25
	days := remaining / (pace * math.Pow(pace/40, 0.1))
	return FIREForecast{remaining, pace, days, now.Add(time.Duration(days * float64(day)))}
}

func QuizDue(xpSinceQuiz int) bool { return xpSinceQuiz >= 150 }
