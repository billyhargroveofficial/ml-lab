package main

import (
	"math"
	"testing"
	"time"
)

// Reference values from py-fsrs 6.3.2 (Scheduler without learning steps or
// fuzz) for the same sequence of ratings and gaps.
func TestFSRSMatchesPyFSRS(t *testing.T) {
	steps := []struct {
		days float64
		g    Rating
		s, d float64
	}{
		{0, Good, 2.3065, 2.118104},
		{3, Good, 13.826904, 2.111214},
		{12, Hard, 37.021367, 4.748285},
		{20, Again, 2.39858, 8.259025},
		{2, Good, 5.080083, 8.245995},
		{30, Easy, 34.421447, 7.645116},
		{0.2, Good, 34.421447, 7.632699},
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var m Memory
	for i, st := range steps {
		now = now.Add(time.Duration(st.days * float64(day)))
		if i == 0 {
			m = m.Learn(st.g, now)
		} else {
			m = m.Review(st.g, now)
		}
		if math.Abs(m.S-st.s) > 1e-4 || math.Abs(m.D-st.d) > 1e-4 {
			t.Fatalf("step %d: S=%.6f D=%.6f, want S=%.6f D=%.6f", i, m.S, m.D, st.s, st.d)
		}
	}
}

func TestRecallAtStabilityIsNinety(t *testing.T) {
	if r := Recall(10, 10); math.Abs(r-0.9) > 1e-9 {
		t.Fatalf("R(S)=%v", r)
	}
}

// Full implicit credit equals a Good review's stability; zero changes nothing.
func TestCreditInterpolatesToGoodReview(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := Memory{}.Learn(Good, now)
	later := now.Add(5 * day)
	full, gain := m.credit(1, later, false)
	good := m.Review(Good, later)
	if math.Abs(full.S-good.S) > 1e-9 {
		t.Fatalf("credit 1: S=%v, Good review S=%v", full.S, good.S)
	}
	if math.Abs(full.RecallNow(later)-1) > 1e-6 || math.Abs(gain-(1-m.RecallNow(later))) > 1e-9 {
		t.Fatalf("recall after full credit %v gain %v", full.RecallNow(later), gain)
	}
	if same, g := m.Credit(0, later); same != m || g != 0 {
		t.Fatal("zero credit changed memory")
	}
	half, _ := m.credit(0.5, later, false)
	if !(half.S > m.S && half.S < good.S) {
		t.Fatalf("half credit S=%v not between %v and %v", half.S, m.S, good.S)
	}
}

// Implicit credit alone cannot push stability past ImplicitCap × the anchor,
// and an idea living on credit becomes due for an explicit check.
func TestImplicitCapAndStaleCheck(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := Memory{}.Learn(Good, now)
	at := now
	for i := 0; i < 40; i++ {
		at = at.Add(time.Duration(IntervalDays(m.S)) * day)
		m, _ = m.Credit(1, at)
	}
	if limit := ImplicitCap * m.AnchorS; m.S > limit+1e-9 {
		t.Fatalf("S=%v above cap %v", m.S, limit)
	}
	check := m.AnchorAt.Add(time.Duration(IntervalDays(ImplicitCap*m.AnchorS) * float64(day)))
	if !m.Due(check) || !m.Stale(check) || m.DueAt.After(check) {
		t.Fatalf("not due for an explicit check at %v (due %v)", check, m.DueAt)
	}
	r := m.Review(Good, check)
	if r.Stale(check) || r.AnchorS != r.S {
		t.Fatalf("explicit review did not re-anchor: %+v", r)
	}
}
