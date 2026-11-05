package main

// Learning-cycle simulator. Runs virtual learners through the real
// scheduler (Next, FSRS memory, FIRe credit) day by day and reports how
// long the program takes and how heavy reviews get. Answers come from a
// learner model that knows the true memory, which differs from what the
// scheduler believes by a factor k on stability.
//
//	SIM=1 go test -run TestSimulate -v .

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

type learner struct {
	Name  string
	Skill float64 // chance to answer a lesson question right
	K     float64 // true stability = scheduler's stability × K
}

type scenario struct {
	Program string // all | math
	Goal    int    // nominal XP of work per day
	L       learner
	NoFIRe  bool
	// Alpha is the share of implicit credit that really reaches the learner's
	// memory: 1 — the encompassing weights are right, 0.5 — twice too
	// generous, 0 — implicit repetition does not work at all.
	Alpha float64
	Cap   float64 // ImplicitCap; 1000 ≈ no guard
	Place int     // what the learner knows before starting, settled by the placement test: 0 nothing, 1 school base, 2 school base + basic linear algebra and probability
}

type simResult struct {
	Scenario                    scenario
	DaysToFinish                int // 0 = not finished within the horizon
	Learned, Total              int
	Probes, Placed, FalsePlaced int
	Lessons, Reviews, Quizzes   int
	ReviewsPerDayLast30         float64
	ReviewShare                 float64 // share of the daily budget spent on reviews and quizzes
	MaxBacklog                  int
	TrueRecallEnd, ReviewPass   float64
	WeakShare                   float64 // learned ideas whose real recall < 0.7 at the end
	ImplicitCredits             int
	MeanStabilityEnd            float64
	LessonFails                 int
	DaysPerMonth                []int // learned ideas at day 30, 60, 90, ...
}

func trueRecall(m Memory, k float64, now time.Time) float64 {
	if m.Status != "learned" {
		return 0
	}
	return Recall(m.S*k, m.elapsed(now))
}

