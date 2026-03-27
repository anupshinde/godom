package main

import "github.com/anupshinde/godom"

// CounterState holds shared counter state that multiple components can reference.
type CounterState struct {
	Count int
	Step  int
}

// Counter is a click-driven component. Multiple instances can share the same
// CounterState and each can have a different template.
type Counter struct {
	godom.Component
	*CounterState
}

func (c *Counter) Increment() {
	c.Count += c.Step
}

func (c *Counter) Decrement() {
	c.Count -= c.Step
}

// CounterDisplay is a read-only view of counter state — a different type
// sharing the same CounterState pointer.
type CounterDisplay struct {
	godom.Component
	*CounterState
}
