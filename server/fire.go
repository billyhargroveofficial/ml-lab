package main

import (
	"math"
	"sort"
	"time"
)

// FIREMemory is the pure scheduling state of one idea. Times are UTC instants.
type FIREMemory struct {
	RepNum, Memory, IntervalDays, Speed, Accuracy float64
	Status                                        string
	LastReviewAt                                  time.Time
	DueAt                                         time.Time
}

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

var fireIntervals = [...]float64{1, 3, 7, 14, 30, 60, 120, 240, 365}

func FIREInterval(rep float64) float64 {
	i := int(math.Floor(math.Max(0, rep)))
	if i >= len(fireIntervals) {
		i = len(fireIntervals) - 1
	}
	return fireIntervals[i]
}
func fireClamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
func FIREMemoryNow(s FIREMemory, now time.Time) float64 {
	if s.LastReviewAt.IsZero() || s.IntervalDays <= 0 {
		return fireClamp(s.Memory, 0, 1)
	}
	days := math.Max(0, now.Sub(s.LastReviewAt).Hours()/24)
	return fireClamp(s.Memory, 0, 1) * math.Exp2(-days/s.IntervalDays)
}
func FIREDueAt(s FIREMemory) time.Time {
	if s.LastReviewAt.IsZero() || s.IntervalDays <= 0 {
		return time.Time{}
	}
	if s.Memory <= 0.5 {
		return s.LastReviewAt
	}
	return s.LastReviewAt.Add(time.Duration(s.IntervalDays * math.Log2(s.Memory/0.5) * float64(24*time.Hour)))
}
func FIREDue(s FIREMemory, now time.Time) bool {
	return s.Status == "learned" && FIREMemoryNow(s, now) < 0.5
}
func FIREAccuracy(s FIREMemory, correct bool, level int) FIREMemory {
	value := 0.0
	if correct {
		value = 1
	}
	s.Accuracy = 0.7*s.Accuracy + 0.3*value
	difficulty := 0.70
	if level == 1 {
		difficulty = 0.85
	}
	if level == 3 {
		difficulty = 0.55
	}
	s.Speed = fireClamp(s.Accuracy/difficulty, 0.5, 2)
	return s
}

// FIREReview applies a fractional success or a failure to an existing memory.
func FIREReview(s FIREMemory, raw float64, now time.Time) FIREMemory {
	m := FIREMemoryNow(s, now)
	multiplier := 1.0
	if raw > 0 {
		multiplier = 1 - m
	} else if raw < 0 && s.IntervalDays > 0 && !s.LastReviewAt.IsZero() {
		overdue := math.Max(0, now.Sub(FIREDueAt(s)).Hours()/24)
		multiplier += overdue / s.IntervalDays
	}
	s.RepNum = math.Max(0, s.RepNum+s.Speed*raw*multiplier)
	if raw < 0 {
		s.Memory = 0.25
	} else {
		s.Memory = fireClamp(m+raw, 0, 1)
	}
	s.IntervalDays = FIREInterval(s.RepNum)
	s.LastReviewAt = now
	s.DueAt = FIREDueAt(s)
	return s
}
func FIRECompleteLesson(s FIREMemory, now time.Time) FIREMemory {
	s.Status = "learned"
	s.RepNum = 1
	s.Memory = 1
	s.IntervalDays = FIREInterval(1)
	s.LastReviewAt = now
	s.DueAt = FIREDueAt(s)
	return s
}

// FIRELessonResult reports completion, pass and whether the lesson was perfect.
func FIRELessonResult(partial bool, answers []bool) (done, passed, perfect bool) {
	needed := 2
	if partial {
		needed = 1
	}
	correct, wrong := 0, 0
	perfect = true
	for _, answer := range answers {
		if answer {
			correct++
			wrong = 0
		} else {
			wrong++
			correct = 0
			perfect = false
		}
		if correct >= needed {
			return true, true, perfect
		}
		if wrong >= 2 {
			return true, false, false
		}
	}
	return false, false, perfect
}
func FIREFrontier(idea FIREIdea, memory map[string]FIREMemory) bool {
	for _, edge := range idea.Prerequisites {
		m := memory[edge.Code]
		if m.Status != "learned" && !(m.Status == "partial" && m.RepNum >= 0.5) {
			return false
		}
	}
	return true
}

