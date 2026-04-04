package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

func main() {
	// Static file server on port 9090
	go func() {
		http.Handle("/", http.FileServer(http.FS(ui)))
		fmt.Println("static server at http://localhost:9090/ui/")
		log.Fatal(http.ListenAndServe(":9090", nil))
	}()

	// godom on a different port — register only, no mount
	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.Port = 9091
	eng.NoBrowser = true

	stocks := []StockInfo{
		{Symbol: "AAPL", Price: 189.50, Display: "AAPL $189.50", prevPrice: 189.50},
		{Symbol: "GOOGL", Price: 141.80, Display: "GOOGL $141.80", prevPrice: 141.80},
		{Symbol: "MSFT", Price: 415.20, Display: "MSFT $415.20", prevPrice: 415.20},
		{Symbol: "AMZN", Price: 178.90, Display: "AMZN $178.90", prevPrice: 178.90},
		{Symbol: "TSLA", Price: 175.40, Display: "TSLA $175.40", prevPrice: 175.40},
		{Symbol: "NVDA", Price: 875.30, Display: "NVDA $875.30", prevPrice: 875.30},
		{Symbol: "META", Price: 502.60, Display: "META $502.60", prevPrice: 502.60},
		{Symbol: "JPM", Price: 198.70, Display: "JPM $198.70", prevPrice: 198.70},
		{Symbol: "V", Price: 279.30, Display: "V $279.30", prevPrice: 279.30},
		{Symbol: "JNJ", Price: 156.20, Display: "JNJ $156.20", prevPrice: 156.20},
		{Symbol: "WMT", Price: 168.40, Display: "WMT $168.40", prevPrice: 168.40},
		{Symbol: "BRK.B", Price: 412.50, Display: "BRK.B $412.50", prevPrice: 412.50},
		{Symbol: "XOM", Price: 108.90, Display: "XOM $108.90", prevPrice: 108.90},
		{Symbol: "UNH", Price: 527.10, Display: "UNH $527.10", prevPrice: 527.10},
		{Symbol: "DIS", Price: 112.30, Display: "DIS $112.30", prevPrice: 112.30},
	}

	stock := &Stock{Stocks: stocks, Symbol: "AAPL", Price: "189.50"}
	stock.TargetName = "stock"
	stock.Template = "ui/stock/index.html"

	marquee := &Stock{Stocks: stocks, Symbol: "AAPL", Price: "189.50"}
	marquee.TargetName = "marquee"
	marquee.Template = "ui/stock/marquee.html"

	eng.Register(stock, marquee)

	go stock.startTicker()
	go marquee.startTicker()

	mux := http.NewServeMux()
	eng.SetMux(mux, nil)
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("godom at http://localhost:9091")
	log.Fatal(eng.ListenAndServe())
}
