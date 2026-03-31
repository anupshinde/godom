package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)

	// Routes — each is a full page using Go html/template layouts.
	// Template data is a plain struct, NOT a godom component.
	eng.Route("/", &PageData{Title: "Dashboard"}, "ui/layout/base.html", "ui/dashboard/page.html")
	eng.Route("/settings", &PageData{Title: "Settings"}, "ui/layout/base.html", "ui/settings/page.html")

	// Live components — registered globally, attach to g-component elements on any page.
	counter := &Counter{Count: 0, Step: 1}
	eng.Register("counter", counter, "ui/counter/index.html")

	clock := &Clock{}
	eng.Register("clock", clock, "ui/clock/index.html")

	go clock.startClock()

	log.Fatal(eng.Start())
}
