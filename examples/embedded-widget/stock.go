package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
)

type StockInfo struct {
	Symbol   string
	Price    float64
	Display  string
	IsUp     bool
	IsDown   bool
	prevPrice float64
}

type Stock struct {
	godom.Island
	Stocks  []StockInfo
	Index   int
	Symbol  string
	Price   string
	IsUp    bool
	IsDown  bool
	Marquee string
}

func (s *Stock) update() {
	si := s.Stocks[s.Index]
	oldPrice := s.Price
	s.Symbol = si.Symbol
	s.Price = fmt.Sprintf("%.2f", si.Price)
	s.IsUp = s.Price > oldPrice
	s.IsDown = s.Price < oldPrice
	s.updateStockDisplays()
}

func (s *Stock) updateStockDisplays() {
	for i := range s.Stocks {
		si := &s.Stocks[i]
		si.Display = fmt.Sprintf("%s $%.2f", si.Symbol, si.Price)
		si.IsUp = si.Price > si.prevPrice
		si.IsDown = si.Price < si.prevPrice
		si.prevPrice = si.Price
	}
}

func (s *Stock) Prev() {
	s.Index--
	if s.Index < 0 {
		s.Index = len(s.Stocks) - 1
	}
	s.update()
}

func (s *Stock) Next() {
	s.Index++
	if s.Index >= len(s.Stocks) {
		s.Index = 0
	}
	s.update()
}

func (s *Stock) startTicker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		for i := range s.Stocks {
			changePct := (rand.Float64() - 0.48) * 0.03
			s.Stocks[i].Price = math.Round(s.Stocks[i].Price*(1+changePct)*100) / 100
		}
		s.update()
		s.Refresh()
	}
}
