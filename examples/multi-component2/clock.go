package main

import (
	"time"

	"github.com/anupshinde/godom"
)

type Clock struct {
	godom.Component
	Time string
	Date string
}

func (c *Clock) startClock() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		now := time.Now()
		c.Time = now.Format("15:04:05")
		c.Date = now.Format("Monday, January 2, 2006")
		c.Refresh()
	}
}
