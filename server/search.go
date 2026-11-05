package main

import (
	"sort"
	"strings"
	"unicode"
)

type SearchRow struct {
	Code       string  `json:"code"`
	Pack       string  `json:"pack"`
	Topic      string  `json:"topic"`
	Name       string  `json:"name"`
	Definition string  `json:"definition,omitempty"`
	Level      int     `json:"level"`
	Questions  int     `json:"questions"`
	Score      float64 `json:"score,omitempty"`
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Search ranks ideas by how many query words (or word prefixes of 4+ letters,
// which catches Russian endings) appear in name, definition, topic and code.
// An empty query lists everything in pack order.
func Search(c *Content, q, pack string) []SearchRow {
	qw := words(q)
	out := []SearchRow{}
	for _, id := range c.Order {
		if pack != "" && id != pack {
			continue
		}
		for _, i := range c.Program[id] {
			topic := ""
			if t := c.Topics[i.TopicID]; t != nil {
				topic = t.Name
			}
			score := 0.0
			if len(qw) > 0 {
				hay := map[string]float64{}
				for _, w := range words(i.Name) {
					hay[w] = max(hay[w], 3)
				}
				for _, w := range words(topic + " " + i.Code) {
					hay[w] = max(hay[w], 2)
				}
				for _, w := range words(i.Definition) {
					hay[w] = max(hay[w], 1)
				}
				for _, w := range qw {
					best := 0.0
					for h, s := range hay {
						if h == w || (len([]rune(w)) >= 4 && len([]rune(h)) >= 4 && strings.HasPrefix(h, string([]rune(w)[:4])) && strings.HasPrefix(w, string([]rune(h)[:4]))) {
							best = max(best, s)
						}
					}
					score += best
				}
				if score == 0 {
					continue
				}
			}
			out = append(out, SearchRow{i.Code, i.Program, topic, i.Name, i.Definition, i.Level, len(i.Questions), score})
		}
	}
	if len(qw) > 0 {
		sort.SliceStable(out, func(a, b int) bool { return out[a].Score > out[b].Score })
	}
	return out
}
