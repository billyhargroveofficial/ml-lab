package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type API struct {
	lab    *Lab
	prefix string // "/ml"
	secure bool
	limit  *limiter
}

const cookieName = "mllab"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func public(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=300")
		h(w, r)
	}
}

func (a *API) user(r *http.Request) (int64, string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return 0, "", false
	}
	return a.lab.st.SessionUser(r.Context(), c.Value)
}

func (a *API) authed(h func(http.ResponseWriter, *http.Request, int64, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, name, ok := a.user(r)
		if !ok {
			fail(w, 401, "login required")
			return
		}
		// Cookie auth plus a custom header blocks cross-site form posts.
		if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "mllab" {
			fail(w, 403, "missing X-Requested-With")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		h(w, r, id, name)
	}
}

func track(r *http.Request) string {
	t := r.URL.Query().Get("track")
	if !tracks[t] {
		return "all"
	}
	return t
}

func (a *API) Routes(mux *http.ServeMux) {
	p := a.prefix + "/api/v1"
	c := a.lab.c
	// ---- open API ----
	// Unknown API paths answer JSON 404 instead of falling through to the app.
	mux.HandleFunc("GET "+p+"/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "unknown endpoint") })
	mux.HandleFunc("GET "+p+"/info", public(func(w http.ResponseWriter, r *http.Request) {
		counts := map[string]int{}
		for prog, ideas := range c.Program {
			counts[prog] = len(ideas)
		}
		writeJSON(w, 200, map[string]any{"info": c.Info, "ideas": counts})
	}))
	mux.HandleFunc("GET "+p+"/program/{program}", public(func(w http.ResponseWriter, r *http.Request) {
		prog := r.PathValue("program")
		secs, ok := c.Sections[prog]
		if !ok {
			fail(w, 404, "program is ml or math")
			return
		}
		type ideaRow struct {
			Code   string  `json:"code"`
			Name   string  `json:"name"`
			Level  int     `json:"level"`
			Weight float64 `json:"weight"`
		}
		ideas := map[string]ideaRow{}
		for _, i := range c.Program[prog] {
			ideas[i.Code] = ideaRow{i.Code, i.Name, i.Level, i.Weight}
		}
		writeJSON(w, 200, map[string]any{"program": prog, "sections": secs, "ideas": ideas})
	}))
	mux.HandleFunc("GET "+p+"/ideas", public(func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("q"))
		prog := r.URL.Query().Get("program")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		type row struct {
			Code       string  `json:"code"`
			Program    string  `json:"program"`
			Name       string  `json:"name"`
			Definition string  `json:"definition"`
			Level      int     `json:"level"`
			Weight     float64 `json:"weight"`
			Questions  int     `json:"questions"`
		}
		var all []row
		for _, pr := range []string{"ml", "math"} {
			if prog != "" && prog != pr {
				continue
			}
			for _, i := range c.Program[pr] {
				if q != "" && !strings.Contains(strings.ToLower(i.Name+" "+i.Definition+" "+i.Code), q) {
					continue
				}
				all = append(all, row{i.Code, i.Program, i.Name, i.Definition, i.Level, i.Weight, len(i.Questions)})
			}
		}
		total := len(all)
		all = all[min(offset, total):min(offset+limit, total)]
		if all == nil {
			all = []row{}
		}
		writeJSON(w, 200, map[string]any{"total": total, "items": all})
	}))
	mux.HandleFunc("GET "+p+"/ideas/{code...}", public(func(w http.ResponseWriter, r *http.Request) {
		i := c.Ideas[r.PathValue("code")]
		if i == nil {
			fail(w, 404, "unknown idea")
			return
		}
		writeJSON(w, 200, i)
	}))
	mux.HandleFunc("GET "+p+"/edges", public(func(w http.ResponseWriter, r *http.Request) {
		kind := r.URL.Query().Get("kind")
		type row struct {
			From string `json:"from"`
			Edge
		}
		out := []row{}
		for _, prog := range []string{"ml", "math"} {
			for _, i := range c.Program[prog] {
				for _, e := range i.Edges {
					if kind == "" || kind == e.Kind {
						out = append(out, row{i.Code, e})
					}
				}
			}
		}
		writeJSON(w, 200, out)
	}))
	var statsMu sync.Mutex
	var statsAt time.Time
	var statsCache *Stats
	mux.HandleFunc("GET "+p+"/stats", public(func(w http.ResponseWriter, r *http.Request) {
		statsMu.Lock()
		defer statsMu.Unlock()
		if statsCache == nil || time.Since(statsAt) > time.Minute {
			s, err := a.lab.Stats(r.Context())
			if err != nil {
				fail(w, 500, "stats unavailable")
				return
			}
			statsCache, statsAt = s, time.Now()
		}
		writeJSON(w, 200, statsCache)
	}))
	mux.HandleFunc("GET "+p+"/leaders", func(w http.ResponseWriter, r *http.Request) {
		viewer, _, _ := a.user(r)
		out, err := a.lab.Leaders(r.Context(), viewer, r.URL.Query().Get("period"))
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, 200, out)
	})

	// ---- account ----
	mux.HandleFunc("POST "+p+"/auth/{action}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") != "mllab" {
			fail(w, 403, "missing X-Requested-With")
			return
		}
		action := r.PathValue("action")
		if action == "logout" {
			if ck, err := r.Cookie(cookieName); err == nil {
				a.lab.st.DropSession(r.Context(), ck.Value)
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Path: a.prefix, MaxAge: -1})
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if !a.limit.allow(clientIP(r)) {
			fail(w, 429, "слишком много попыток, подождите минуту")
			return
		}
		var body struct{ Name, Password string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			fail(w, 400, "bad json")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		var id int64
		var err error
		switch action {
		case "register":
			if n := utf8.RuneCountInString(body.Name); n < 2 || n > 24 || strings.ContainsAny(body.Name, "<>\"'`\n\t") {
				fail(w, 400, "ник — от 2 до 24 символов")
				return
			}
			if len(body.Password) < 6 || len(body.Password) > 200 {
				fail(w, 400, "пароль — от 6 символов")
				return
			}
			id, err = a.lab.st.Register(r.Context(), body.Name, body.Password)
			if errors.Is(err, errNameTaken) {
				fail(w, 409, "этот ник уже занят")
				return
			}
		case "login", "delete":
			id, err = a.lab.st.Login(r.Context(), body.Name, body.Password)
			if errors.Is(err, errBadLogin) {
				fail(w, 401, "неверный ник или пароль")
				return
			}
			if err == nil && action == "delete" {
				if err = a.lab.st.DeleteUser(r.Context(), id); err != nil {
					fail(w, 500, "server error")
					return
				}
				http.SetCookie(w, &http.Cookie{Name: cookieName, Path: a.prefix, MaxAge: -1})
				writeJSON(w, 200, map[string]bool{"deleted": true})
				return
			}
		default:
			fail(w, 404, "unknown action")
			return
		}
		if err != nil {
			fail(w, 500, "server error")
			return
		}
		token, err := a.lab.st.NewSession(r.Context(), id)
		if err != nil {
			fail(w, 500, "server error")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: a.prefix, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second)})
		writeJSON(w, 200, map[string]string{"name": body.Name})
	})

	// ---- learning ----
	mux.HandleFunc("GET "+p+"/me", a.authed(func(w http.ResponseWriter, r *http.Request, id int64, name string) {
		me, err := a.lab.Me(r.Context(), id, name, track(r))
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, me)
	}))
	mux.HandleFunc("GET "+p+"/next", a.authed(func(w http.ResponseWriter, r *http.Request, id int64, _ string) {
		out, err := a.lab.Next(r.Context(), id, track(r))
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("GET "+p+"/pin", a.authed(func(w http.ResponseWriter, r *http.Request, id int64, _ string) {
		pin, err := a.lab.GetPin(r.Context(), id)
		a.respond(w, map[string]any{"pin": pin}, err)
	}))
	mux.HandleFunc("POST "+p+"/pin", a.authed(func(w http.ResponseWriter, r *http.Request, id int64, _ string) {
		var body struct{ Code string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			fail(w, 400, "bad json")
			return
		}
		if err := a.lab.SetPin(r.Context(), id, body.Code); err != nil {
			a.respond(w, nil, err)
			return
		}
		pin, err := a.lab.GetPin(r.Context(), id)
		a.respond(w, map[string]any{"pin": pin}, err)
	}))
	mux.HandleFunc("GET "+p+"/history", a.authed(func(w http.ResponseWriter, r *http.Request, id int64, _ string) {
		out, err := a.lab.History(r.Context(), id)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("POST "+p+"/attempts", a.authed(func(w http.ResponseWriter, r *http.Request, id int64, _ string) {
		var body struct{ Kind, Code string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			fail(w, 400, "bad json")
			return
		}
		out, err := a.lab.Start(r.Context(), id, body.Kind, body.Code)
		a.respond(w, out, err)
	}))
	mux.HandleFunc("POST "+p+"/attempts/{id}/answer", a.authed(func(w http.ResponseWriter, r *http.Request, user int64, _ string) {
		attempt, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var body struct{ Choice int }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			fail(w, 400, "bad json")
			return
		}
		out, err := a.lab.Answer(r.Context(), user, attempt, body.Choice)
		a.respond(w, out, err)
	}))
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

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		// Caddy appends the real peer address last; earlier values are client-supplied.
		parts := strings.Split(f, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// limiter allows 10 auth attempts per IP per minute.
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 10 {
		l.hits[ip] = keep
		return false
	}
	l.hits[ip] = append(keep, now)
	return true
}
