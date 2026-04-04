package main

import (
	"time"

	"github.com/anupshinde/godom"
)

type Clock struct {
	godom.Component
	Time string
}

func (c *Clock) Start() {
	go func() {
		for {
			c.Time = time.Now().Format("15:04:05")
			c.Refresh()
			time.Sleep(1 * time.Second)
		}
	}()
}
