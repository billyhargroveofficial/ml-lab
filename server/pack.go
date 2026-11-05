package main

// Packs: the only source of content. A pack is a folder packs/<id>/ with a
// pack.json and, optionally, lessons as Markdown files next to it. The server
// loads every pack at start and again whenever the folder changes; progress
// lives separately in a local SQLite file and is keyed by idea code, so
// editing a pack never loses progress on the ideas that stay.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const PackFormat = "fire-pack-v1"

type PackFile struct {
	Format      string      `json:"format"`
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Language    string      `json:"language,omitempty"`
	Description string      `json:"description,omitempty"`
	Topics      []PackTopic `json:"topics"`
}

type PackTopic struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Definition string     `json:"definition,omitempty"`
	Ideas      []PackIdea `json:"ideas"`
}

type PackIdea struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Definition string     `json:"definition,omitempty"`
	Level      int        `json:"level"`
	Lesson     string     `json:"lesson,omitempty"`
	LessonFile string     `json:"lesson_file,omitempty"`
	Requires   []PackLink `json:"requires,omitempty"`
	Uses       []PackLink `json:"uses,omitempty"`
	Questions  []Question `json:"questions"`
}

type PackLink struct {
	Code string  `json:"code"`
	W    float64 `json:"w"`
	Key  bool    `json:"key,omitempty"`
}

