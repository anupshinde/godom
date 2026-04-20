package main

import "github.com/anupshinde/godom"

// SlotInfo describes a named slot in the layout.
type SlotInfo struct {
	RegisteredName string
	Title          string
}

// Layout is the root component that owns the page structure.
// Its Slots list drives a g-for loop with drag-to-reorder support.
type Layout struct {
	godom.Island
	Slots []SlotInfo
}

func (l *Layout) Reorder(from, to float64) {
	f, d := int(from), int(to)
	if f == d || f < 0 || d < 0 || f >= len(l.Slots) || d >= len(l.Slots) {
		return
	}
	item := l.Slots[f]
	l.Slots = append(l.Slots[:f], l.Slots[f+1:]...)
	l.Slots = append(l.Slots[:d], append([]SlotInfo{item}, l.Slots[d:]...)...)
}
