package main

import (
	"embed"
	"fmt"
	"log"
	"math"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

// ---------------------------------------------------------------------------
// Color Picker component — manages its own complex UI state
// ---------------------------------------------------------------------------

type ColorPicker struct {
	godom.Component
	Hue float64 // 0–360
	Sat float64 // 0–100
	Lit float64 // 0–100
}

func (c *ColorPicker) SetHue(h float64) {
	c.Hue = math.Max(0, math.Min(360, h))
}

func (c *ColorPicker) SetSat(s float64) {
	c.Sat = math.Max(0, math.Min(100, s))
}

func (c *ColorPicker) SetLit(l float64) {
	c.Lit = math.Max(0, math.Min(100, l))
}

// Value is the component's bound output — what g-bind reads.
func (c *ColorPicker) Value() string {
	return hslToHex(c.Hue, c.Sat, c.Lit)
}

// ---------------------------------------------------------------------------
// Main app — owns the color list
// ---------------------------------------------------------------------------

type App struct {
	godom.Component
	NewColor string   // bound to the color picker
	Colors   []string // the list on the left
}

func (a *App) HasColors() bool {
	return len(a.Colors) > 0
}

func (a *App) AddColor() {
	if a.NewColor == "" {
		return
	}
	a.Colors = append(a.Colors, a.NewColor)
}

func (a *App) RemoveColor(i int) {
	a.Colors = append(a.Colors[:i], a.Colors[i+1:]...)
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	eng := godom.NewEngine()
	eng.RegisterComponent("color-picker", &ColorPicker{Hue: 200, Sat: 70, Lit: 50})
	eng.Mount(&App{}, ui, "ui/index.html")
	log.Fatal(eng.Start())
}

// ---------------------------------------------------------------------------
// HSL → Hex conversion
// ---------------------------------------------------------------------------

func hslToHex(h, s, l float64) string {
	s /= 100
	l /= 100

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	ri := int(math.Round((r + m) * 255))
	gi := int(math.Round((g + m) * 255))
	bi := int(math.Round((b + m) * 255))

	return fmt.Sprintf("#%02x%02x%02x", ri, gi, bi)
}
