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

// Page templates — each page parsed with layout, once at startup.
var (
	dashboardTmpl = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/dashboard/page.html"))
	settingsTmpl  = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/settings/page.html"))
)

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
		dashboardTmpl.Execute(w, &PageData{Title: "Dashboard", Page: "dashboard"})
	})

	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		settingsTmpl.Execute(w, &PageData{Title: "Settings", Page: "settings"})
	})

	// godom registers /ws and /godom.js on the user's mux and starts component lifecycle.
	eng.SetMux(mux, nil) // default paths; for custom paths:
	// eng.SetMux(mux, &godom.MuxOptions{
	//     WSPath:     "/app/ws",
	//     ScriptPath: "/app/godom.js",
	// })
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	log.Fatal(eng.ListenAndServe())
}
