package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Component
	Count int
	Step  int
}

func (a *App) Increment() {
	a.Count += a.Step
}

func (a *App) Decrement() {
	a.Count -= a.Step
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	log.Fatal(eng.QuickServe(&App{Step: 1}, "ui/index.html"))
}
