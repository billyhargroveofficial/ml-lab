// Command fire runs the learning app and the tools agents use to add packs.
//
//	fire dev                 API + Vite dev server with hot reload (http://127.0.0.1:5190)
//	fire serve               API + the built web app (http://127.0.0.1:17490)
//	fire validate [pack…]    check packs; exit 1 on errors
//	fire search <words>      find existing ideas to link to
//	fire show <code>         print one idea as JSON
//	fire new <id>            scaffold packs/<id>/
//	fire docs [--check]      regenerate docs/ (or fail if stale)
//	fire progress export|import <file>|reset
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const usage = `fire — FIRe spaced repetition over your own packs

  dev                 API + Vite dev server with hot reload (http://127.0.0.1:5190)
  serve               API + built web app (http://127.0.0.1:17490)
  validate [pack…]    check packs (all by default); exit 1 on errors
  search <words>      find existing ideas to link to   [-pack id] [-json] [-n 20]
  show <code>         print one idea as JSON
  new <id>            scaffold packs/<id>/ from the template
  docs [-check]       regenerate docs/ (with -check: fail if stale)
  progress export [file] | import <file> | reset -yes

Common flags: -packs packs  -progress data/progress.db
`

func main() {
	root, err := findRoot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		log.Fatal(err)
	}
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	packs := fs.String("packs", "packs", "packs folder")
	progress := fs.String("progress", filepath.Join("data", "progress.db"), "local progress database")
	switch cmd {
	case "serve", "dev":
		addr := fs.String("addr", "127.0.0.1:17490", "listen address (keep it on localhost)")
		web := fs.String("web", filepath.Join("web", "dist"), "built web app")
		_ = fs.Parse(args)
		serve(cmd == "dev", *addr, *packs, *progress, *web)
	case "validate":
		asJSON := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(args)
		os.Exit(validate(*packs, fs.Args(), *asJSON))
	case "search":
		pack := fs.String("pack", "", "only this pack")
		asJSON := fs.Bool("json", false, "print JSON")
		n := fs.Int("n", 20, "how many")
		_ = fs.Parse(args)
		c, _ := LoadPacks(*packs)
		res := Search(c, strings.Join(fs.Args(), " "), *pack)
		res = res[:min(*n, len(res))]
		if *asJSON {
			printJSON(res)
			return
		}
		if len(res) == 0 {
			fmt.Println("nothing found")
		}
		for _, r := range res {
			fmt.Printf("%-48s %s — %s (level %d)\n", r.Code, r.Name, r.Topic, r.Level)
		}
	case "show":
		_ = fs.Parse(args)
		c, _ := LoadPacks(*packs)
		i := c.Ideas[fs.Arg(0)]
		if i == nil {
			log.Fatalf("no idea %q (try: fire search …)", fs.Arg(0))
		}
		printJSON(i)
	case "new":
		title := fs.String("title", "", "pack title")
		_ = fs.Parse(args)
		if err := scaffold(*packs, fs.Arg(0), *title); err != nil {
			log.Fatal(err)
		}
	case "docs":
		check := fs.Bool("check", false, "fail if docs/ is stale")
		_ = fs.Parse(args)
		os.Exit(docs(*check))
	case "progress":
		yes := false
		var rest []string
		for _, a := range args {
			if a == "-yes" || a == "--yes" {
				yes = true
			} else {
				rest = append(rest, a)
			}
		}
		_ = fs.Parse(rest)
		os.Exit(progressCmd(*progress, fs.Args(), yes))
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

// findRoot walks up to the repository root (the folder with AGENTS.md and
// server/), so every command works from any subfolder.
func findRoot() (string, error) {
	dir, _ := os.Getwd()
	for d := dir; ; d = filepath.Dir(d) {
		if fileExists(filepath.Join(d, "AGENTS.md")) && fileExists(filepath.Join(d, "server", "go.mod")) {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("run inside the repository (no AGENTS.md + server/go.mod above %s)", dir)
		}
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func serve(dev bool, addr, packs, progress, web string) {
	_ = os.MkdirAll(packs, 0o755)
	_ = os.MkdirAll(filepath.Dir(progress), 0o755)
	st, err := OpenStore(progress)
	if err != nil {
		log.Fatalf("progress: %v", err)
	}
	c, status := LoadPacks(packs)
	logPacks(status)
	api := &API{lab: &Lab{c: c, st: st}, pack: status}
	mux := http.NewServeMux()
	api.Routes(mux)
	if !dev {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if !localHost(r.Host) {
				http.Error(w, "local only", 403)
				return
			}
			rel := strings.TrimPrefix(r.URL.Path, "/")
			file := filepath.Join(web, filepath.FromSlash(filepath.Clean("/"+rel)))
			if fi, err := os.Stat(file); rel != "" && err == nil && !fi.IsDir() {
				http.ServeFile(w, r, file)
				return
			}
			index := filepath.Join(web, "index.html")
			if !fileExists(index) {
				http.Error(w, "web app is not built: cd web && npm ci && npm run build (or use: fire dev)", 503)
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, index)
		})
	}
	go watch(packs, func() {
		c, status := LoadPacks(packs)
		api.Swap(c, status)
		log.Printf("packs reloaded")
		logPacks(status)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	if dev {
		npm := "npm"
		if !fileExists(filepath.Join("web", "node_modules")) {
			run := exec.Command(npm, "ci")
			run.Dir, run.Stdout, run.Stderr = "web", os.Stdout, os.Stderr
			if err := run.Run(); err != nil {
				log.Fatalf("npm ci: %v (is Node.js 20+ installed?)", err)
			}
		}
		vite := exec.Command(npm, "run", "dev")
		vite.Dir, vite.Stdout, vite.Stderr = "web", os.Stdout, os.Stderr
		if err := vite.Start(); err != nil {
			log.Fatalf("vite: %v", err)
		}
		log.Printf("open http://127.0.0.1:5190/  (API on %s)", addr)
		go func() { _ = vite.Wait(); stop <- os.Interrupt }()
		<-stop
		if vite.Process != nil {
			_ = vite.Process.Signal(os.Interrupt)
		}
	} else {
		log.Printf("open http://%s/", addr)
		<-stop
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func logPacks(status []PackStatus) {
	if len(status) == 0 {
		log.Printf("no packs yet: ask your agent for a topic (see AGENTS.md) or run: fire new <id>")
	}
	for _, s := range status {
		state := "ok"
		if !s.Loaded {
			state = fmt.Sprintf("NOT LOADED, %d errors (fire validate %s)", len(s.Errors), s.ID)
		}
		log.Printf("pack %-24s %3d ideas  %s", s.ID, s.Ideas, state)
	}
}

// watch calls reload when files under dir change and then stay unchanged
// for one more poll (an agent may still be writing).
func watch(dir string, reload func()) {
	last := fingerprint(dir)
	pending := ""
	for range time.Tick(2 * time.Second) {
		fp := fingerprint(dir)
		switch {
		case fp != last && fp != pending:
			pending = fp
		case fp == pending && fp != last:
			last, pending = fp, ""
			reload()
		}
	}
}

func fingerprint(dir string) string {
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			fmt.Fprintf(&b, "%s|%d|%d\n", p, fi.Size(), fi.ModTime().UnixNano())
		}
		return nil
	})
	return b.String()
}

func validate(packs string, only []string, asJSON bool) int {
	_, status := LoadPacks(packs)
	if len(only) > 0 {
		want := map[string]bool{}
		for _, o := range only {
			want[strings.TrimSuffix(filepath.Base(o), "/")] = true
		}
		var keep []PackStatus
		for _, s := range status {
			if want[s.ID] {
				keep = append(keep, s)
				delete(want, s.ID)
			}
		}
		for id := range want {
			keep = append(keep, PackStatus{ID: id, Errors: []string{"no such folder in " + packs}})
		}
		status = keep
	}
	bad := 0
	if asJSON {
		printJSON(status)
	}
	for _, s := range status {
		if len(s.Errors) > 0 {
			bad++
		}
		if asJSON {
			continue
		}
		mark := "ok  "
		if len(s.Errors) > 0 {
			mark = "FAIL"
		}
		fmt.Printf("%s %s — %d topics, %d ideas\n", mark, s.ID, s.Topics, s.Ideas)
		for _, e := range s.Errors {
			fmt.Println("  error:", e)
		}
		for _, w := range s.Warnings {
			fmt.Println("  warning:", w)
		}
	}
	if len(status) == 0 && !asJSON {
		fmt.Println("no packs in", packs)
	}
	if bad > 0 {
		return 1
	}
	return 0
}

func scaffold(packs, id, title string) error {
	if !slugRe.MatchString(id) {
		return fmt.Errorf("pack id %q: use lowercase latin letters, digits and dashes", id)
	}
	dir := filepath.Join(packs, id)
	if fileExists(dir) {
		return fmt.Errorf("%s already exists", dir)
	}
	if title == "" {
		title = id
	}
	raw, err := os.ReadFile(filepath.Join("examples", "packs", "softmax", "pack.json"))
	if err != nil {
		return err
	}
	var p PackFile
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	p.ID, p.Title, p.Description = id, title, ""
	p.Topics = p.Topics[:1]
	p.Topics[0].Ideas = p.Topics[0].Ideas[:1]
	p.Topics[0].Ideas[0].Requires = nil
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("created %s/pack.json from the example — replace its topic and idea, then run: fire validate %s\n", dir, id)
	return nil
}

func progressCmd(path string, args []string, yes bool) int {
	st, err := OpenStore(path)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	switch {
	case len(args) >= 1 && args[0] == "export":
		w := os.Stdout
		if len(args) > 1 {
			f, err := os.Create(args[1])
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			w = f
		}
		if err := st.Export(ctx, w); err != nil {
			log.Fatal(err)
		}
	case len(args) == 2 && args[0] == "import":
		f, err := os.Open(args[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err := st.Import(ctx, f); err != nil {
			log.Fatal(err)
		}
		fmt.Println("imported")
	case len(args) == 1 && args[0] == "reset":
		if !yes {
			fmt.Fprintln(os.Stderr, "this forgets all progress; run again with -yes")
			return 1
		}
		if err := st.Reset(ctx); err != nil {
			log.Fatal(err)
		}
		fmt.Println("progress reset")
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	return 0
}
