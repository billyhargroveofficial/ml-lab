package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// API serves one local learner. There are no accounts: the server listens on
// localhost and refuses other Host headers (DNS rebinding) and cross-site
// writes (a custom header a plain form cannot send).
type API struct {
	lab  *Lab
	mu   sync.RWMutex // content swaps take the write lock
	pack []PackStatus
}

// route is one endpoint; the same table feeds the mux and docs/api.md.
type route struct {
	Method, Path, Summary string
	h                     http.HandlerFunc
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func localHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// guard wraps every API call: local Host only, the custom header on writes,
// and a read lock so a pack reload never swaps content mid-request.
func (a *API) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) {
			fail(w, 403, "local only")
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "fire" {
			fail(w, 403, "missing X-Requested-With: fire")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		a.mu.RLock()
		defer a.mu.RUnlock()
		h(w, r)
	}
}

// Swap installs freshly loaded packs.
func (a *API) Swap(c *Content, status []PackStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lab.c = c
	a.pack = status
	a.lab.placeMu.Lock()
	a.lab.graphs = nil
	a.lab.placeMu.Unlock()
}

func (a *API) track(r *http.Request) string {
	t := r.URL.Query().Get("track")
	if !a.lab.c.HasTrack(t) {
		return "all"
	}
	return t
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		fail(w, 400, "bad json")
		return false
	}
	return true
}

