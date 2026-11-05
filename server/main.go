// Command server runs ML Lab: the open API, the learning API and the web app.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:17490", "listen address")
	content := flag.String("content", "data/content.db", "read-only content database")
	users := flag.String("db", "data/users.db", "progress database")
	web := flag.String("web", "web/dist", "built web app")
	prefix := flag.String("prefix", "/ml", "URL prefix")
	secure := flag.Bool("secure", false, "mark cookies Secure (behind HTTPS)")
	flag.Parse()

	c, err := LoadContent(*content)
	if err != nil {
		log.Fatalf("content: %v", err)
	}
	st, err := OpenStore(*users)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	api := &API{lab: &Lab{c, st}, prefix: *prefix, secure: *secure, limit: &limiter{hits: map[string][]time.Time{}}}
	mux := http.NewServeMux()
	api.Routes(mux)

	// The content database is published as is: anyone can download and query it.
	mux.HandleFunc("GET "+*prefix+"/data/content.db", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Disposition", `attachment; filename="ml-lab-content.db"`)
		http.ServeFile(w, r, *content)
	})
	// SPA: real files from the build, everything else falls back to index.html.
	mux.HandleFunc("GET "+*prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, *prefix+"/")
		file := filepath.Join(*web, filepath.FromSlash(filepath.Clean("/"+rel)))
		if fi, err := os.Stat(file); rel != "" && err == nil && !fi.IsDir() {
			if strings.HasPrefix(rel, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFile(w, r, file)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(*web, "index.html"))
	})
	mux.HandleFunc("GET "+*prefix, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, *prefix+"/", http.StatusMovedPermanently) })

	log.Printf("ml-lab on %s%s (%d ml, %d math ideas)", *addr, *prefix, len(c.Program["ml"]), len(c.Program["math"]))
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
