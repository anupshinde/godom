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
	eng.SetUI(ui)
	chartjs.Register(eng)

	// Child components — registered by name, auto-wired to layout's <g-slot> tags
	navbar := &Navbar{ComponentCount: 7, Status: "Connected"}
	eng.Register("navbar", navbar, "ui/navbar/index.html")

	toast := &Toast{}
	eng.Register("toast", toast, "ui/toast/index.html")

	sidebar := NewSidebar()
	sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
	eng.Register("sidebar", sidebar, "ui/sidebar/index.html")

	// Shared state: Counter and CounterDisplay both reference the same CounterState.
	// Incrementing/decrementing in Counter is immediately visible in CounterDisplay.
	sharedState := &CounterState{Count: 0, Step: 1}

	counter := &Counter{CounterState: sharedState}
	eng.Register("counter", counter, "ui/counter/index.html")

	counterDisplay := &CounterDisplay{CounterState: sharedState}
	eng.Register("counter_display", counterDisplay, "ui/counter-display/index.html")

	clock := &Clock{}
	eng.Register("clock", clock, "ui/clock/index.html")

	monitor := &Monitor{}
	eng.Register("monitor", monitor, "ui/monitor/index.html")

	ticker := &Ticker{}
	eng.Register("ticker", ticker, "ui/ticker/index.html")

	tips := &Tips{}
	eng.Register("tips", tips, "ui/tips/index.html")

	// Layout — root component with orderable slots
	layout := &Layout{
		Slots: []SlotInfo{
			{RegisteredName: "counter", Title: "Counter"},
			{RegisteredName: "counter_display", Title: "Counter (Read-Only)"},
			{RegisteredName: "clock", Title: "Clock"},
			{RegisteredName: "monitor", Title: "System Monitor"},
		},
	}
	eng.Mount(layout, "ui/layout/index.html")

	go clock.startClock()
	go monitor.startMonitor()
	go ticker.startTicker()
	go tips.startTips()

	log.Fatal(eng.Start())
}
