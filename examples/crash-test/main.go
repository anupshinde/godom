package main

import (
	"embed"
	"log"
	"os"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Component
	Countdown    int
	Action       string
	BGCountdown  int
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
	eng := godom.NewEngine()
	eng.SetFS(ui)

	app := &App{BGCountdown: 30}
	app.Template = "ui/index.html"

	// Background goroutine crash — not recoverable by the framework.
	go func() {
		for app.BGCountdown > 0 {
			time.Sleep(1 * time.Second)
			app.BGCountdown--
			app.Refresh()
		}
		panic("crash-test: background goroutine panic!")
	}()

	log.Fatal(eng.QuickServe(app))
}
