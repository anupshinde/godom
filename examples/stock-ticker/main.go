package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Stock struct {
	Symbol        string
	Company       string
	Sector        string
	Price         string
	Change        string
	ChangePercent string
	Volume        string
	MarketCap     string
	High          string
	Low           string
	IsUp          bool
	IsDown        bool
	// internal, unexported
}

type stockState struct {
	symbol    string
	company   string
	sector    string
	basePrice float64
	price     float64
	open      float64
	high      float64
	low       float64
	volume    int64
	marketCap float64 // in billions
	shares    float64 // in billions
	// Per-stock tick timing
	tickInterval time.Duration // how often this stock updates
	nextTick     time.Time     // when it should next update
}

type App struct {
	godom.Component
	Stocks       []Stock
	MarketStatus string
	TotalStocks  string
	Gainers      string
	Losers       string
	LastUpdate   string
}

func newStock(symbol, company, sector string, price, shares float64) stockState {
	return stockState{symbol: symbol, company: company, sector: sector, basePrice: price, shares: shares}
}

var stocks = []stockState{
	newStock("AAPL", "Apple Inc.", "Technology", 189.50, 2.95),
	newStock("MSFT", "Microsoft Corp.", "Technology", 415.20, 7.43),
	newStock("GOOGL", "Alphabet Inc.", "Technology", 141.80, 12.26),
	newStock("AMZN", "Amazon.com Inc.", "Consumer", 178.90, 10.33),
	newStock("NVDA", "NVIDIA Corp.", "Technology", 875.30, 2.46),
	newStock("META", "Meta Platforms", "Technology", 502.60, 2.56),
	newStock("TSLA", "Tesla Inc.", "Automotive", 175.40, 3.19),
	newStock("BRK.B", "Berkshire Hathaway", "Finance", 408.50, 1.30),
	newStock("JPM", "JPMorgan Chase", "Finance", 198.70, 2.87),
	newStock("V", "Visa Inc.", "Finance", 279.30, 1.64),
	newStock("JNJ", "Johnson & Johnson", "Healthcare", 156.20, 2.41),
	newStock("WMT", "Walmart Inc.", "Consumer", 168.40, 2.69),
	newStock("PG", "Procter & Gamble", "Consumer", 162.80, 2.36),
	newStock("MA", "Mastercard Inc.", "Finance", 458.90, 0.93),
	newStock("HD", "Home Depot", "Consumer", 362.10, 0.99),
	newStock("XOM", "Exxon Mobil", "Energy", 104.30, 4.09),
	newStock("CVX", "Chevron Corp.", "Energy", 155.60, 1.87),
	newStock("LLY", "Eli Lilly & Co.", "Healthcare", 782.40, 0.95),
	newStock("ABBV", "AbbVie Inc.", "Healthcare", 170.50, 1.77),
	newStock("PFE", "Pfizer Inc.", "Healthcare", 28.60, 5.63),
	newStock("KO", "Coca-Cola Co.", "Consumer", 60.80, 4.32),
	newStock("PEP", "PepsiCo Inc.", "Consumer", 171.20, 1.37),
	newStock("COST", "Costco Wholesale", "Consumer", 728.50, 0.44),
	newStock("DIS", "Walt Disney Co.", "Media", 112.40, 1.83),
	newStock("NFLX", "Netflix Inc.", "Media", 628.70, 0.43),
	newStock("CRM", "Salesforce Inc.", "Technology", 272.30, 0.97),
	newStock("AMD", "Advanced Micro Devices", "Technology", 164.50, 1.62),
	newStock("INTC", "Intel Corp.", "Technology", 43.20, 4.18),
	newStock("BA", "Boeing Co.", "Industrial", 208.60, 0.60),
	newStock("GS", "Goldman Sachs", "Finance", 402.80, 0.34),
}

