package main

import "github.com/anupshinde/godom"

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
