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
	app := godom.New()
	app.Mount(&App{Step: 1}, ui)
	log.Fatal(app.Start())
}
