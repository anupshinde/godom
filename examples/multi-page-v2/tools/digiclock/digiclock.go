// Package digiclock is a minimal digital clock island. It demonstrates
// godom.Island.TemplateHTML — an inline HTML template with no filesystem.
// Use it for very small islands where the ergonomics of a tiny .html file
// feel like overkill.
package digiclock

import (
	"time"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/counter"
)

// Inline template. Because this is all we need, there's no .html file and
// no embed.FS — just a string literal. Shared partials (RegisterPartial /
// UsePartials) still work; local sibling partials do not (no FS).
const tmpl = `<span class="font-mono text-blue-600 font-medium"><span g-text="Time">--:--:--</span>, with count <span g-text="Count">0</span></span>`

type DigiClock struct {
	godom.Island
	*counter.State // shared counter — auto-refreshed when Counter increments
	Time           string
}

func (c *DigiClock) tick() {
	c.Time = time.Now().Format("15:04:05")
}

func (c *DigiClock) Run() {
	t := time.NewTicker(time.Second)
	for range t.C {
		old := c.Time
		c.tick()
		if c.Time != old {
			c.Refresh()
		}
	}
}

func New(s *counter.State) *DigiClock {
	c := &DigiClock{
		Island: godom.Island{
			TargetName:   "digiclock",
			TemplateHTML: tmpl,
		},
		State: s,
	}
	c.tick()
	return c
}
