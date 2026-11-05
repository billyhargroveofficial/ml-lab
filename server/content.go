package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

// Content is the whole read-only program, loaded once from content.db.
type Section struct {
	ID          string   `json:"id"`
	Program     string   `json:"program"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Interviews  int      `json:"interviews"`
	Topics      []*Topic `json:"topics"`
}

type Topic struct {
	ID         string   `json:"id"`
	SectionID  string   `json:"section_id"`
	Code       string   `json:"code,omitempty"`
	Name       string   `json:"name"`
	Definition string   `json:"definition,omitempty"`
	Interviews int      `json:"interviews"`
	Ideas      []string `json:"ideas"`
}

type Share struct {
	K        int     `json:"k"`
	Share    float64 `json:"share"`
	Low      float64 `json:"low"`
	High     float64 `json:"high"`
	Frequent bool    `json:"frequent"`
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

type Formulation struct {
	Text   string          `json:"text"`
	Counts json.RawMessage `json:"counts,omitempty"`
}

type Idea struct {
	Code         string           `json:"code"`
	ID           string           `json:"id"`
	Program      string           `json:"program"`
	TopicID      string           `json:"topic_id"`
	Name         string           `json:"name"`
	Definition   string           `json:"definition,omitempty"`
	Level        int              `json:"level"`
	Weight       float64          `json:"weight"`
	Interviews   int              `json:"interviews"`
	Lesson       string           `json:"lesson,omitempty"`
	Shares       map[string]Share `json:"shares,omitempty"`
	Formulations []Formulation    `json:"formulations,omitempty"`
	Questions    []Question       `json:"questions,omitempty"`
	Edges        []Edge           `json:"prerequisites"`
	EncompassedBy []Edge          `json:"encompassed_by"`
	order        int
}

type MarketTrack struct {
	Name       string  `json:"name"`
	Interviews float64 `json:"interviews"`
	Unseen     float64 `json:"unseen"`
	Curve      struct {
		Job          []float64 `json:"job"`
		PerInterview []float64 `json:"per_interview"`
	} `json:"curve"`
	Thresholds struct {
		O1 *int `json:"o1"`
	} `json:"thresholds"`
	Order []struct {
		Code  string `json:"code"`
		Asked int    `json:"asked"`
	} `json:"order"`
}

type Content struct {
	Info      json.RawMessage
	MarketRaw json.RawMessage
	Market    struct {
		Tracks map[string]MarketTrack `json:"tracks"`
	}
	Sections map[string][]*Section // by program
	Ideas    map[string]*Idea      // by code
	ByID     map[string]*Idea
	Program  map[string][]*Idea // by program, in order
	Graph    map[string]FIREIdea // full graph incl. implicit edges, for propagation
	Asked    map[string]map[string]float64 // track → code → interview count
}

func LoadContent(path string) (*Content, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	c := &Content{Sections: map[string][]*Section{}, Ideas: map[string]*Idea{}, ByID: map[string]*Idea{}, Program: map[string][]*Idea{}, Graph: map[string]FIREIdea{}, Asked: map[string]map[string]float64{}}
	var info, market string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='info'`).Scan(&info); err != nil {
		return nil, fmt.Errorf("meta: %w", err)
	}
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='market'`).Scan(&market); err != nil {
		return nil, err
	}
	c.Info, c.MarketRaw = json.RawMessage(info), json.RawMessage(market)
	if err := json.Unmarshal(c.MarketRaw, &c.Market); err != nil {
		return nil, err
	}
	sections := map[string]*Section{}
	rows, err := db.Query(`SELECT id, program, name, coalesce(description,''), coalesce(interviews,0) FROM sections ORDER BY program, ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		s := &Section{Topics: []*Topic{}}
		if err := rows.Scan(&s.ID, &s.Program, &s.Name, &s.Description, &s.Interviews); err != nil {
			return nil, err
		}
		sections[s.ID] = s
		c.Sections[s.Program] = append(c.Sections[s.Program], s)
	}
	topics := map[string]*Topic{}
	rows, err = db.Query(`SELECT id, section_id, coalesce(code,''), name, coalesce(definition,''), coalesce(interviews,0) FROM topics ORDER BY program, ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t := &Topic{Ideas: []string{}}
		if err := rows.Scan(&t.ID, &t.SectionID, &t.Code, &t.Name, &t.Definition, &t.Interviews); err != nil {
			return nil, err
		}
		topics[t.ID] = t
		sections[t.SectionID].Topics = append(sections[t.SectionID].Topics, t)
	}
	rows, err = db.Query(`SELECT code, id, program, topic_id, ord, name, coalesce(definition,''), level, weight, interviews, lesson FROM ideas ORDER BY program, ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		i := &Idea{Shares: map[string]Share{}, Edges: []Edge{}, EncompassedBy: []Edge{}}
		if err := rows.Scan(&i.Code, &i.ID, &i.Program, &i.TopicID, &i.order, &i.Name, &i.Definition, &i.Level, &i.Weight, &i.Interviews, &i.Lesson); err != nil {
			return nil, err
		}
		c.Ideas[i.Code], c.ByID[i.ID] = i, i
		c.Program[i.Program] = append(c.Program[i.Program], i)
		topics[i.TopicID].Ideas = append(topics[i.TopicID].Ideas, i.Code)
	}
	rows, err = db.Query(`SELECT code, track, k, share, low, high, frequent FROM interview_shares`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code, track string
		var s Share
		if err := rows.Scan(&code, &track, &s.K, &s.Share, &s.Low, &s.High, &s.Frequent); err != nil {
			return nil, err
		}
		c.Ideas[code].Shares[track] = s
	}
	rows, err = db.Query(`SELECT code, text, coalesce(counts,'{}') FROM formulations ORDER BY code, ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code string
		var f Formulation
		var counts string
		if err := rows.Scan(&code, &f.Text, &counts); err != nil {
			return nil, err
		}
		f.Counts = json.RawMessage(counts)
		c.Ideas[code].Formulations = append(c.Ideas[code].Formulations, f)
	}
	rows, err = db.Query(`SELECT code, question, options, correct, coalesce(explanation,''), coalesce(difficulty,0) FROM questions ORDER BY code, ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code, options string
		var q Question
		if err := rows.Scan(&code, &q.Question, &options, &q.Correct, &q.Explanation, &q.Difficulty); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(options), &q.Options); err != nil {
			return nil, err
		}
		c.Ideas[code].Questions = append(c.Ideas[code].Questions, q)
	}
	rows, err = db.Query(`SELECT code, prereq, key, encompassing, kind, source FROM edges`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code string
		var e Edge
		if err := rows.Scan(&code, &e.Code, &e.Key, &e.Encompassing, &e.Kind, &e.Source); err != nil {
			return nil, err
		}
		c.Ideas[code].Edges = append(c.Ideas[code].Edges, e)
		back := e
		back.Code = code
		c.Ideas[e.Code].EncompassedBy = append(c.Ideas[e.Code].EncompassedBy, back)
	}
	for code, i := range c.Ideas {
		sort.Slice(i.Edges, func(a, b int) bool { return i.Edges[a].Encompassing > i.Edges[b].Encompassing })
		sort.Slice(i.EncompassedBy, func(a, b int) bool { return i.EncompassedBy[a].Encompassing > i.EncompassedBy[b].Encompassing })
		g := FIREIdea{Code: code, ID: i.ID, Level: i.Level, Weight: i.Weight}
		for _, e := range i.Edges {
			g.Prerequisites = append(g.Prerequisites, FIREEdge{Code: e.Code, Key: e.Key, Encompassing: e.Encompassing})
		}
		c.Graph[code] = g
	}
	for name, t := range c.Market.Tracks {
		m := map[string]float64{}
		for _, o := range t.Order {
			m[o.Code] = float64(o.Asked)
		}
		c.Asked[name] = m
	}
	return c, nil
}

