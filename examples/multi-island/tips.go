package main

import (
	"time"

	"github.com/anupshinde/godom"
)

// Tips cycles through demo hints with a typing animation.
type Tips struct {
	godom.Island
	Text string
}

var tipMessages = []string{
	"Drag and drop the cards to reorder them",
	"Sidebar navigation only raises toast notifications in this demo",
	"The clock updates every second from a Go goroutine",
	"Stock prices are simulated with random walks",
	"System monitor uses the Chart.js plugin",
	"All UI state lives in Go — try refreshing the page",
	"Components communicate via callbacks — no shared pointers",
	"Built with godom — zero JavaScript written",
}

func (t *Tips) startTips() {
	idx := 0
	for {
		msg := tipMessages[idx]
		// Type out one rune at a time
		runes := []rune(msg)
		for i := 1; i <= len(runes); i++ {
			t.Text = string(runes[:i])
			t.Refresh()
			time.Sleep(35 * time.Millisecond)
		}
		// Pause to read
		time.Sleep(3 * time.Second)
		// Erase
		for i := len(runes); i >= 0; i-- {
			t.Text = string(runes[:i])
			t.Refresh()
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond)
		idx = (idx + 1) % len(tipMessages)
	}
}