func simulate(c *Content, sc scenario, horizon int, seed int64) simResult {
	rng := rand.New(rand.NewSource(seed))
	ImplicitCap = sc.Cap
	defer func() { ImplicitCap = 5 }()
	ideas := c.schedulingIdeas(sc.Program)
	inProgram := map[string]bool{}
	for _, i := range ideas {
		inProgram[i.Code] = true
	}
	mem := map[string]Memory{}   // what the scheduler believes
	truth := map[string]Memory{} // the learner's real memory
	res := simResult{Scenario: sc, Total: len(ideas)}
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	sinceQuiz := 0
	reviewsByDay := make([]int, horizon)
	var reviewPass, reviewAll float64
	partial := map[string]bool{}
	propagate := func(code string, ok bool, now time.Time) {
		if sc.NoFIRe && ok {
			return
		}
		g := Propagate(c.Graph, c.Up, mem, code, ok, now)
		for _, v := range g {
			if v >= 0.01 {
				res.ImplicitCredits++
			}
		}
		if ok {
			for target, w := range spread(c.Graph, c.Up, code, false) {
				if m, have := truth[target]; have && m.Status == "learned" {
					truth[target], _ = m.credit(w*sc.Alpha, now, false) // real memory has no cap
				}
			}
		}
	}
	// Explicit work changes both the belief and the real memory.
	learn := func(code string, g Rating, now time.Time) {
		mem[code] = mem[code].Learn(g, now)
		truth[code] = truth[code].Learn(g, now)
	}
	review := func(code string, g Rating, now time.Time) {
		mem[code] = mem[code].Review(g, now)
		truth[code] = truth[code].Review(g, now)
	}
	if sc.Place > 0 {
		knows := priorKnowledge(c, ideas, sc.Place)
		for code := range knows {
			truth[code] = Memory{Status: "learned", S: 60, D: 5, Reps: 1, LastReviewAt: start}
		}
		p := NewPlacement(NewPlacementGraph(ideas), nil)
		for code := p.Next(); code != ""; code = p.Next() {
			right := false
			if knows[code] {
				right = rng.Float64() < 0.9
			} else {
				right = rng.Float64() < 0.3*0.25 // "don't know" most of the time, else a guess
			}
			p.Record(code, right)
		}
		res.Probes = p.Probes
		for code := range p.Known {
			mem[code] = Placed(code, p.Direct[code], start)
			res.Placed++
			if !knows[code] {
				res.FalsePlaced++
			}
		}
	}
	for d := 0; d < horizon; d++ {
		now := start.Add(time.Duration(d) * day)
		budget := float64(sc.Goal)
		due := 0
		for _, i := range ideas {
			if mem[i.Code].Due(now) {
				due++
			}
		}
		res.MaxBacklog = max(res.MaxBacklog, due)
		for budget > 0 {
			now = now.Add(4 * time.Minute)
			if QuizDue(sinceQuiz) {
				pool := []string{}
				for code, m := range mem {
					if m.Status == "learned" && inProgram[code] {
						pool = append(pool, code)
					}
				}
				if len(pool) >= 5 {
					sort.Strings(pool)
					rng.Shuffle(len(pool), func(a, b int) { pool[a], pool[b] = pool[b], pool[a] })
					pool = pool[:min(10, len(pool))]
					for _, code := range pool {
						ok := rng.Float64() < trueRecall(truth[code], sc.L.K, now)
						g := Good
						if !ok {
							g = Again
						}
						review(code, g, now)
						propagate(code, ok, now)
						if !ok {
							mem[code] = mem[code].DueNow(now)
						}
					}
					res.Quizzes++
					sinceQuiz = 0
					budget -= 15
					continue
				}
			}
			tasks := Next(ideas, mem, now, 8) // the learner takes the first of the feed, as in the app
			if len(tasks) == 0 {
				break
			}
			idea := c.ByID[tasks[0].IdeaID]
			code := idea.Code
			if tasks[0].Kind == "lesson" {
				nq := max(3, len(idea.Questions))
				answers := []bool{}
				done, pass, perfect := false, false, false
				for len(answers) < nq && !done {
					answers = append(answers, rng.Float64() < sc.L.Skill)
					done, pass, perfect = LessonResult(partial[code], answers)
				}
				if !done {
					right := 0
					for _, a := range answers {
						if a {
							right++
						}
					}
					pass = right > len(answers)-right
				}
				xp := 3 + 3*idea.Level
				budget -= float64(xp)
				res.Lessons++
				if pass {
					g := Good
					if !perfect {
						g = Hard
					}
					learn(code, g, now)
					propagate(code, true, now)
					sinceQuiz += xp
				} else {
					res.LessonFails++
					m := mem[code]
					m.Status = "partial"
					mem[code] = m
					partial[code] = true
					propagate(code, false, now)
					KeyReviews(c.Graph[code], mem, now)
				}
				continue
			}
			// Review: two questions at true recall.
			right := 0
			for q := 0; q < 2; q++ {
				if rng.Float64() < trueRecall(truth[code], sc.L.K, now) {
					right++
				}
			}
			g := RatingFrom(right, 2)
			review(code, g, now)
			propagate(code, g > Again, now)
			budget -= 3
			res.Reviews++
			reviewsByDay[d]++
			reviewAll++
			reviewPass += float64(right) / 2
			if right > 0 {
				sinceQuiz += 3
			}
		}
		learned := 0
		for _, i := range ideas {
			if mem[i.Code].Status == "learned" {
				learned++
			}
		}
		if (d+1)%30 == 0 {
			res.DaysPerMonth = append(res.DaysPerMonth, learned)
		}
		if learned == len(ideas) && res.DaysToFinish == 0 {
			res.DaysToFinish = d + 1
		}
	}
	end := start.Add(time.Duration(horizon) * day)
	var rsum, ssum float64
	weakN := 0
	for _, i := range ideas {
		m := mem[i.Code]
		if m.Status == "learned" {
			res.Learned++
			tr := trueRecall(truth[i.Code], sc.L.K, end)
			rsum += tr
			ssum += m.S
			if tr < 0.7 {
				weakN++
			}
		}
	}
	if res.Learned > 0 {
		res.WeakShare = float64(weakN) / float64(res.Learned)
	}
	if res.Learned > 0 {
		res.TrueRecallEnd = rsum / float64(res.Learned)
		res.MeanStabilityEnd = ssum / float64(res.Learned)
	}
	if reviewAll > 0 {
		res.ReviewPass = reviewPass / reviewAll
	}
	sum := 0
	for _, r := range reviewsByDay[horizon-30:] {
		sum += r
	}
	res.ReviewsPerDayLast30 = float64(sum) / 30
	totalWork := float64(res.Lessons)*9 + float64(res.Reviews)*3 + float64(res.Quizzes)*15
	if totalWork > 0 {
		res.ReviewShare = (float64(res.Reviews)*3 + float64(res.Quizzes)*15) / totalWork
	}
	return res
}

// priorKnowledge: the ideas a learner knows before starting — every level-1
// idea (level ≥ 2: also level-2) with everything below it, so the knowledge
// is downward closed as a real learner's roughly is.
func priorKnowledge(c *Content, ideas []FIREIdea, level int) map[string]bool {
	g := NewPlacementGraph(ideas)
	knows := map[string]bool{}
	for n, i := range ideas {
		if c.Ideas[i.Code].Level <= level-0 && c.Ideas[i.Code].Level < 3 && (level >= 2 || c.Ideas[i.Code].Level == 1) {
			for j, code := range g.Codes {
				if g.below[n].has(j) {
					knows[code] = true
				}
			}
		}
	}
	return knows
}

