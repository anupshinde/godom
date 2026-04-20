// multi-page-v2: demonstrates multiple tool islands (counter, clock, monitor,
// solar system) organized as separate Go packages, mounted on per-tool pages
// and a combined dashboard page.
//
// Page chrome is rendered with Go's html/template; islands render into
// g-island="name" targets on each page.
//
// Run with GODOM_DEV=1 to switch the engine's shared FS from embedded to
// os.DirFS("examples/multi-page-v2") — edits to shared/*.html then take effect
// on browser refresh without rebuilding the binary. Run from the repo root:
//
//	GODOM_DEV=1 GODOM_NO_AUTH=1 go run ./examples/multi-page-v2/
package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"

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

//go:embed partials
var partialsFS embed.FS

//go:embed shared
var sharedEmbedFS embed.FS

// Welcome is an inline island declared in main — no separate package, no
// AssetsFS. Its template lives in the engine's shared FS (set via SetFS), so
// it demonstrates the engine-default FS path without per-island embedding.
type Welcome struct {
	godom.Island
	Mode string
}

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
	eng.Use(chartjs.Plugin, solar.Plugin)

	// --- Shared FS (SetFS) — embedded by default, os.DirFS in DEV mode ---
	// Tool packages carry their own AssetsFS, so they don't rely on SetFS.
	// The welcome island below does — it uses the engine default.
	var sharedFS fs.FS = sharedEmbedFS
	mode := "embedded (//go:embed)"
	if os.Getenv("GODOM_DEV") == "1" {
		sharedFS = os.DirFS("examples/multi-page-v2")
		mode = "os.DirFS (runtime edits)"
		log.Println("DEV mode: engine SetFS using os.DirFS; edit shared/*.html and refresh")
	}
	eng.SetFS(sharedFS)

	// --- Shared partials ---
	// UsePartials bulk-registers every *.html under partials/. RegisterPartial
	// takes a raw string for one-off inline partials — shown here with <kbd-key>,
	// a styled keyboard-key wrapper used by the welcome banner.
	eng.UsePartials(partialsFS, "partials")
	eng.RegisterPartial("kbd-key",
		`<kbd class="inline-block bg-gray-100 text-gray-700 text-xs font-mono px-1.5 py-0.5 rounded border border-gray-300"><g-slot/></kbd>`)

	// --- Tool islands ---
	// Counter, clock, monitor, and digiclock share one *counter.State pointer.
	// Mutating Count/Step in Counter auto-refreshes the other three views via
	// godom's shared-pointer refresh.
	sharedCounter := &counter.State{Step: 1}
	counterI := counter.New(sharedCounter)
	clockI := clock.New(sharedCounter)
	monitorI := monitor.New(sharedCounter)
	solarI := solar.New()

	// digiclock demonstrates Island.TemplateHTML — no HTML file, no embed,
	// just an inline Go string literal.
	digiClockI := digiclock.New(sharedCounter)

	// welcomeI uses the engine's shared SetFS (no AssetsFS on the island).
	welcomeI := &Welcome{
		Island: godom.Island{
			TargetName: "welcome",
			Template:   "shared/welcome.html",
		},
		Mode: mode,
	}

	eng.Register(welcomeI, counterI, clockI, monitorI, solarI, digiClockI)

	go clockI.Run()
	go monitorI.Run()
	go solarI.Run()
	go digiClockI.Run()

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