// PackStatus is what /api/v1/packs and `lab validate` report per pack.
type PackStatus struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Dir      string   `json:"dir"`
	Ideas    int      `json:"ideas"`
	Topics   int      `json:"topics"`
	Loaded   bool     `json:"loaded"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// IdeaCode is the permanent address of an idea: <pack>.<topic>/<idea>.
func IdeaCode(pack, topic, idea string) string { return pack + "." + topic + "/" + idea }

// TopicCode addresses a whole topic (used by pins): <pack>.<topic>.
func TopicCode(pack, topic string) string { return pack + "." + topic }

func readPack(dir string) (*PackFile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "pack.json"))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p PackFile
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("pack.json: %v", err)
	}
	return &p, nil
}

// checkPack validates one pack on its own (structure, lessons, questions).
// Links to other packs are checked later, against everything that loaded.
func checkPack(dir string, p *PackFile) (errs, warns []string) {
	e := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	w := func(f string, a ...any) { warns = append(warns, fmt.Sprintf(f, a...)) }
	if p.Format != PackFormat {
		e("format must be %q", PackFormat)
	}
	if !slugRe.MatchString(p.ID) || len(p.ID) > 40 {
		e("id %q: lowercase latin letters, digits and dashes, up to 40", p.ID)
	}
	if base := filepath.Base(dir); p.ID != base {
		e("id %q must equal the folder name %q", p.ID, base)
	}
	if strings.TrimSpace(p.Title) == "" {
		e("title is empty")
	}
	if len(p.Topics) == 0 {
		e("no topics")
	}
	seenT, seenI := map[string]bool{}, map[string]bool{}
	for _, t := range p.Topics {
		where := "topic " + t.ID
		if !slugRe.MatchString(t.ID) {
			e("%s: id must be a slug", where)
		}
		if seenT[t.ID] {
			e("%s: duplicate topic id", where)
		}
		seenT[t.ID] = true
		if strings.TrimSpace(t.Name) == "" {
			e("%s: name is empty", where)
		}
		if len(t.Ideas) == 0 {
			e("%s: no ideas", where)
		}
		for _, i := range t.Ideas {
			where := IdeaCode(p.ID, t.ID, i.ID)
			if !slugRe.MatchString(i.ID) {
				e("%s: idea id must be a slug", where)
			}
			if seenI[t.ID+"/"+i.ID] {
				e("%s: duplicate idea", where)
			}
			seenI[t.ID+"/"+i.ID] = true
			if strings.TrimSpace(i.Name) == "" {
				e("%s: name is empty", where)
			}
			if i.Level < 1 || i.Level > 3 {
				e("%s: level must be 1, 2 or 3", where)
			}
			lesson, err := lessonText(dir, i)
			if err != nil {
				e("%s: %v", where, err)
			} else {
				if n := len([]rune(lesson)); n < 300 {
					e("%s: lesson is %d characters, needs at least 300", where, n)
				} else if n > 8000 {
					w("%s: lesson is %d characters; over 8000 is hard to finish in one sitting", where, n)
				}
				for _, m := range checkMath(lesson) {
					e("%s: lesson: %s", where, m)
				}
			}
			if n := len(i.Questions); n < 3 || n > 8 {
				e("%s: needs 3–8 questions, has %d", where, n)
			}
			for qi, q := range i.Questions {
				qw := fmt.Sprintf("%s: question %d", where, qi+1)
				if strings.TrimSpace(q.Question) == "" {
					e("%s: text is empty", qw)
				}
				if len(q.Options) < 3 || len(q.Options) > 6 {
					e("%s: needs 3–6 options", qw)
				}
				if q.Correct < 0 || q.Correct >= len(q.Options) {
					e("%s: correct=%d is not an option index (0-based)", qw, q.Correct)
				}
				seen := map[string]bool{}
				for _, o := range q.Options {
					k := strings.TrimSpace(strings.ToLower(o))
					if k == "" || seen[k] {
						e("%s: options must be distinct and non-empty", qw)
					}
					seen[k] = true
				}
				if strings.TrimSpace(q.Explanation) == "" {
					e("%s: explanation is empty", qw)
				}
				if q.Difficulty < 0 || q.Difficulty > 3 {
					e("%s: difficulty must be 1–3", qw)
				}
				for _, s := range append([]string{q.Question, q.Explanation}, q.Options...) {
					for _, m := range checkMath(s) {
						e("%s: %s", qw, m)
					}
				}
			}
			for _, l := range append(append([]PackLink{}, i.Requires...), i.Uses...) {
				if l.W < 0.05 || l.W > 1 {
					e("%s: link to %s: w must be 0.05–1", where, l.Code)
				}
			}
			if len(i.Questions) > 0 {
				first := i.Questions[0].Correct
				same := true
				for _, q := range i.Questions {
					same = same && q.Correct == first
				}
				if same && len(i.Questions) >= 3 {
					w("%s: every correct answer is option %d — shuffle them", where, first)
				}
			}
		}
	}
	return errs, warns
}

func lessonText(dir string, i PackIdea) (string, error) {
	if i.LessonFile == "" {
		if strings.TrimSpace(i.Lesson) == "" {
			return "", fmt.Errorf("no lesson (set lesson or lesson_file)")
		}
		return i.Lesson, nil
	}
	if i.Lesson != "" {
		return "", fmt.Errorf("set lesson or lesson_file, not both")
	}
	file := filepath.Join(dir, filepath.FromSlash(i.LessonFile))
	if rel, err := filepath.Rel(dir, file); err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(i.LessonFile) {
		return "", fmt.Errorf("lesson_file must stay inside the pack folder")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("lesson_file: %v", err)
	}
	return string(raw), nil
}

// checkMath catches the LaTeX mistakes models make most: unbalanced $ and
// control characters left by a JSON string with "\frac" instead of "\\frac".
func checkMath(s string) []string {
	var out []string
	for _, r := range s {
		if r == '\f' || r == '\b' || r == '\v' || r == '\a' {
			out = append(out, `control character in text — in JSON write "\\frac", not "\frac"`)
			break
		}
	}
	plain := strings.ReplaceAll(s, `\$`, "")
	// Code spans and blocks may hold a literal $.
	plain = regexp.MustCompile("(?s)```.*?```|`[^`]*`").ReplaceAllString(plain, "")
	if strings.Count(plain, "$")%2 != 0 {
		out = append(out, "unbalanced $ in math")
	}
	return out
}

