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
	Color       string
	Colors      []string
	Message     string
	Agree       bool
	Dark        bool
	Title       string
	Description string
}

func (a *App) Confirm() {
	a.Message = "You picked: " + a.Color
}

func (a *App) ResetToGreen() {
	a.Color = "green"
	a.Message = "Reset to green from Go"
}

func (a *App) ToggleDark() {
	a.Dark = !a.Dark
}

func (a *App) Summary() string {
	return a.Title + "...." + a.Description
}

func main() {
	eng := godom.NewEngine()
	eng.SetUI(ui)
	eng.Mount(&App{
		Color:  "blue",
		Colors: []string{"red", "green", "blue", "yellow"},
		Agree:  true,
	}, "ui/index.html")
	log.Fatal(eng.Start())
}
