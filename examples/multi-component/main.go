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
	chartjs.Register(eng)

	// Layout — root component with orderable slots
	layout := &Layout{
		Slots: []SlotInfo{
			{Name: "counter", Title: "Counter"},
			{Name: "counter-display", Title: "Counter (Read-Only)"},
			{Name: "clock", Title: "Clock"},
			{Name: "monitor", Title: "System Monitor"},
		},
	}
	eng.Mount(layout, ui, "ui/layout/index.html")

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

	// Dynamic components — referenced via {{slot.Name}} in a g-for loop,
	// so they need AddToSlot for parent wiring.
	counterDisplay := &CounterDisplay{CounterState: sharedState}

	counter := &Counter{CounterState: sharedState, Display: counterDisplay}
	eng.Register("counter", counter, "ui/counter/index.html")
	eng.AddToSlot(layout, "counter", counter)
	eng.Register("counter-display", counterDisplay, "ui/counter-display/index.html")
	eng.AddToSlot(layout, "counter-display", counterDisplay)

	clock := &Clock{}
	eng.Register("clock", clock, "ui/clock/index.html")
	eng.AddToSlot(layout, "clock", clock)

	monitor := &Monitor{}
	eng.Register("monitor", monitor, "ui/monitor/index.html")
	eng.AddToSlot(layout, "monitor", monitor)

	ticker := &Ticker{}
	eng.Register("ticker", ticker, "ui/ticker/index.html")

	tips := &Tips{}
	eng.Register("tips", tips, "ui/tips/index.html")

	go clock.startClock()
	go monitor.startMonitor()
	go ticker.startTicker()
	go tips.startTips()

	log.Fatal(eng.Start())
}