// LoadPacks reads every packs/<id>/pack.json, builds the content from those
// that pass, and reports every pack. A broken pack is skipped, and so are
// packs that link to it.
func LoadPacks(dir string) (*Content, []PackStatus) {
	entries, _ := os.ReadDir(dir)
	type loaded struct {
		dir string
		p   *PackFile
	}
	var packs []loaded
	var status []PackStatus
	idx := map[string]int{}
	for _, en := range entries {
		if !en.IsDir() || strings.HasPrefix(en.Name(), ".") || strings.HasPrefix(en.Name(), "_") {
			continue
		}
		d := filepath.Join(dir, en.Name())
		st := PackStatus{ID: en.Name(), Dir: d}
		p, err := readPack(d)
		if err != nil {
			st.Errors = []string{err.Error()}
		} else {
			st.Title = p.Title
			st.Errors, st.Warnings = checkPack(d, p)
			for _, t := range p.Topics {
				st.Topics++
				st.Ideas += len(t.Ideas)
			}
		}
		idx[en.Name()] = len(status)
		status = append(status, st)
		if len(st.Errors) == 0 {
			packs = append(packs, loaded{d, p})
		}
	}
	// Links must point at ideas of packs that loaded; drop packs until stable.
	for {
		codes := map[string]string{}
		for _, l := range packs {
			for _, t := range l.p.Topics {
				for _, i := range t.Ideas {
					codes[IdeaCode(l.p.ID, t.ID, i.ID)] = l.p.ID
				}
			}
		}
		var keep []loaded
		for _, l := range packs {
			var errs []string
			for _, t := range l.p.Topics {
				for _, i := range t.Ideas {
					from := IdeaCode(l.p.ID, t.ID, i.ID)
					for _, link := range append(append([]PackLink{}, i.Requires...), i.Uses...) {
						if link.Code == from {
							errs = append(errs, fmt.Sprintf("%s: links to itself", from))
						} else if codes[link.Code] == "" {
							errs = append(errs, fmt.Sprintf("%s: link to unknown idea %s%s", from, link.Code, suggest(link.Code, codes)))
						}
					}
				}
			}
			if len(errs) == 0 {
				keep = append(keep, l)
			} else {
				s := &status[idx[l.p.ID]]
				s.Errors = append(s.Errors, errs...)
			}
		}
		if len(keep) == len(packs) {
			break
		}
		packs = keep
	}
	files := make([]*PackFile, 0, len(packs))
	dirs := map[string]string{}
	for _, l := range packs {
		files = append(files, l.p)
		dirs[l.p.ID] = l.dir
	}
	c, cycle := buildContent(files, dirs)
	if cycle != "" {
		// A cycle spans packs; refuse all packs on it.
		for _, id := range strings.Split(cycle, " ") {
			if i, ok := idx[strings.SplitN(id, ".", 2)[0]]; ok {
				status[i].Errors = append(status[i].Errors, "requires form a cycle: "+cycle)
			}
		}
		var keep []*PackFile
		for _, f := range files {
			if !strings.Contains(" "+cycle, " "+f.ID+".") {
				keep = append(keep, f)
			}
		}
		c, _ = buildContent(keep, dirs)
	}
	for i := range status {
		_, ok := c.Packs[status[i].ID]
		status[i].Loaded = ok && len(status[i].Errors) == 0
	}
	return c, status
}

func suggest(code string, codes map[string]string) string {
	best, bestD := "", 1<<30
	for c := range codes {
		if d := editDistance(code, c); d < bestD {
			best, bestD = c, d
		}
	}
	if best == "" || bestD > len(code)/3 {
		return ""
	}
	return " (did you mean " + best + "?)"
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// findCycle returns the codes of one cycle in the requires graph, or "".
func findCycle(req map[string][]string) string {
	state := map[string]int{} // 1 visiting, 2 done
	var stack []string
	var found []string
	var visit func(string) bool
	visit = func(n string) bool {
		state[n] = 1
		stack = append(stack, n)
		for _, m := range req[n] {
			if state[m] == 1 {
				for i, s := range stack {
					if s == m {
						found = append([]string{}, stack[i:]...)
					}
				}
				return true
			}
			if state[m] == 0 && visit(m) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = 2
		return false
	}
	keys := make([]string, 0, len(req))
	for k := range req {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if state[k] == 0 && visit(k) {
			return strings.Join(found, " ")
		}
	}
	return ""
}
