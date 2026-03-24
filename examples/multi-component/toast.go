package main

import (
	"fmt"
	"time"

	"github.com/anupshinde/godom"
)

// ToastItem is a single toast message in the queue.
type ToastItem struct {
	ID      string
	Message string
	IsInfo  bool
	IsOk    bool
	IsWarn  bool
	IsError bool
	Closing bool
}

// Toast is a stacking toast notification component.
type Toast struct {
	godom.Component
	Items   []ToastItem
	counter int
}

func (t *Toast) Show(message, kind string) {
	t.counter++
	id := fmt.Sprintf("toast-%d", t.counter)
	item := ToastItem{ID: id, Message: message}
	switch kind {
	case "info":
		item.IsInfo = true
	case "success":
		item.IsOk = true
	case "warning":
		item.IsWarn = true
	case "error":
		item.IsError = true
	default:
		item.IsInfo = true
	}
	t.Items = append(t.Items, item)
	t.Refresh()
	go func() {
		// After visible duration, start closing animation
		time.Sleep(5 * time.Second)
		t.markClosing(id)
		// After animation completes, remove from list
		time.Sleep(400 * time.Millisecond)
		t.remove(id)
	}()
}

func (t *Toast) Dismiss(id string) {
	t.markClosing(id)
	go func() {
		time.Sleep(400 * time.Millisecond)
		t.remove(id)
	}()
}

func (t *Toast) markClosing(id string) {
	for i := range t.Items {
		if t.Items[i].ID == id {
			t.Items[i].Closing = true
			t.Refresh()
			return
		}
	}
}

func (t *Toast) remove(id string) {
	for i := range t.Items {
		if t.Items[i].ID == id {
			t.Items = append(t.Items[:i], t.Items[i+1:]...)
			t.Refresh()
			return
		}
	}
}
