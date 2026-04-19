package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Island
	Countdown   int
	Action      string
	BGCountdown int
}

func (a *App) Exit() {
	a.Action = "Exiting"
	go a.countdownAndDo(func() {
		os.Exit(0)
	})
}

func (a *App) Crash() {
	a.Action = "Crashing"
	go a.countdownAndDo(func() {
		a.ExecJS("godom.call('DoCrash')", func([]byte, string) {})
	})
}

func (a *App) DoCrash() {
	panic("crash-test: deliberate panic!")
}

func (a *App) countdownAndDo(fn func()) {
	a.Countdown = 5
	a.Refresh()
	for a.Countdown > 0 {
		time.Sleep(1 * time.Second)
		a.Countdown--
		a.Refresh()
	}
	fn()
}

func main() {
	useDefault := flag.Bool("default", false, "use default disconnect overlay/badge")
	embedMode := flag.Bool("embed", false, "run in embedded mode (tests disconnect badge)")
	flag.Parse()

	eng := godom.NewEngine()
	eng.SetFS(ui)

	if !*useDefault {
		disconnectHTML, err := fs.ReadFile(ui, "ui/partials/disconnect.html")
		if err != nil {
			log.Fatal(err)
		}
		eng.DisconnectHTML = string(disconnectHTML)

		badgeHTML, err := fs.ReadFile(ui, "ui/partials/disconnect-badge.html")
		if err != nil {
			log.Fatal(err)
		}
		eng.DisconnectBadgeHTML = string(badgeHTML)
	}

	app := &App{BGCountdown: 30}
	clock := &Clock{}
	clock.Start()

	// Background goroutine crash — not recoverable by the framework.
	go func() {
		for app.BGCountdown > 0 {
			time.Sleep(1 * time.Second)
			app.BGCountdown--
			app.Refresh()
		}
		panic("crash-test: background goroutine panic!")
	}()

	if *embedMode {
		// Embedded mode: static page with g-island targets.
		app.TargetName = "crashtest"
		app.Template = "ui/controls/index.html"

		clock.TargetName = "clock"
		clock.Template = "ui/clock/index.html"

		eng.Register(app, clock)

		embedHTML, err := fs.ReadFile(ui, "ui/embed.html")
		if err != nil {
			log.Fatal(err)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(embedHTML)
		})

		eng.SetMux(mux, nil)
		if err := eng.Run(); err != nil {
			log.Fatal(err)
		}
		log.Fatal(eng.ListenAndServe())
	} else {
		// Root mode: QuickServe with full-page overlay.
		app.Template = "ui/index.html"
		log.Fatal(eng.QuickServe(app))
	}
}
