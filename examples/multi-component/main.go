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
	eng.Register("navbar", navbar, "ui/navbar/index.html")

	toast := &Toast{}
	eng.Register("toast", toast, "ui/toast/index.html")

	sidebar := NewSidebar()
	sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
	eng.Register("sidebar", sidebar, "ui/sidebar/index.html")

	// Shared state: Counter and Monitor both reference the same CounterState.
	// Incrementing/decrementing in Counter is immediately visible in Monitor's read-only display.
	sharedState := &CounterState{Count: 0, Step: 1}

	counter := &Counter{CounterState: sharedState}
	eng.Register("counter", counter, "ui/counter/index.html")

	clock := &Clock{}
	eng.Register("clock", clock, "ui/clock/index.html")

	monitor := &Monitor{CounterState: sharedState}
	eng.Register("monitor", monitor, "ui/monitor/index.html")

	ticker := &Ticker{}
	eng.Register("ticker", ticker, "ui/ticker/index.html")

	tips := &Tips{}
	eng.Register("tips", tips, "ui/tips/index.html")

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
	log.Fatal(eng.QuickServe(layout, "ui/layout/index.html"))
}