func TestSimulate(t *testing.T) {
	if os.Getenv("SIM") == "" {
		t.Skip("set SIM=1 to run the learning simulator")
	}
	dir := os.Getenv("SIM_PACKS") // default: the repo's packs/
	if dir == "" {
		dir = "../packs"
	}
	c, _ := LoadPacks(dir)
	if len(c.Ideas) == 0 {
		t.Skip("no packs in " + dir)
	}
	strong := learner{"сильный", 0.92, 1.4}
	average := learner{"средний", 0.82, 1.0}
	weak := learner{"слабый", 0.68, 0.6}
	var scs []scenario
	for _, g := range []int{40, 80, 120} {
		scs = append(scs, scenario{Program: "all", Goal: g, L: average, Alpha: 1, Cap: 5})
	}
	scs = append(scs,
		scenario{Program: "all", Goal: 120, L: strong, Alpha: 1, Cap: 5},
		scenario{Program: "all", Goal: 120, L: weak, Alpha: 1, Cap: 5},
		scenario{Program: "all", Goal: 120, L: average, Alpha: 0.5, Cap: 5},
		scenario{Program: "all", Goal: 120, L: average, Alpha: 1, Cap: 5, Place: 1},
		scenario{Program: "all", Goal: 120, L: average, Alpha: 1, Cap: 5, Place: 2},
		scenario{Program: "all", Goal: 120, L: weak, Alpha: 1, Cap: 5, Place: 2},
	)
	horizon := 540
	var b strings.Builder
	fmt.Fprintf(&b, "probes/placed/false\tprogram\tgoal\tlearner\tFIRe\tfinish_day\tlearned\tlessons\tfails\treviews\tquizzes\treviews/day(last30)\treview_share\tmax_backlog\ttrue_recall_end\tweak_share\treview_pass\timplicit\tmeanS\tby_month\n")
	for _, sc := range scs {
		var rs []simResult
		for seed := int64(1); seed <= 3; seed++ {
			rs = append(rs, simulate(c, sc, horizon, seed))
		}
		r := average3(rs)
		fire := fmt.Sprintf("a=%.1f place=%d", sc.Alpha, sc.Place)
		if sc.NoFIRe {
			fire = "off"
		}
		fmt.Fprintf(&b, "%d/%d/%d\t%s\t%d\t%s\t%s\t%d\t%d/%d\t%d\t%d\t%d\t%d\t%.1f\t%.0f%%\t%d\t%.2f\t%.0f%%\t%.2f\t%d\t%.0f\t%v\n",
			r.Probes, r.Placed, r.FalsePlaced, sc.Program, sc.Goal, sc.L.Name, fire, r.DaysToFinish, r.Learned, r.Total, r.Lessons, r.LessonFails, r.Reviews, r.Quizzes,
			r.ReviewsPerDayLast30, r.ReviewShare*100, r.MaxBacklog, r.TrueRecallEnd, r.WeakShare*100, r.ReviewPass, r.ImplicitCredits, r.MeanStabilityEnd, r.DaysPerMonth)
	}
	out := os.Getenv("SIM_OUT")
	if out != "" {
		_ = os.WriteFile(out, []byte(b.String()), 0o644)
	}
	t.Log("\n" + b.String())
}

func average3(rs []simResult) simResult {
	r := rs[0]
	n := float64(len(rs))
	avgI := func(f func(simResult) int) int {
		s := 0.0
		for _, x := range rs {
			s += float64(f(x))
		}
		return int(math.Round(s / n))
	}
	avgF := func(f func(simResult) float64) float64 {
		s := 0.0
		for _, x := range rs {
			s += f(x)
		}
		return s / n
	}
	finished := 0
	for _, x := range rs {
		if x.DaysToFinish > 0 {
			finished++
		}
	}
	if finished == len(rs) {
		r.DaysToFinish = avgI(func(x simResult) int { return x.DaysToFinish })
	} else {
		r.DaysToFinish = 0
	}
	r.Learned = avgI(func(x simResult) int { return x.Learned })
	r.Probes = avgI(func(x simResult) int { return x.Probes })
	r.Placed = avgI(func(x simResult) int { return x.Placed })
	r.FalsePlaced = avgI(func(x simResult) int { return x.FalsePlaced })
	r.Lessons = avgI(func(x simResult) int { return x.Lessons })
	r.LessonFails = avgI(func(x simResult) int { return x.LessonFails })
	r.Reviews = avgI(func(x simResult) int { return x.Reviews })
	r.Quizzes = avgI(func(x simResult) int { return x.Quizzes })
	r.MaxBacklog = avgI(func(x simResult) int { return x.MaxBacklog })
	r.ImplicitCredits = avgI(func(x simResult) int { return x.ImplicitCredits })
	r.ReviewsPerDayLast30 = avgF(func(x simResult) float64 { return x.ReviewsPerDayLast30 })
	r.ReviewShare = avgF(func(x simResult) float64 { return x.ReviewShare })
	r.TrueRecallEnd = avgF(func(x simResult) float64 { return x.TrueRecallEnd })
	r.WeakShare = avgF(func(x simResult) float64 { return x.WeakShare })
	r.ReviewPass = avgF(func(x simResult) float64 { return x.ReviewPass })
	r.MeanStabilityEnd = avgF(func(x simResult) float64 { return x.MeanStabilityEnd })
	return r
}