func (a *API) routes() []route {
	l := a.lab
	return []route{
		{"GET", "/info", "Loaded packs and how many ideas each has.", func(w http.ResponseWriter, r *http.Request) {
			packs := []*PackInfo{}
			for _, id := range l.c.Order {
				packs = append(packs, l.c.Packs[id])
			}
			writeJSON(w, 200, map[string]any{"packs": packs, "ideas": len(l.c.Ideas)})
		}},
		{"GET", "/packs", "Every pack folder with its status, errors and warnings.", func(w http.ResponseWriter, r *http.Request) {
			out := a.pack
			if out == nil {
				out = []PackStatus{}
			}
			writeJSON(w, 200, out)
		}},
		{"GET", "/program/{pack}", "One pack as a tree: section → topics → idea codes, plus a short card per idea.", func(w http.ResponseWriter, r *http.Request) {
			prog := r.PathValue("pack")
			secs, ok := l.c.Sections[prog]
			if !ok {
				fail(w, 404, "unknown pack")
				return
			}
			type ideaRow struct {
				Code  string `json:"code"`
				Name  string `json:"name"`
				Level int    `json:"level"`
			}
			ideas := map[string]ideaRow{}
			for _, i := range l.c.Program[prog] {
				ideas[i.Code] = ideaRow{i.Code, i.Name, i.Level}
			}
			writeJSON(w, 200, map[string]any{"program": prog, "title": l.c.Packs[prog].Title, "sections": secs, "ideas": ideas})
		}},
		{"GET", "/ideas", "Search ideas: ?q= text, ?pack=, ?limit= (≤200), ?offset=.", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query().Get("q")
			res := Search(l.c, q, r.URL.Query().Get("pack"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			if limit <= 0 || limit > 200 {
				limit = 50
			}
			total := len(res)
			res = res[min(offset, total):min(offset+limit, total)]
			writeJSON(w, 200, map[string]any{"total": total, "items": res})
		}},
		{"GET", "/graph", "Every idea as a graph node: [{code, name, pack, topic, topic_name}]; edges come from /edges.", func(w http.ResponseWriter, r *http.Request) {
			type node struct {
				Code      string `json:"code"`
				Name      string `json:"name"`
				Pack      string `json:"pack"`
				Topic     string `json:"topic"`
				TopicName string `json:"topic_name"`
			}
			out := []node{}
			for _, id := range l.c.Order {
				for _, i := range l.c.Program[id] {
					tn := ""
					if t := l.c.Topics[i.TopicID]; t != nil {
						tn = t.Name
					}
					out = append(out, node{i.Code, i.Name, i.Program, i.TopicID, tn})
				}
			}
			writeJSON(w, 200, out)
		}},
		{"GET", "/names", "Every idea code with its title: {code: name}.", func(w http.ResponseWriter, r *http.Request) {
			out := map[string]string{}
			for code, i := range l.c.Ideas {
				out[code] = i.Name
			}
			writeJSON(w, 200, out)
		}},
		{"GET", "/ideas/{code...}", "One idea: lesson, questions with answers, links both ways.", func(w http.ResponseWriter, r *http.Request) {
			i := l.c.Ideas[r.PathValue("code")]
			if i == nil {
				fail(w, 404, "unknown idea")
				return
			}
			writeJSON(w, 200, i)
		}},
		{"GET", "/edges", "Every link as {from, code, key, encompassing, kind}; ?kind=prerequisite|implicit.", func(w http.ResponseWriter, r *http.Request) {
			kind := r.URL.Query().Get("kind")
			type row struct {
				From string `json:"from"`
				Edge
			}
			out := []row{}
			for _, id := range l.c.Order {
				for _, i := range l.c.Program[id] {
					for _, e := range i.Edges {
						if kind == "" || kind == e.Kind {
							out = append(out, row{i.Code, e})
						}
					}
				}
			}
			writeJSON(w, 200, out)
		}},
		{"GET", "/me", "Progress on a track (?track=all|<pack>): learned, due, XP by day, memory of each idea, forecast.", func(w http.ResponseWriter, r *http.Request) {
			me, err := l.Me(r.Context(), localUser, "local", a.track(r))
			a.respond(w, me, err)
		}},
		{"GET", "/next", "The FIRe feed for a track: up to 8 tasks {kind, code, title, xp, covers}.", func(w http.ResponseWriter, r *http.Request) {
			out, err := l.Next(r.Context(), localUser, a.track(r))
			a.respond(w, out, err)
		}},
		{"GET", "/plan", "Every lesson of a track in teaching order with its state: learned, due, available, partial or locked (and what it waits for).", func(w http.ResponseWriter, r *http.Request) {
			out, err := l.Plan(r.Context(), localUser, a.track(r))
			a.respond(w, out, err)
		}},
		{"GET", "/history", "The last 40 finished tasks.", func(w http.ResponseWriter, r *http.Request) {
			out, err := l.History(r.Context(), localUser)
			a.respond(w, out, err)
		}},
		{"GET", "/pin", "The current goal, if any: {pin: {code, kind, title, left, next} | null}.", func(w http.ResponseWriter, r *http.Request) {
			pin, err := l.GetPin(r.Context(), localUser)
			a.respond(w, map[string]any{"pin": pin}, err)
		}},
		{"POST", "/pin", `Set a goal {"code": idea or topic code}; an empty code clears it. The next lesson on the way to it leads the feed.`, func(w http.ResponseWriter, r *http.Request) {
			var body struct{ Code string }
			if !decode(w, r, &body) {
				return
			}
			if err := l.SetPin(r.Context(), localUser, body.Code); err != nil {
				a.respond(w, nil, err)
				return
			}
			pin, err := l.GetPin(r.Context(), localUser)
			a.respond(w, map[string]any{"pin": pin}, err)
		}},
		{"POST", "/attempts", `Start a task {"kind": "lesson"|"review"|"quiz", "code"}; returns the lesson and the first question without its answer.`, func(w http.ResponseWriter, r *http.Request) {
			var body struct{ Kind, Code string }
			if !decode(w, r, &body) {
				return
			}
			out, err := l.Start(r.Context(), localUser, body.Kind, body.Code)
			a.respond(w, out, err)
		}},
		{"POST", "/attempts/{id}/answer", `Answer {"choice": n}; returns correctness, explanation and the next question or the result with implicit credit.`, func(w http.ResponseWriter, r *http.Request) {
			attempt, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
			var body struct{ Choice int }
			if !decode(w, r, &body) {
				return
			}
			out, err := l.Answer(r.Context(), localUser, attempt, body.Choice)
			a.respond(w, out, err)
		}},
		{"POST", "/placement", "Start the placement test on ?track=.", func(w http.ResponseWriter, r *http.Request) {
			out, err := l.StartPlacement(r.Context(), localUser, a.track(r))
			a.respond(w, out, err)
		}},
		{"POST", "/placement/answer", `Answer a placement question {"choice": n}, −1 is "don't know".`, func(w http.ResponseWriter, r *http.Request) {
			var body struct{ Choice int }
			if !decode(w, r, &body) {
				return
			}
			out, err := l.AnswerPlacement(r.Context(), localUser, body.Choice)
			a.respond(w, out, err)
		}},
		{"GET", "/progress/export", "Download all progress as JSON (format fire-progress-v1).", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename="fire-progress.json"`)
			w.Header().Set("Content-Type", "application/json")
			var b bytes.Buffer
			if err := l.st.Export(r.Context(), &b); err != nil {
				fail(w, 500, err.Error())
				return
			}
			_, _ = w.Write(b.Bytes())
		}},
		{"POST", "/progress/import", "Replace all progress with an export (the request body).", func(w http.ResponseWriter, r *http.Request) {
			err := l.st.Import(r.Context(), http.MaxBytesReader(w, r.Body, 64<<20))
			a.respond(w, map[string]bool{"ok": err == nil}, err)
		}},
		{"POST", "/progress/reset", `Forget all progress; needs {"confirm": "reset"}.`, func(w http.ResponseWriter, r *http.Request) {
			var body struct{ Confirm string }
			if !decode(w, r, &body) {
				return
			}
			if body.Confirm != "reset" {
				fail(w, 400, `send {"confirm": "reset"}`)
				return
			}
			a.respond(w, map[string]bool{"ok": true}, l.st.Reset(r.Context()))
		}},
	}
}

// Routes mounts the API under /api/v1. Unknown API paths answer JSON 404.
func (a *API) Routes(mux *http.ServeMux) {
	const p = "/api/v1"
	mux.HandleFunc("GET "+p+"/", a.guard(func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "unknown endpoint") }))
	for _, rt := range a.routes() {
		mux.HandleFunc(rt.Method+" "+p+rt.Path, a.guard(rt.h))
	}
}

// APIDoc is the Markdown reference generated into docs/api.md.
func (a *API) APIDoc() string {
	rs := a.routes()
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Path < rs[j].Path })
	var b strings.Builder
	b.WriteString("# API reference\n\n" + generated + "\n\nThe server listens on `127.0.0.1:17490`. Every path starts with `/api/v1`. Writes (`POST`) need the header `X-Requested-With: fire`; only `localhost`, `127.0.0.1` and `[::1]` are accepted as Host. There is one local learner and no accounts.\n\n| Method | Path | What it does |\n|---|---|---|\n")
	for _, r := range rs {
		b.WriteString("| " + r.Method + " | `/api/v1" + r.Path + "` | " + strings.ReplaceAll(r.Summary, "|", "\\|") + " |\n")
	}
	return b.String()
}

func (a *API) respond(w http.ResponseWriter, v any, err error) {
	switch {
	case err == nil:
		writeJSON(w, 200, v)
	case errors.Is(err, errNotFound):
		fail(w, 404, err.Error())
	case errors.Is(err, errUnavailable), errors.Is(err, errFinished):
		fail(w, 409, err.Error())
	default:
		fail(w, 500, err.Error())
	}
}
