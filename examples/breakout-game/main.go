package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/anupshinde/godom"
)

//go:embed components
var components embed.FS

//go:embed pages
var pages embed.FS

//go:embed gyro.js
var gyroJS string

//go:embed sfx.js
var sfxJS string

// Page templates — each page parsed with layout.
var (
	menuTmpl       = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/menu/page.html"))
	playTmpl       = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/play/page.html"))
	controllerHTML string
	scoresTmpl     = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/scores/page.html"))
)

func init() {
	b, err := fs.ReadFile(pages, "pages/controller/page.html")
	if err != nil {
		panic(err)
	}
	controllerHTML = string(b)
}

type PageData struct {
	Title string
	Page  string
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(components)
	eng.RegisterPlugin("gyro", gyroJS)
	eng.RegisterPlugin("sfx", sfxJS)

	scores := NewScores()
	scores.TargetName = "scores"
	scores.Template = "components/scores/index.html"

	// Shared game state — both views embed the same *GameState pointer.
	// Changes from either view are visible to both, and godom's shared
	// pointer refresh keeps them in sync across browsers.
	state := NewGameState()
	state.onGameOver = func(score int) {
		if score > 0 {
			scores.Add(score)
		}
	}

	playView := &PlayView{GameState: state}
	playView.TargetName = "game"
	playView.Template = "components/game/play.html"

	controllerView := &ControllerView{GameState: state}
	controllerView.TargetName = "controller"
	controllerView.Template = "components/game/controller.html"

	statusView := &StatusView{GameState: state}
	statusView.TargetName = "status"
	statusView.Template = "components/status/index.html"

	eng.Register(scores, playView, controllerView, statusView)

	go playView.Run()
	go statusView.RunStatusRefresh()

	// User owns the mux and routes.
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		menuTmpl.Execute(w, &PageData{Title: "Menu", Page: "menu"})
	})

	mux.HandleFunc("/play", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		playTmpl.Execute(w, &PageData{Title: "Play", Page: "play"})
	})

	mux.HandleFunc("/controller", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(controllerHTML))
	})

	mux.HandleFunc("/scores", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		scoresTmpl.Execute(w, &PageData{Title: "High Scores", Page: "scores"})
	})

	eng.SetMux(mux, nil)
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Breakout — classic brick-breaking game in Go")
	log.Fatal(eng.ListenAndServe())
}
