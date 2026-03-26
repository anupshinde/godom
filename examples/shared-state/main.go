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
	eng.SetUI(ui)

	layout := &Layout{}
	eng.Mount(layout, ui, "ui/layout/index.html")

	// One shared state, four components.
	shared := &CounterState{Step: 1}

	// Three Counter instances — same Go type, different templates.
	counterA := &Counter{CounterState: shared}
	eng.Register("counter_full", counterA, "ui/counter/full.html")

	counterB := &Counter{CounterState: shared}
	eng.Register("counter_compact", counterB, "ui/counter/compact.html")

	counterC := &Counter{CounterState: shared}
	eng.Register("counter_mini", counterC, "ui/counter/mini.html")

	// One CounterDisplay — different type, read-only template.
	display := &CounterDisplay{CounterState: shared}
	eng.Register("counter_display", display, "ui/counter-display/index.html")

	log.Fatal(eng.Start())
}
