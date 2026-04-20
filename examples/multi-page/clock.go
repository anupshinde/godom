package main

import (
	"time"

	"github.com/anupshinde/godom"
)

type Clock struct {
	godom.Island
	Time   string
	Date   string
	ticker *time.Ticker
}

func (c *Clock) startClock() {
	c.ticker = time.NewTicker(1 * time.Second)
	for range c.ticker.C {
		now := time.Now()
		c.Time = now.Format("15:04:05")
		c.Date = now.Format("Monday, January 2, 2006")
		c.Refresh()
	}
}

func (c *Clock) Cleanup() {
	if c.ticker != nil {
		c.ticker.Stop()
	}
}
