package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Component
	BoxTop    string
	BoxLeft   string
	PingCount int

	dragging bool
	posX     float64
	posY     float64
	offsetX  float64
	offsetY  float64
}

func (a *App) updateCSS() {
	a.BoxTop = fmt.Sprintf("%.0fpx", a.posY)
	a.BoxLeft = fmt.Sprintf("%.0fpx", a.posX)
}

func (a *App) DragStart(x, y float64) {
	a.dragging = true
	a.offsetX = x - a.posX
	a.offsetY = y - a.posY
}

func (a *App) DragMove(x, y float64) {
	return
	if !a.dragging {
		return
	}
	a.posX = x - a.offsetX
	a.posY = y - a.offsetY
	a.updateCSS()
	a.MarkRefresh("BoxTop", "BoxLeft")
}

func (a *App) DragEnd(x, y float64) {
	a.dragging = false
}

func (a *App) Ping() {
	a.PingCount++
	fmt.Printf("Ping %d\n", a.PingCount)
}

func main() {
	app := &App{posX: 100, posY: 250}
	app.updateCSS()

	eng := godom.NewEngine()
	eng.Mount(app, ui, "ui/index.html")
	log.Fatal(eng.Start())
}
