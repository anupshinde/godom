package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/anupshinde/godom"
)

//go:embed pages
var pages embed.FS

//go:embed components
var components embed.FS

// Page templates — parsed once at startup.
var templates = template.Must(template.ParseFS(pages, "pages/dashboard/page.html", "pages/settings/page.html"))

func main() {
	eng := godom.NewEngine()
	eng.SetFS(components)

	// Live components — godom templates live in components/
	counter := &Counter{Count: 0, Step: 1}
	eng.Register("counter", counter, "components/counter/index.html")

	clock := &Clock{}
	eng.Register("clock", clock, "components/clock/index.html")

	go clock.startClock()

	// User owns the mux, routes, and server.
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.ExecuteTemplate(w, "dashboard", &PageData{Title: "Dashboard"})
	})

	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.ExecuteTemplate(w, "settings", &PageData{Title: "Settings"})
	})

	// godom registers handlers on the user's mux and starts component lifecycle.
	eng.SetMux(mux, &godom.MuxOptions{
		WSPath:     "/godom/ws",
		ScriptPath: "/godom/godom.js",
	})
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	log.Fatal(eng.ListenAndServe())
}
