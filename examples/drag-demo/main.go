package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type PaletteColor struct {
	Name string
	Hex  string
}

type Card struct {
	Color string
	Label string
}

type Demo struct {
	godom.Island
	Palette []PaletteColor
	Canvas  []Card
}

// Add is called when a palette item is dropped on the canvas.
// Receives: (paletteIndex, targetValue) from the drop handler.
func (d *Demo) Add(from, to float64) {
	i := int(from)
	if i < 0 || i >= len(d.Palette) {
		return
	}
	c := d.Palette[i]
	d.Canvas = append(d.Canvas, Card{Color: c.Hex, Label: c.Name})
}

// Reorder is called when a canvas card is dropped on another canvas card.
func (d *Demo) Reorder(from, to float64) {
	f, t := int(from), int(to)
	if f == t || f < 0 || t < 0 || f >= len(d.Canvas) || t >= len(d.Canvas) {
		return
	}
	item := d.Canvas[f]
	d.Canvas = append(d.Canvas[:f], d.Canvas[f+1:]...)
	d.Canvas = append(d.Canvas[:t], append([]Card{item}, d.Canvas[t:]...)...)
}

// Remove is called when a canvas card is dropped on the trash zone.
func (d *Demo) Remove(from, to float64) {
	f := int(from)
	if f < 0 || f >= len(d.Canvas) {
		return
	}
	d.Canvas = append(d.Canvas[:f], d.Canvas[f+1:]...)
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.Port = 8083
	demo := &Demo{
		Palette: []PaletteColor{
			{Name: "Red", Hex: "#e74c3c"},
			{Name: "Green", Hex: "#27ae60"},
			{Name: "Blue", Hex: "#2980b9"},
			{Name: "Orange", Hex: "#e67e22"},
			{Name: "Purple", Hex: "#8e44ad"},
			{Name: "Pink", Hex: "#e91e8b"},
		},
	}
	demo.Template = "ui/index.html"
	log.Fatal(eng.QuickServe(demo))
}