// FIREPropagate applies the maximum-weight path to each node once. Graphs are
// expected to have already passed the content importer's cycle validation.
func FIREPropagate(ideas map[string]FIREIdea, memory map[string]FIREMemory, code string, raw float64, now time.Time) {
	weights := map[string]float64{code: 1}
	queue := []string{code}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		edges := ideas[current].Prerequisites
		if raw < 0 {
			edges = nil
			for child, idea := range ideas {
				for _, e := range idea.Prerequisites {
					if e.Code == current {
						edges = append(edges, FIREEdge{Code: child, Encompassing: e.Encompassing})
					}
				}
			}
		}
		for _, edge := range edges {
			w := weights[current] * edge.Encompassing
			if w < 0.1 || w <= weights[edge.Code] {
				continue
			}
			weights[edge.Code] = w
			queue = append(queue, edge.Code)
		}
	}
	delete(weights, code)
	for target, weight := range weights {
		s, ok := memory[target]
		if !ok || s.Status != "learned" {
			continue
		}
		if raw > 0 {
			if s.Speed < 1 {
				continue
			}
			s = FIREReview(s, raw*weight, now)
		} else if raw < 0 {
			s.Memory = math.Max(0, FIREMemoryNow(s, now)-0.5*weight)
			s.LastReviewAt = now
			s.DueAt = FIREDueAt(s)
		}
		memory[target] = s
	}
}
func FIREKeyReviews(idea FIREIdea, memory map[string]FIREMemory, now time.Time) {
	for _, edge := range idea.Prerequisites {
		if !edge.Key {
			continue
		}
		s, ok := memory[edge.Code]
		if ok && s.Status == "learned" {
			s.Memory = math.Min(FIREMemoryNow(s, now), 0.4)
			s.LastReviewAt = now
			s.DueAt = now
			memory[edge.Code] = s
		}
	}
}

type FIRETask struct{ Kind, IdeaID string }

// FIRENext produces at most limit tasks with a lesson share of at least 25%
// whenever enough frontier lessons exist. Covered reviews yield to lessons.
func FIRENext(ideas []FIREIdea, memory map[string]FIREMemory, now time.Time, limit int, quizDue bool) []FIRETask {
	if limit <= 0 {
		return nil
	}
	if quizDue {
		return []FIRETask{{Kind: "quiz"}}
	}
	lessons, reviews := []FIREIdea{}, []FIREIdea{}
	for _, idea := range ideas {
		m := memory[idea.Code]
		if (m.Status == "" || m.Status == "new" || m.Status == "partial") && FIREFrontier(idea, memory) {
			lessons = append(lessons, idea)
		}
		if FIREDue(m, now) {
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
	sort.Slice(reviews, func(i, j int) bool { return less(reviews[i], reviews[j]) })
	selected := map[string]bool{}
	tasks := []FIRETask{}
	add := func(kind string, idea FIREIdea) {
		if len(tasks) < limit && !selected[idea.ID] {
			tasks = append(tasks, FIRETask{kind, idea.ID})
			selected[idea.ID] = true
		}
	}
	for _, review := range reviews {
		for _, lesson := range lessons {
			for _, edge := range lesson.Prerequisites {
				if edge.Code == review.Code && edge.Encompassing >= 0.5 {
					add("lesson", lesson)
				}
			}
		}
		if len(tasks) < limit {
			covered := false
			for _, lesson := range lessons {
				if selected[lesson.ID] {
					for _, edge := range lesson.Prerequisites {
						if edge.Code == review.Code && edge.Encompassing >= 0.5 {
							covered = true
						}
					}
				}
			}
			if !covered {
				add("review", review)
			}
		}
	}
	minLessons := (min(limit, len(reviews)+len(lessons)) + 3) / 4
	for _, lesson := range lessons {
		count := 0
		for _, task := range tasks {
			if task.Kind == "lesson" {
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

func FIREEstimate(ideas []FIREIdea, memory map[string]FIREMemory, passRate, pace, quizMean float64, now time.Time) FIREForecast {
	if passRate <= 0 || passRate > 1 {
		passRate = 0.85
	}
	if pace <= 0 {
		pace = 20
	}
	if quizMean < 0 || quizMean > 1 {
		quizMean = 0.75
	}
	remaining := 0.0
	for _, idea := range ideas {
		s := memory[idea.Code]
		rep := s.RepNum
		if s.Status != "learned" {
			xp := 9.0
			if idea.Level == 1 {
				xp = 6
			}
			if idea.Level == 3 {
				xp = 12
			}
			remaining += xp / passRate
			rep = 1
		}
		speed := s.Speed
		if speed <= 0 {
			speed = 1
		}
		remaining += 3 * math.Max(0, 4-rep) / speed / passRate
	}
	remaining += remaining / 150 * 15 * (2 - quizMean)
	days := remaining / (pace * math.Pow(pace/40, 0.1))
	return FIREForecast{remaining, pace, days, now.Add(time.Duration(days * float64(24*time.Hour)))}
}

// FIREQuizDue uses XP earned in ML Lab since the last completed quiz.
func FIREQuizDue(xpSinceQuiz int) bool { return xpSinceQuiz >= 150 }
