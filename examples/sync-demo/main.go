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

type Box struct {
	Top  string
	Left string
}

type App struct {
	godom.Component
	Box       Box
	PingCount int
	Items     []string
	Inputs    map[string]any
	Processing bool
	Countdown  int
	Agreed     bool

	dragging bool
	posX     float64
	posY     float64
	offsetX  float64
	offsetY  float64
}

func (a *App) updateCSS() {
	a.Box.Top = fmt.Sprintf("%.0fpx", a.posY)
	a.Box.Left = fmt.Sprintf("%.0fpx", a.posX)
}

func (a *App) DragStart(x, y float64) {
	a.dragging = true
	a.offsetX = x - a.posX
	a.offsetY = y - a.posY
}

func (a *App) DragMove(x, y float64) {
	if !a.dragging {
		return
	}
	a.posX = x - a.offsetX
	a.posY = y - a.offsetY
	a.updateCSS()
	a.MarkRefresh("Box")
}

func (a *App) DragEnd(x, y float64) {
	a.dragging = false
}

func (a *App) Ping() {
	a.PingCount++
	fmt.Printf("Ping %d\n", a.PingCount)
}

func (a *App) AddItem() {
	a.Items = append(a.Items, "")
}

func (a *App) DoNothing() {
	fmt.Println("DoNothing does nothing")
}

func (a *App) Process() {
	if a.Processing {
		return
	}
	a.Processing = true
	a.Countdown = 10
	go func() {
		for a.Countdown > 0 {
			time.Sleep(1 * time.Second)
			a.Countdown--
			a.MarkRefresh("Countdown")
			a.Refresh()
		}
		a.Processing = false
		a.Refresh()
	}()
}

func main() {
	app := &App{posX: 200, posY: 350, Inputs: map[string]any{}}
	app.updateCSS()

	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.Port = 61820
	eng.Mount(app, "ui/index.html")
	log.Fatal(eng.Start())
}
