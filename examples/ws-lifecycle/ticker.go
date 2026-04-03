package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
)

// Ticker is a background component — registered but not rendered on the page.
// It proves godom is alive even when no g-component target exists in the DOM.
// In the future, dynamic mount (godom.mount) would allow showing it on demand.
type Ticker struct {
	godom.Component
	Symbol string
	Price  float64
	Change float64
	price  float64
}

func NewTicker() *Ticker {
	return &Ticker{
		Symbol: "GODOM",
		price:  100.0,
		Price:  100.0,
	}
}

func (t *Ticker) Run() {
	tick := time.NewTicker(1 * time.Second)
	for range tick.C {
		change := (rand.Float64() - 0.48) * 2
		t.price = math.Max(1, t.price+change)
		t.Price = math.Round(t.price*100) / 100
		t.Change = math.Round(change*100) / 100
		fmt.Printf("[ticker] %s $%.2f (%+.2f)\n", t.Symbol, t.Price, t.Change)
		t.Refresh()
	}
}
