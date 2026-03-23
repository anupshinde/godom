package main

import (
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

// Counter is a click-driven component with its own state.
type Counter struct {
	godom.Component
	Count int
	Step  int
}

func (c *Counter) Increment() {
	c.Count += c.Step
}

func (c *Counter) Decrement() {
	c.Count -= c.Step
}

// Clock is a time-driven component that refreshes from a goroutine.
type Clock struct {
	godom.Component
	Time       string
	HourHand   string
	MinuteHand string
	SecondHand string
}

func (c *Clock) updateTime() {
	now := time.Now()
	c.Time = now.Format("15:04:05")

	h, m, s := now.Hour()%12, now.Minute(), now.Second()
	hourAngle := float64(h)*30 + float64(m)*0.5
	minuteAngle := float64(m)*6 + float64(s)*0.1
	secondAngle := float64(s) * 6

	c.HourHand = fmt.Sprintf("rotate(%.1f 50 50)", hourAngle)
	c.MinuteHand = fmt.Sprintf("rotate(%.1f 50 50)", minuteAngle)
	c.SecondHand = fmt.Sprintf("rotate(%.1f 50 50)", secondAngle)
}

func (c *Clock) startClock() {
	ticker := time.NewTicker(50 * time.Millisecond)
	for range ticker.C {
		old := c.Time
		c.updateTime()
		if c.Time != old {
			c.Refresh()
		}
	}
}

func main() {
	eng := godom.NewEngine()

	// Shell: static HTML with slots for components
	eng.SetShell(ui, "ui/index.html")

	// Counter component — click-driven
	counter := &Counter{Step: 1}
	eng.Mount(counter, ui, "ui/counter/index.html")
	eng.BindToID(counter, "CounterSlot")

	// Clock component — goroutine-driven
	clock := &Clock{}
	eng.Mount(clock, ui, "ui/clock/index.html")
	eng.BindToID(clock, "ClockSlot")

	go clock.startClock()

	log.Fatal(eng.Start())
}
