package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/plugins/chartjs"
)

//go:embed ui
var ui embed.FS

// Alternative: use os.DirFS(".") instead of embed.FS to load templates from
// the local filesystem.
//   ui := os.DirFS(".")

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	chartjs.Register(eng)

	// Child components — registered by name, auto-wired via g-component attributes
	navbar := &Navbar{ComponentCount: 6, Status: "Connected"}
	navbar.TargetName = "navbar"
	navbar.Template = "ui/navbar/index.html"

	toast := &Toast{}
	toast.TargetName = "toast"
	toast.Template = "ui/toast/index.html"

	sidebar := NewSidebar()
	sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
	sidebar.TargetName = "sidebar"
	sidebar.Template = "ui/sidebar/index.html"

	// Shared state: Counter and Monitor both reference the same CounterState.
	// Incrementing/decrementing in Counter is immediately visible in Monitor's read-only display.
	sharedState := &CounterState{Count: 0, Step: 1}

	counter := &Counter{CounterState: sharedState}
	counter.TargetName = "counter"
	counter.Template = "ui/counter/index.html"

	clock := &Clock{}
	clock.TargetName = "clock"
	clock.Template = "ui/clock/index.html"

	monitor := &Monitor{CounterState: sharedState}
	monitor.TargetName = "monitor"
	monitor.Template = "ui/monitor/index.html"

	ticker := &Ticker{}
	ticker.TargetName = "ticker"
	ticker.Template = "ui/ticker/index.html"

	tips := &Tips{}
	tips.TargetName = "tips"
	tips.Template = "ui/tips/index.html"

	eng.Register(navbar, toast, sidebar, counter, clock, monitor, ticker, tips)

	go clock.startClock()
	go monitor.startMonitor()
	go ticker.startTicker()
	go tips.startTips()

	// Layout — root component, rendered into document.body via QuickServe
	layout := &Layout{
		Slots: []SlotInfo{
			{RegisteredName: "counter", Title: "Counter"},
			{RegisteredName: "clock", Title: "Clock"},
			{RegisteredName: "monitor", Title: "System Monitor"},
		},
	}
	layout.Template = "ui/layout/index.html"
	log.Fatal(eng.QuickServe(layout))
}
