package main

// Placement test: a short adaptive test that marks what a learner already
// knows, so the program starts where they are rather than at the bottom.
//
// The track's prerequisite graph is a partial order. Knowing an idea implies
// knowing everything below it; not knowing it implies not knowing anything
// built on it. Every probe therefore settles a whole region of the graph, and
// we probe the idea that settles the most in the worse case: the unsettled
// idea with the largest min(unsettled below, unsettled above). This is a
// binary search generalised from a line to a DAG.
//
// Guessing on multiple choice is the risk, so the test has a "don't know"
// answer, and a probe that would settle a large region below it needs two
// right answers. A wrongly placed idea is cheap to fix: its first review
// fails and FSRS sends it back.

import (
	"hash/fnv"
	"math/bits"
	"time"
)

const (
	placementMaxProbes = 48
	placementConfirm   = 25 // a right answer settling this many ideas below needs a second one
)

type bitset []uint64

func newBitset(n int) bitset    { return make(bitset, (n+63)/64) }
func (b bitset) set(i int)      { b[i/64] |= 1 << (i % 64) }
func (b bitset) has(i int) bool { return b[i/64]&(1<<(i%64)) != 0 }
func (b bitset) clear(i int)    { b[i/64] &^= 1 << (i % 64) }
func (b bitset) and(o bitset) (n int) {
	for k := range b {
		n += bits.OnesCount64(b[k] & o[k])
	}
	return n
}

// PlacementGraph holds the closures of one track, computed once.
type PlacementGraph struct {
	Codes      []string
	index      map[string]int
	below      []bitset // prerequisite closure, including the idea itself
	above      []bitset // dependents closure, including the idea itself
	selectable []bool   // has questions
}

func NewPlacementGraph(ideas []FIREIdea) *PlacementGraph {
	g := &PlacementGraph{index: map[string]int{}}
	for _, i := range ideas {
		g.index[i.Code] = len(g.Codes)
		g.Codes = append(g.Codes, i.Code)
	}
	n := len(g.Codes)
	pre := make([][]int, n)
	for _, i := range ideas {
		for _, e := range i.Prerequisites {
			if j, ok := g.index[e.Code]; ok {
				pre[g.index[i.Code]] = append(pre[g.index[i.Code]], j)
			}
		}
	}
	g.below = make([]bitset, n)
	var down func(i int) bitset
	down = func(i int) bitset {
		if g.below[i] != nil {
			return g.below[i]
		}
		b := newBitset(n)
		b.set(i)
		g.below[i] = b // the graph is acyclic; set early as a guard anyway
		for _, j := range pre[i] {
			for k, w := range down(j) {
				b[k] |= w
			}
		}
		return b
	}
	g.above = make([]bitset, n)
	for i := range g.above {
		g.above[i] = newBitset(n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if down(i).has(j) {
				g.above[j].set(i)
			}
		}
	}
	g.selectable = make([]bool, n)
	for i := range g.selectable {
		g.selectable[i] = true
	}
	return g
}

// Placement is one learner's test in progress.
type Placement struct {
	g       *PlacementGraph
	open    bitset // unsettled ideas
	Known   map[string]bool
	Direct  map[string]bool // answered right themselves (not inferred)
	Probes  int
	pending string // idea waiting for a confirming second answer
}

// NewPlacement starts a test; ideas already learned are settled as known.
func NewPlacement(g *PlacementGraph, learned map[string]bool) *Placement {
	p := &Placement{g: g, open: newBitset(len(g.Codes)), Known: map[string]bool{}, Direct: map[string]bool{}}
	for i, code := range g.Codes {
		if !learned[code] {
			p.open.set(i)
		}
	}
	return p
}

// Next returns the idea to probe, or "" when the test is over.
func (p *Placement) Next() string {
	if p.pending != "" {
		return p.pending
	}
	if p.Probes >= placementMaxProbes {
		return ""
	}
	best, score := -1, 0
	for i := range p.g.Codes {
		if !p.open.has(i) || !p.g.selectable[i] {
			continue
		}
		s := min(p.g.below[i].and(p.open), p.g.above[i].and(p.open))
		if s > score {
			best, score = i, s
		}
	}
	// Only singletons left: each probe settles one idea — not worth the
	// learner's time; lessons will sort them out.
	if best < 0 || score < 2 {
		return ""
	}
	return p.g.Codes[best]
}

// Record applies an answer to the probe returned by Next.
func (p *Placement) Record(code string, right bool) {
	i, ok := p.g.index[code]
	if !ok || !p.open.has(i) {
		return
	}
	p.Probes++
	if right && p.pending == "" && p.g.below[i].and(p.open) >= placementConfirm {
		p.pending = code // ask once more before settling a large region
		return
	}
	p.pending = ""
	if right {
		p.Direct[code] = true
		for j := range p.g.Codes {
			if p.g.below[i].has(j) && p.open.has(j) {
				p.open.clear(j)
				p.Known[p.g.Codes[j]] = true
			}
		}
		return
	}
	for j := range p.g.Codes {
		if p.g.above[i].has(j) {
			p.open.clear(j)
		}
	}
}

// Placed is the starting memory of an idea the test settled as known: as if
// reviewed now with a stability of 10–30 days (an idea answered directly gets
// 30), spread by code so the first checks do not all land on the same day.
func Placed(code string, direct bool, now time.Time) Memory {
	h := fnv.New32a()
	h.Write([]byte(code))
	s := 10 + float64(h.Sum32()%15)
	if direct {
		s = 30
	}
	m := Memory{Status: "learned", S: s, D: 5, Reps: 1, LastReviewAt: now}
	m.anchor(now)
	m.schedule()
	return m
}
