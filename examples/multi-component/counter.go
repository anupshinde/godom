package main

import "github.com/anupshinde/godom"

// CounterState holds shared counter state that multiple components can reference.
type CounterState struct {
	Count int
	Step  int
}

// Counter is a click-driven component with increment/decrement buttons.
type Counter struct {
	godom.Component
	*CounterState
	Display *CounterDisplay // sibling that shares our state
}

func (c *Counter) Increment() {
	c.Count += c.Step
	c.Display.Refresh()
}

func (c *Counter) Decrement() {
	c.Count -= c.Step
	c.Display.Refresh()
}

// CounterDisplay is a read-only view of the shared counter state.
type CounterDisplay struct {
	godom.Component
	*CounterState
}
