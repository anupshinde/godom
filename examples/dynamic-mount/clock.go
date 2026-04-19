package main

import (
	"time"

	"github.com/anupshinde/godom"
)

type Clock struct {
	godom.Island
	Time string
}

func (c *Clock) startClock() {
	now := time.Now()
	c.Time = now.Format("15:04:05")

	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		c.Time = time.Now().Format("15:04:05")
		c.Refresh()
	}
}
