package main

import (
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Component
	Percent  int
	Width    string
	Label    string
	Running  bool
}

func (a *App) Start() {
	if a.Running {
		return
	}
	a.Running = true
	a.Percent = 0
	a.updateWidth()
	go a.simulate()
}

func (a *App) Reset() {
	a.Running = false
	a.Percent = 0
	a.updateWidth()
}

func (a *App) simulate() {
	for a.Percent < 100 && a.Running {
		time.Sleep(80 * time.Millisecond)
		a.Percent++
		a.updateWidth()
		a.Refresh()
	}
	a.Running = false
	a.Refresh()
}

func (a *App) updateWidth() {
	a.Width = fmt.Sprintf("%d%%", a.Percent)
	a.Label = fmt.Sprintf("%d%%", a.Percent)
}

func main() {
	eng := godom.NewEngine()
	eng.SetUI(ui)
	root := &App{Label: "0%", Width: "0%"}
	eng.Mount(root, "ui/index.html")
	log.Fatal(eng.Start())
}
