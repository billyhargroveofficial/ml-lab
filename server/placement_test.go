package main

import "testing"

// On a chain the test is a binary search: a learner who knows the first 37 of
// 100 ideas is placed at (almost) exactly 37 in about a dozen probes.
func TestPlacementChain(t *testing.T) {
	ideas := make([]FIREIdea, 100)
	for i := range ideas {
		ideas[i] = FIREIdea{Code: string(rune('A'+i/26)) + string(rune('a'+i%26)), ID: "x"}
		if i > 0 {
			ideas[i].Prerequisites = []FIREEdge{{Code: ideas[i-1].Code, Encompassing: 1}}
		}
	}
	know := map[string]bool{}
	for i := 0; i < 37; i++ {
		know[ideas[i].Code] = true
	}
	p := NewPlacement(NewPlacementGraph(ideas), nil)
	for code := p.Next(); code != ""; code = p.Next() {
		p.Record(code, know[code])
	}
	for code := range p.Known {
		if !know[code] {
			t.Fatalf("placed %s the learner does not know", code)
		}
	}
	if len(p.Known) < 35 || p.Probes > 16 {
		t.Fatalf("known %d after %d probes", len(p.Known), p.Probes)
	}
}
