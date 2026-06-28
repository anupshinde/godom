// Computed fields — derived values the engine keeps in sync for you.
//
// Subtotal, Tax, Total, and the checkout button's state are all DERIVED from the
// two inputs (Qty and UnitPrice). We never recompute them by hand in a handler:
// Compute() declares how each is derived and which fields it depends on, and the
// engine recomputes it (in dependency order) and surgically patches its bound
// nodes whenever a dependency changes.
package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Cart struct {
	godom.Island

	// Inputs — edited in the browser via g-bind.
	Qty       int
	UnitPrice int // whole dollars, for a simple demo

	// Computed fields — maintained by the engine; never assigned by hand.
	Subtotal        int    // Qty * UnitPrice
	SubtotalText    string // "$x"
	TaxText         string // 8% of subtotal
	TotalText       string // subtotal + tax
	CheckoutEnabled bool   // Qty > 0
}

func dollars(n int) string { return fmt.Sprintf("$%d", n) }

// defineComputeds declares the derived fields. Call it before Register/QuickServe
// (godom does not auto-run it). Note SubtotalText/TaxText/TotalText depend on the
// Subtotal computed — the engine resolves the dependency order automatically.
func (c *Cart) defineComputeds() {
	c.Compute("Subtotal", func() any { return c.Qty * c.UnitPrice }, "Qty", "UnitPrice")
	c.Compute("SubtotalText", func() any { return dollars(c.Subtotal) }, "Subtotal")
	c.Compute("TaxText", func() any { return dollars(c.Subtotal * 8 / 100) }, "Subtotal")
	c.Compute("TotalText", func() any { return dollars(c.Subtotal + c.Subtotal*8/100) }, "Subtotal")
	c.Compute("CheckoutEnabled", func() any { return c.Qty > 0 }, "Qty")
}

func main() {
	cart := &Cart{Qty: 2, UnitPrice: 20}
	cart.Template = "ui/index.html"
	cart.defineComputeds() // before QuickServe (which registers the island)

	eng := godom.NewEngine()
	eng.SetFS(ui)
	log.Fatal(eng.QuickServe(cart))
}
