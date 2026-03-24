package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/plugins/chartjs"
)

//go:embed ui
var ui embed.FS

func main() {
	eng := godom.NewEngine()
	chartjs.Register(eng)

	// Layout — root component with orderable slots
	layout := &Layout{
		Slots: []SlotInfo{
			{Name: "counter", Title: "Counter"},
			{Name: "clock", Title: "Clock"},
			{Name: "monitor", Title: "System Monitor"},
		},
	}
	eng.Mount(layout, ui, "ui/layout/index.html")

	// Navbar
	navbar := &Navbar{ComponentCount: 7, Status: "Connected"}
	eng.Mount(navbar, ui, "ui/navbar/index.html")
	eng.AddToSlot(layout, "navbar", navbar)

	// Toast
	toast := &Toast{}
	eng.Mount(toast, ui, "ui/toast/index.html")
	eng.AddToSlot(layout, "toast", toast)

	// Sidebar
	sidebar := &Sidebar{
		ActiveID: "dashboard",
		Items: []MenuItem{
			{ID: "dashboard", Icon: "\u25A0", Label: "Dashboard", Active: true},
			{ID: "counter", Icon: "\u25B6", Label: "Counter"},
			{ID: "clock", Icon: "\u25CB", Label: "Clock"},
			{ID: "ticker", Icon: "\u25B2", Label: "Ticker"},
			{ID: "users", Icon: "\u25C6", Label: "Users"},
			{ID: "analytics", Icon: "\u25AC", Label: "Analytics"},
			{ID: "settings", Icon: "\u2699", Label: "Settings"},
		},
	}
	for i := range sidebar.Items {
		if !sidebar.Items[i].Active {
			sidebar.Items[i].Inactive = true
		}
	}
	sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
	eng.Mount(sidebar, ui, "ui/sidebar/index.html")
	eng.AddToSlot(layout, "sidebar", sidebar)

	// Counter
	counter := &Counter{Step: 1}
	eng.Mount(counter, ui, "ui/counter/index.html")
	eng.AddToSlot(layout, "counter", counter)

	// Clock
	clock := &Clock{}
	eng.Mount(clock, ui, "ui/clock/index.html")
	eng.AddToSlot(layout, "clock", clock)

	// System Monitor
	monitor := &Monitor{}
	eng.Mount(monitor, ui, "ui/monitor/index.html")
	eng.AddToSlot(layout, "monitor", monitor)

	// Stock Ticker
	ticker := &Ticker{}
	eng.Mount(ticker, ui, "ui/ticker/index.html")
	eng.AddToSlot(layout, "ticker", ticker)

	// Tips
	tips := &Tips{}
	eng.Mount(tips, ui, "ui/tips/index.html")
	eng.AddToSlot(layout, "tips", tips)

	go clock.startClock()
	go monitor.startMonitor()
	go ticker.startTicker()
	go tips.startTips()

	log.Fatal(eng.Start())
}
