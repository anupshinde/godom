package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Card struct {
	Color string
	Label string
}

type Demo struct {
	godom.Component
	Canvas []Card
}

// Add is called when a palette item is dropped on the canvas.
// Receives: from (string color name), to (dropzone value), position.
func (d *Demo) Add(color string) {
	labels := map[string]string{
		"red": "Red", "green": "Green", "blue": "Blue",
		"orange": "Orange", "purple": "Purple", "pink": "Pink",
	}
	d.Canvas = append(d.Canvas, Card{
		Color: color,
		Label: labels[color],
	})
}

// Reorder is called when a canvas card is dropped on another canvas card.
func (d *Demo) Reorder(from, to float64, position string) {
	f, t := int(from), int(to)
	if f == t || f < 0 || t < 0 || f >= len(d.Canvas) || t >= len(d.Canvas) {
		return
	}
	item := d.Canvas[f]
	d.Canvas = append(d.Canvas[:f], d.Canvas[f+1:]...)
	// If dropping below the target, insert after it
	if position == "below" && t > f {
		// t already adjusted by removal
	} else if position == "below" && t <= f {
		t++
		if t > len(d.Canvas) {
			t = len(d.Canvas)
		}
	}
	d.Canvas = append(d.Canvas[:t], append([]Card{item}, d.Canvas[t:]...)...)
}

// Remove is called when a canvas card is dropped on the trash zone.
func (d *Demo) Remove(from float64) {
	f := int(from)
	if f < 0 || f >= len(d.Canvas) {
		return
	}
	d.Canvas = append(d.Canvas[:f], d.Canvas[f+1:]...)
}

func main() {
	app := godom.New()
	app.Port = 8083
	app.Mount(&Demo{}, ui)
	log.Fatal(app.Start())
}
