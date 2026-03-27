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

//go:embed apexcharts-bridge.js
var apexBridgeJS string

type App struct {
	godom.Component
	TempChart  Chart
	StockChart Chart
}

func (a *App) startUpdates() {
	a.initCharts()

	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		label := time.Now().Format("15:04:05")

		a.TempChart.PushPoint(label, round1(20+rand.Float64()*15), 30)

		last := a.StockChart.Series[0]["data"].([]float64)
		lastVal := last[len(last)-1]
		next := round1(math.Max(0, lastVal+(rand.Float64()-0.48)*5))
		a.StockChart.PushPoint(label, next, 30)

		a.Refresh()
	}
}

func (a *App) initCharts() {
	tempData := make([]float64, 30)
	for i := range tempData {
		tempData[i] = round1(20 + rand.Float64()*15)
	}

	now := time.Now()
	labels := make([]string, 30)
	for i := range labels {
		labels[i] = now.Add(time.Duration(i-29) * time.Second).Format("15:04:05")
	}

	stockData := make([]float64, 30)
	stockData[0] = 100
	for i := 1; i < len(stockData); i++ {
		stockData[i] = round1(math.Max(0, stockData[i-1]+(rand.Float64()-0.48)*5))
	}

	xaxis := M{
		"categories": labels,
		"labels":     M{"style": M{"colors": "#666"}, "rotate": 0},
		"tickAmount": 8,
	}

	a.TempChart = Chart{
		Chart: M{
			"type":       "area",
			"height":     280,
			"animations": M{"enabled": false},
			"toolbar":    M{"show": false},
			"background": "transparent",
		},
		Series: []M{
			{"name": "Temperature (°C)", "data": tempData},
		},
		Xaxis:      xaxis,
		Yaxis:      M{"min": 15.0, "max": 40.0, "labels": M{"style": M{"colors": "#666"}}},
		DataLabels: M{"enabled": false},
		Colors:     []string{"#e94560"},
		Fill:       M{"type": "gradient", "gradient": M{"shadeIntensity": 1, "opacityFrom": 0.4, "opacityTo": 0.05}},
		Stroke:     M{"curve": "smooth", "width": 2},
		Grid:       M{"borderColor": "rgba(255,255,255,0.06)"},
		Theme:      M{"mode": "dark"},
		Tooltip:    M{"theme": "dark"},
	}

	a.StockChart = Chart{
		Chart: M{
			"type":       "line",
			"height":     280,
			"animations": M{"enabled": false},
			"toolbar":    M{"show": false},
			"background": "transparent",
		},
		Series: []M{
			{"name": "Price ($)", "data": stockData},
		},
		Xaxis:   xaxis,
		Yaxis:   M{"labels": M{"style": M{"colors": "#666"}}},
		Colors:  []string{"#6366f1"},
		Stroke:  M{"curve": "smooth", "width": 2},
		Grid:    M{"borderColor": "rgba(255,255,255,0.06)"},
		Theme:   M{"mode": "dark"},
		Tooltip: M{"theme": "dark"},
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func main() {
	eng := godom.NewEngine()
	eng.SetUI(ui)
	eng.RegisterPlugin("apexcharts", apexBridgeJS)

	root := &App{}
	go root.startUpdates()

	fmt.Println("ApexCharts demo — no plugin package, just inline bridge")
	eng.Mount(root, "ui/index.html")
	log.Fatal(eng.Start())
}
