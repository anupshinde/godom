package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
)

// TickerStock is a single stock row in the ticker display.
type TickerStock struct {
	Symbol string
	Price  string
	Change string
	IsUp   bool
	IsDown bool
}

type tickerState struct {
	symbol string
	price  float64
	open   float64
}

// Ticker is a stock ticker component with random price updates.
type Ticker struct {
	godom.Component
	Stocks []TickerStock
	states []tickerState
}

func (t *Ticker) initStocks() {
	symbols := []struct {
		sym   string
		price float64
	}{
		{"AAPL", 189.50}, {"MSFT", 415.20}, {"GOOGL", 141.80},
		{"AMZN", 178.90}, {"NVDA", 875.30}, {"META", 502.60},
		{"TSLA", 175.40}, {"JPM", 198.70}, {"V", 279.30},
		{"JNJ", 156.20},
	}

	t.states = make([]tickerState, len(symbols))
	for i, s := range symbols {
		t.states[i] = tickerState{symbol: s.sym, price: s.price, open: s.price}
	}
	t.buildStocks()
}

func (t *Ticker) buildStocks() {
	result := make([]TickerStock, len(t.states))
	for i, s := range t.states {
		change := s.price - s.open
		isUp := change >= 0
		sign := ""
		if isUp {
			sign = "+"
		}
		result[i] = TickerStock{
			Symbol: s.symbol,
			Price:  fmt.Sprintf("$%.2f", s.price),
			Change: fmt.Sprintf("%s%.2f", sign, change),
			IsUp:   isUp,
			IsDown: !isUp,
		}
	}
	t.Stocks = result
}

func (t *Ticker) startTicker() {
	t.initStocks()
	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		for i := range t.states {
			changePct := (rand.Float64() - 0.48) * 0.03
			t.states[i].price = math.Round(t.states[i].price*(1+changePct)*100) / 100
		}
		t.buildStocks()
		t.Refresh()
	}
}