func initStocks() {
	now := time.Now()
	for i := range stocks {
		s := &stocks[i]
		s.price = s.basePrice
		s.open = s.basePrice
		s.high = s.basePrice
		s.low = s.basePrice
		s.volume = int64(rand.Intn(5000000)) + 1000000
		s.marketCap = s.price * s.shares

		// High-volume stocks tick fast (300ms), low-volume tick slow (up to 5s).
		// shares field roughly correlates with trading activity.
		switch {
		case s.shares > 5: // mega-cap, very active (GOOGL, AMZN, PFE, INTC)
			s.tickInterval = time.Duration(250+rand.Intn(200)) * time.Millisecond
		case s.shares > 2: // large-cap, active (AAPL, MSFT, TSLA, etc.)
			s.tickInterval = time.Duration(400+rand.Intn(600)) * time.Millisecond
		case s.shares > 1: // mid activity
			s.tickInterval = time.Duration(1000+rand.Intn(1500)) * time.Millisecond
		default: // lower volume (COST, NFLX, GS, MA, BA)
			s.tickInterval = time.Duration(2500+rand.Intn(2500)) * time.Millisecond
		}
		// Stagger initial ticks so they don't all fire at once
		s.nextTick = now.Add(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}
}

// tickPrices updates only stocks whose interval has elapsed. Returns true if anything changed.
func tickPrices() bool {
	now := time.Now()
	changed := false
	for i := range stocks {
		s := &stocks[i]
		if now.Before(s.nextTick) {
			continue
		}
		changed = true

		// Random walk: -1.5% to +1.5% per tick
		changePct := (rand.Float64() - 0.48) * 0.03 // slight upward bias
		s.price = s.price * (1 + changePct)
		s.price = math.Round(s.price*100) / 100

		if s.price > s.high {
			s.high = s.price
		}
		if s.price < s.low {
			s.low = s.price
		}
		s.volume += int64(rand.Intn(50000)) + 5000
		s.marketCap = s.price * s.shares

		// Schedule next tick with some jitter (±20%)
		jitter := float64(s.tickInterval) * (0.8 + rand.Float64()*0.4)
		s.nextTick = now.Add(time.Duration(jitter))
	}
	return changed
}

func formatVolume(v int64) string {
	if v >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(v)/1_000_000)
	}
	if v >= 1_000 {
		return fmt.Sprintf("%.0fK", float64(v)/1_000)
	}
	return fmt.Sprintf("%d", v)
}

func formatMarketCap(billions float64) string {
	if billions >= 1000 {
		return fmt.Sprintf("$%.2fT", billions/1000)
	}
	return fmt.Sprintf("$%.1fB", billions)
}

func (a *App) buildStocks() {
	gainers := 0
	losers := 0
	result := make([]Stock, len(stocks))
	for i, s := range stocks {
		change := s.price - s.open
		changePct := (change / s.open) * 100
		isUp := change >= 0
		if isUp {
			gainers++
		} else {
			losers++
		}

		sign := ""
		if isUp {
			sign = "+"
		}

		result[i] = Stock{
			Symbol:        s.symbol,
			Company:       s.company,
			Sector:        s.sector,
			Price:         fmt.Sprintf("$%.2f", s.price),
			Change:        fmt.Sprintf("%s%.2f", sign, change),
			ChangePercent: fmt.Sprintf("%s%.2f%%", sign, changePct),
			Volume:        formatVolume(s.volume),
			MarketCap:     formatMarketCap(s.marketCap),
			High:          fmt.Sprintf("$%.2f", s.high),
			Low:           fmt.Sprintf("$%.2f", s.low),
			IsUp:          isUp,
			IsDown:        !isUp,
		}
	}
	a.Stocks = result
	a.TotalStocks = fmt.Sprintf("%d", len(stocks))
	a.Gainers = fmt.Sprintf("%d", gainers)
	a.Losers = fmt.Sprintf("%d", losers)
	a.LastUpdate = time.Now().Format("15:04:05")
}

func (a *App) startTicker() {
	initStocks()
	a.MarketStatus = "OPEN"
	a.buildStocks()

	// Fast base loop — individual stocks tick at their own rates
	ticker := time.NewTicker(200 * time.Millisecond)
	for range ticker.C {
		if tickPrices() {
			a.buildStocks()
			a.Refresh()
		}
	}
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	root := &App{}
	root.Template = "ui/index.html"
	go root.startTicker()
	log.Fatal(eng.QuickServe(root))
}
