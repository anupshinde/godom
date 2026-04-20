package clock

import (
	"fmt"
	"time"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/counter"
)

type Clock struct {
	godom.Island
	*counter.State // shared with Counter — displays Count/Step alongside time
	Time           string
	HourHand       string
	MinuteHand     string
	SecondHand     string
}

func (c *Clock) tick() {
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

func (c *Clock) Run() {
	ticker := time.NewTicker(50 * time.Millisecond)
	for range ticker.C {
		old := c.Time
		c.tick()
		if c.Time != old {
			c.Refresh()
		}
	}
}

// New takes the shared counter state for display. Pass nil if not sharing.
func New(s *counter.State) *Clock {
	c := &Clock{
		Island: godom.Island{
			TargetName: "clock",
			Template:   "island-templates/clock/index.html",
		},
		State: s,
	}
	c.tick()
	return c
}
