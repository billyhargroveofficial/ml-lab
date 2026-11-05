package main

import (
	"sort"
	"strings"
)

// Content is everything the packs define, rebuilt whenever they change.
type Section struct {
	ID          string   `json:"id"`
	Program     string   `json:"program"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Topics      []*Topic `json:"topics"`
}

type Topic struct {
	ID         string   `json:"id"`
	SectionID  string   `json:"section_id"`
	Code       string   `json:"code,omitempty"`
	Name       string   `json:"name"`
	Definition string   `json:"definition,omitempty"`
	Ideas      []string `json:"ideas"`
}

type Question struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Correct     int      `json:"correct"`
	Explanation string   `json:"explanation,omitempty"`
	Difficulty  int      `json:"difficulty,omitempty"`
}

type Edge struct {
	Code         string  `json:"code"`
	Key          bool    `json:"key"`
	Encompassing float64 `json:"encompassing"`
	Kind         string  `json:"kind"`
	Source       string  `json:"source"`
}

type Idea struct {
	Code          string     `json:"code"`
	ID            string     `json:"id"`
	Program       string     `json:"program"`
	TopicID       string     `json:"topic_id"`
	Name          string     `json:"name"`
	Definition    string     `json:"definition,omitempty"`
	Level         int        `json:"level"`
	Weight        float64    `json:"weight"`
	Lesson        string     `json:"lesson,omitempty"`
	Questions     []Question `json:"questions,omitempty"`
	Edges         []Edge     `json:"prerequisites"`
	EncompassedBy []Edge     `json:"encompassed_by"`
	order         int
}

type PackInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Ideas       int    `json:"ideas"`
	Topics      int    `json:"topics"`
}

type Content struct {
	Packs    map[string]*PackInfo
	Order    []string              // pack ids, sorted
	Sections map[string][]*Section // by program
	Ideas    map[string]*Idea      // by code
	Topics   map[string]*Topic     // by id
	ByID     map[string]*Idea
	Program  map[string][]*Idea    // by program, in order
	Graph    map[string]FIREIdea   // full graph incl. implicit edges, for propagation
	Up       map[string][]FIREEdge // reverse edges: idea → ideas built on it
	Track    map[string][]*Idea    // ideas of each track, in program order
}

// buildContent turns validated packs into the graph. Each pack is one program
// with one section; its topics and ideas keep their order. It returns the
// codes of a requires-cycle if there is one (the caller drops those packs).
func buildContent(packs []*PackFile, dirs map[string]string) (*Content, string) {
	c := &Content{Packs: map[string]*PackInfo{}, Sections: map[string][]*Section{}, Ideas: map[string]*Idea{}, Topics: map[string]*Topic{},
		ByID: map[string]*Idea{}, Up: map[string][]FIREEdge{}, Program: map[string][]*Idea{}, Graph: map[string]FIREIdea{}, Track: map[string][]*Idea{}}
	sort.Slice(packs, func(a, b int) bool { return packs[a].ID < packs[b].ID })
	order := 0
	type link struct {
		from string
		l    PackLink
		kind string
	}
	var links []link
	for _, p := range packs {
		info := &PackInfo{ID: p.ID, Title: p.Title, Description: p.Description, Language: p.Language}
		c.Packs[p.ID] = info
		c.Order = append(c.Order, p.ID)
		sec := &Section{ID: p.ID, Program: p.ID, Name: p.Title, Description: p.Description, Topics: []*Topic{}}
		c.Sections[p.ID] = []*Section{sec}
		for _, t := range p.Topics {
			top := &Topic{ID: TopicCode(p.ID, t.ID), SectionID: sec.ID, Code: TopicCode(p.ID, t.ID), Name: t.Name, Definition: t.Definition, Ideas: []string{}}
			sec.Topics = append(sec.Topics, top)
			c.Topics[top.ID] = top
			info.Topics++
			for _, pi := range t.Ideas {
				code := IdeaCode(p.ID, t.ID, pi.ID)
				lesson, _ := lessonText(dirs[p.ID], pi)
				i := &Idea{Code: code, ID: code, Program: p.ID, TopicID: top.ID, Name: pi.Name, Definition: pi.Definition, Level: pi.Level, Weight: 1,
					Lesson: lesson, Questions: pi.Questions, Edges: []Edge{}, EncompassedBy: []Edge{}, order: order}
				order++
				c.Ideas[code], c.ByID[code] = i, i
				c.Program[p.ID] = append(c.Program[p.ID], i)
				top.Ideas = append(top.Ideas, code)
				info.Ideas++
				for _, l := range pi.Requires {
					links = append(links, link{code, l, "prerequisite"})
				}
				for _, l := range pi.Uses {
					links = append(links, link{code, l, "implicit"})
				}
			}
		}
	}
	req := map[string][]string{}
	for _, l := range links {
		from, to := c.Ideas[l.from], c.Ideas[l.l.Code]
		if to == nil {
			continue
		}
		dup := false
		for _, e := range from.Edges {
			dup = dup || e.Code == to.Code
		}
		if dup { // requires wins over uses for the same pair
			continue
		}
		e := Edge{Code: to.Code, Key: l.l.Key, Encompassing: l.l.W, Kind: l.kind, Source: from.Program}
		from.Edges = append(from.Edges, e)
		back := e
		back.Code = from.Code
		to.EncompassedBy = append(to.EncompassedBy, back)
		if l.kind == "prerequisite" {
			req[from.Code] = append(req[from.Code], to.Code)
		}
	}
	if cycle := findCycle(req); cycle != "" {
		return c, cycle
	}
	for code, i := range c.Ideas {
		sort.SliceStable(i.Edges, func(a, b int) bool { return i.Edges[a].Encompassing > i.Edges[b].Encompassing })
		sort.SliceStable(i.EncompassedBy, func(a, b int) bool { return i.EncompassedBy[a].Encompassing > i.EncompassedBy[b].Encompassing })
		g := FIREIdea{Code: code, ID: i.ID, Level: i.Level, Weight: i.Weight}
		for _, e := range i.Edges {
			g.Prerequisites = append(g.Prerequisites, FIREEdge{Code: e.Code, Key: e.Key, Encompassing: e.Encompassing})
			c.Up[e.Code] = append(c.Up[e.Code], FIREEdge{Code: code, Key: e.Key, Encompassing: e.Encompassing})
		}
		c.Graph[code] = g
	}
	c.buildTracks()
	return c, ""
}

// buildTracks: "all" is every pack; each pack is also a track of its own —
// its ideas plus whatever they require from other packs, so the scheduler
// can teach the missing basics first.
func (c *Content) buildTracks() {
	var all []*Idea
	for _, id := range c.Order {
		all = append(all, c.Program[id]...)
	}
	c.Track["all"] = all
	for _, id := range c.Order {
		need := map[string]bool{}
		var walk func(code string)
		walk = func(code string) {
			if need[code] {
				return
			}
			need[code] = true
			for _, e := range c.Ideas[code].Edges {
				if e.Kind == "prerequisite" {
					walk(e.Code)
				}
			}
		}
		for _, i := range c.Program[id] {
			walk(i.Code)
		}
		var t []*Idea
		for _, i := range all {
			if need[i.Code] {
				t = append(t, i)
			}
		}
		c.Track[id] = t
	}
}

// HasTrack: "all" or a loaded pack id.
func (c *Content) HasTrack(t string) bool { _, ok := c.Track[t]; return ok }

// schedulingIdeas is a track as FIRe sees it for next tasks: requires edges
// gate lessons, uses edges only spread repetition credit (see Graph). Lessons
// come in pack order: earlier first.
func (c *Content) schedulingIdeas(track string) []FIREIdea {
	ideas := c.Track[track]
	out := make([]FIREIdea, 0, len(ideas))
	for _, i := range ideas {
		if len(i.Questions) == 0 {
			continue
		}
		g := FIREIdea{Code: i.Code, ID: i.ID, Level: i.Level, Weight: -float64(i.order)}
		for _, e := range i.Edges {
			if e.Kind == "prerequisite" {
				g.Prerequisites = append(g.Prerequisites, FIREEdge{Code: e.Code, Key: e.Key, Encompassing: e.Encompassing})
			}
		}
		out = append(out, g)
	}
	return out
}

// weight weighs progress: every idea counts the same.
func (c *Content) weight(_ *Idea, _ string) float64 { return 1 }

var _ = strings.TrimSpace
