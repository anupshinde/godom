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

	// One shared state, four components.
	shared := &CounterState{Step: 1}

	// Three Counter instances — same Go type, different templates.
	counterA := &Counter{CounterState: shared}
	counterA.TargetName = "counter_full"
	counterA.Template = "ui/counter/full.html"

	counterB := &Counter{CounterState: shared}
	counterB.TargetName = "counter_compact"
	counterB.Template = "ui/counter/compact.html"

	counterC := &Counter{CounterState: shared}
	counterC.TargetName = "counter_mini"
	counterC.Template = "ui/counter/mini.html"

	// One CounterDisplay — different type, read-only template.
	display := &CounterDisplay{CounterState: shared}
	display.TargetName = "counter_display"
	display.Template = "ui/counter-display/index.html"

	eng.Register(counterA, counterB, counterC, display)

	layout := &Layout{}
	layout.Template = "ui/layout/index.html"
	log.Fatal(eng.QuickServe(layout))
}