// Tracks: the ML program is weighted per interview track, math by its own weight.
var tracks = map[string]bool{"all": true, "ds": true, "ml": true, "ai": true, "math": true}

func programOf(track string) string {
	if track == "math" {
		return "math"
	}
	return "ml"
}

// weight is the scheduling and coverage weight of an idea in a track.
func (c *Content) weight(i *Idea, track string) float64 {
	if track == "math" {
		return i.Weight
	}
	return c.Asked[track][i.Code]
}

// schedulingIdeas is the program as FIRe sees it for next tasks: only blocking
// prerequisites of the same program. Implicit cross-program edges never lock a
// lesson; they only spread repetition credit (see Graph).
func (c *Content) schedulingIdeas(track string) []FIREIdea {
	ideas := c.Program[programOf(track)]
	out := make([]FIREIdea, 0, len(ideas))
	for _, i := range ideas {
		if len(i.Questions) == 0 {
			continue
		}
		g := FIREIdea{Code: i.Code, ID: i.ID, Level: i.Level, Weight: c.weight(i, track)}
		for _, e := range i.Edges {
			if e.Kind == "prerequisite" {
				g.Prerequisites = append(g.Prerequisites, FIREEdge{Code: e.Code, Key: e.Key, Encompassing: e.Encompassing})
			}
		}
		out = append(out, g)
	}
	return out
}
