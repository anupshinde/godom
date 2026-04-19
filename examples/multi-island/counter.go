package main

import "github.com/anupshinde/godom"

// CounterState holds shared counter state that multiple components can reference.
type CounterState struct {
	Count int
	Step  int
}

// Counter is a click-driven component with increment/decrement buttons.
type Counter struct {
	godom.Island
	*CounterState
}

func (c *Counter) Increment() {
	c.Count += c.Step
}

func (c *Counter) Decrement() {
	c.Count -= c.Step
}
