// multi-page-v2: demonstrates multiple tool islands (counter, clock, monitor,
// solar system) organized as separate Go packages, mounted on per-tool pages
// and a combined dashboard page.
//
// Page chrome is rendered with Go's html/template; islands render into
// g-island="name" targets on each page.
package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/plugins/chartjs"

	"github.com/anupshinde/godom/examples/multi-page-v2/tools/clock"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/counter"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/digiclock"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/monitor"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/solar"
)

//go:embed pages
var pagesFS embed.FS

type PageData struct {
	Title string
	Page  string
}

func mustTmpl(page string) *template.Template {
	return template.Must(template.ParseFS(pagesFS,
		"pages/layout/base.html",
		"pages/"+page+"/page.html",
	))
}

func main() {
	eng := godom.NewEngine()
	// No SetFS needed — each tool brings its own AssetsFS.
	eng.Use(chartjs.Plugin, solar.Plugin)

	// Build tool islands. Counter, clock, and monitor all share one
	// *counter.State pointer — mutating Count/Step in Counter auto-refreshes
	// the clock and monitor views via godom's shared-pointer refresh.
	sharedCounter := &counter.State{Step: 1}
	counterI := counter.New(sharedCounter)
	clockI := clock.New(sharedCounter)
	monitorI := monitor.New(sharedCounter)
	solarI := solar.New()

	// digiclock demonstrates Island.TemplateHTML — the tool carries no HTML
	// file at all, just an inline string. Used as inline prose on the dashboard.
	digiClockI := digiclock.New()

	eng.Register(counterI, clockI, monitorI, solarI, digiClockI)

	// Fire up goroutine-driven tools.
	go clockI.Run()
	go monitorI.Run()
	go solarI.Run()
	go digiClockI.Run()

	// Pre-parse page templates once.
	pages := map[string]*template.Template{
		"dashboard": mustTmpl("dashboard"),
		"counter":   mustTmpl("counter"),
		"clock":     mustTmpl("clock"),
		"monitor":   mustTmpl("monitor"),
		"solar":     mustTmpl("solar"),
	}

	mux := http.NewServeMux()
	serve := func(path, pageKey, title string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = pages[pageKey].ExecuteTemplate(w, "base", &PageData{Title: title, Page: pageKey})
		})
	}
	serve("/", "dashboard", "Dashboard")
	serve("/counter", "counter", "Counter")
	serve("/clock", "clock", "Clock")
	serve("/monitor", "monitor", "Monitor")
	serve("/solar", "solar", "Solar System")

	eng.SetMux(mux, nil)
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}
	log.Fatal(eng.ListenAndServe())
}
