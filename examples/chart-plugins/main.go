package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/plugins/echarts"
	"github.com/anupshinde/godom/plugins/plotly"
)

//go:embed ui
var ui embed.FS

type M = map[string]interface{}

const maxPoints = 40

type App struct {
	godom.Component

	// Plotly charts
	PlotlyLine    plotly.Chart
	PlotlyBar     plotly.Chart
	PlotlyScatter plotly.Chart

	// ECharts charts
	EChartsLine echarts.Chart
	EChartsBar  echarts.Chart
	EChartsPie  echarts.Chart

	// Shared data buffers
	labels []string
	temps  []float64
	humid  []float64
	wind   []float64
	press  []float64

	Tick int
}

func (a *App) startSimulation() {
	a.initCharts()

	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		a.Tick++
		label := time.Now().Format("15:04:05")

		// Simulate weather-station-like data
		temp := 20 + 8*math.Sin(float64(a.Tick)/20) + rand.Float64()*2
		humidity := 50 + 15*math.Cos(float64(a.Tick)/25) + rand.Float64()*3
		windSpeed := 5 + 3*math.Sin(float64(a.Tick)/10) + rand.Float64()*2
		pressure := 1013 + 5*math.Sin(float64(a.Tick)/30) + rand.Float64()

		a.labels = append(a.labels, label)
		a.temps = append(a.temps, round1(temp))
		a.humid = append(a.humid, round1(humidity))
		a.wind = append(a.wind, round1(windSpeed))
		a.press = append(a.press, round1(pressure))

		if len(a.labels) > maxPoints {
			a.labels = a.labels[1:]
			a.temps = a.temps[1:]
			a.humid = a.humid[1:]
			a.wind = a.wind[1:]
			a.press = a.press[1:]
		}

		a.updatePlotlyCharts()
		a.updateEChartsCharts()
		a.Refresh()
	}
}

func (a *App) initCharts() {
	// --- Plotly charts ---
	a.PlotlyLine = plotly.Chart{
		Data: []M{
			{"x": []string{}, "y": []float64{}, "type": "scatter", "mode": "lines", "name": "Temperature", "line": M{"color": "#e94560", "width": 2}},
			{"x": []string{}, "y": []float64{}, "type": "scatter", "mode": "lines", "name": "Humidity", "line": M{"color": "#6366f1", "width": 2}, "yaxis": "y2"},
		},
		Layout: M{
			"paper_bgcolor": "rgba(0,0,0,0)",
			"plot_bgcolor":  "rgba(0,0,0,0)",
			"font":          M{"color": "#999"},
			"margin":        M{"l": 50, "r": 50, "t": 10, "b": 40},
			"yaxis":         M{"title": "Temp (C)", "titlefont": M{"color": "#e94560"}, "tickfont": M{"color": "#e94560"}, "gridcolor": "rgba(255,255,255,0.06)"},
			"yaxis2":        M{"title": "Humidity (%)", "titlefont": M{"color": "#6366f1"}, "tickfont": M{"color": "#6366f1"}, "overlaying": "y", "side": "right", "gridcolor": "rgba(255,255,255,0.06)"},
			"xaxis":         M{"gridcolor": "rgba(255,255,255,0.06)"},
			"legend":        M{"x": 0, "y": 1.1, "orientation": "h"},
			"showlegend":    true,
		},
		Config: M{"responsive": true, "displayModeBar": false},
	}

	a.PlotlyBar = plotly.Chart{
		Data: []M{
			{"x": []string{}, "y": []float64{}, "type": "bar", "name": "Wind Speed", "marker": M{"color": "#10b981"}},
		},
		Layout: M{
			"paper_bgcolor": "rgba(0,0,0,0)",
			"plot_bgcolor":  "rgba(0,0,0,0)",
			"font":          M{"color": "#999"},
			"margin":        M{"l": 50, "r": 20, "t": 10, "b": 40},
			"yaxis":         M{"title": "m/s", "gridcolor": "rgba(255,255,255,0.06)"},
			"xaxis":         M{"gridcolor": "rgba(255,255,255,0.06)"},
			"showlegend":    false,
		},
		Config: M{"responsive": true, "displayModeBar": false},
	}

	a.PlotlyScatter = plotly.Chart{
		Data: []M{
			{"x": []float64{}, "y": []float64{}, "type": "scatter", "mode": "markers", "name": "Temp vs Humidity",
				"marker": M{"color": []float64{}, "colorscale": "Viridis", "showscale": true, "size": 8,
					"colorbar": M{"title": M{"text": "Wind"}, "tickfont": M{"color": "#999"}, "titlefont": M{"color": "#999"}}}},
		},
		Layout: M{
			"paper_bgcolor": "rgba(0,0,0,0)",
			"plot_bgcolor":  "rgba(0,0,0,0)",
			"font":          M{"color": "#999"},
			"margin":        M{"l": 50, "r": 20, "t": 10, "b": 40},
			"xaxis":         M{"title": "Temperature (C)", "gridcolor": "rgba(255,255,255,0.06)"},
			"yaxis":         M{"title": "Humidity (%)", "gridcolor": "rgba(255,255,255,0.06)"},
			"showlegend":    false,
		},
		Config: M{"responsive": true, "displayModeBar": false},
	}

	// --- ECharts charts ---
	a.EChartsLine = echarts.Chart{
		Color:   []string{"#f59e0b", "#8b5cf6"},
		Tooltip: M{"trigger": "axis"},
		Legend:  M{"data": []string{"Pressure", "Wind"}, "textStyle": M{"color": "#999"}, "top": "0"},
		Grid:    M{"left": "50", "right": "50", "top": "30", "bottom": "30"},
		XAxis:   M{"type": "category", "data": []string{}, "axisLabel": M{"color": "#666"}, "axisLine": M{"lineStyle": M{"color": "#333"}}},
		YAxis:   M{"type": "value", "axisLabel": M{"color": "#666"}, "splitLine": M{"lineStyle": M{"color": "rgba(255,255,255,0.06)"}}},
		Series: []M{
			{"name": "Pressure", "type": "line", "data": []float64{}, "smooth": true, "showSymbol": false, "areaStyle": M{"opacity": 0.1}},
			{"name": "Wind", "type": "line", "data": []float64{}, "smooth": true, "showSymbol": false, "areaStyle": M{"opacity": 0.1}},
		},
	}

	a.EChartsBar = echarts.Chart{
		Color:   []string{"#e94560", "#6366f1", "#10b981"},
		Tooltip: M{"trigger": "axis"},
		Legend:  M{"data": []string{"Temp", "Humidity", "Wind"}, "textStyle": M{"color": "#999"}, "top": "0"},
		Grid:    M{"left": "40", "right": "20", "top": "30", "bottom": "30"},
		XAxis:   M{"type": "category", "data": []string{}, "axisLabel": M{"color": "#666"}, "axisLine": M{"lineStyle": M{"color": "#333"}}},
		YAxis:   M{"type": "value", "axisLabel": M{"color": "#666"}, "splitLine": M{"lineStyle": M{"color": "rgba(255,255,255,0.06)"}}},
		Series: []M{
			{"name": "Temp", "type": "bar", "data": []float64{}},
			{"name": "Humidity", "type": "bar", "data": []float64{}},
			{"name": "Wind", "type": "bar", "data": []float64{}},
		},
	}

	a.EChartsPie = echarts.Chart{
		Tooltip: M{"trigger": "item"},
		Color:   []string{"#e94560", "#6366f1", "#10b981", "#f59e0b"},
		Series: []M{
			{
				"type":   "pie",
				"radius": []string{"40%", "70%"},
				"center": []string{"50%", "55%"},
				"label":  M{"color": "#999"},
				"data": []M{
					{"value": 0, "name": "Temperature"},
					{"value": 0, "name": "Humidity"},
					{"value": 0, "name": "Wind Speed"},
					{"value": 0, "name": "Pressure Δ"},
				},
			},
		},
	}
}

func (a *App) updatePlotlyCharts() {
	// Line: temperature + humidity (dual axis)
	a.PlotlyLine.Data[0]["x"] = a.labels
	a.PlotlyLine.Data[0]["y"] = a.temps
	a.PlotlyLine.Data[1]["x"] = a.labels
	a.PlotlyLine.Data[1]["y"] = a.humid

	// Bar: wind speed (last 10 points)
	n := len(a.labels)
	start := 0
	if n > 10 {
		start = n - 10
	}
	a.PlotlyBar.Data[0]["x"] = a.labels[start:]
	a.PlotlyBar.Data[0]["y"] = a.wind[start:]

	// Scatter: temp vs humidity, colored by wind
	a.PlotlyScatter.Data[0]["x"] = a.temps
	a.PlotlyScatter.Data[0]["y"] = a.humid
	a.PlotlyScatter.Data[0]["marker"].(M)["color"] = a.wind
}

func (a *App) updateEChartsCharts() {
	// Line: pressure + wind over time
	a.EChartsLine.XAxis["data"] = a.labels
	a.EChartsLine.Series[0]["data"] = a.press
	a.EChartsLine.Series[1]["data"] = a.wind

	// Bar: last 10 readings
	n := len(a.labels)
	start := 0
	if n > 10 {
		start = n - 10
	}
	a.EChartsBar.XAxis["data"] = a.labels[start:]
	a.EChartsBar.Series[0]["data"] = a.temps[start:]
	a.EChartsBar.Series[1]["data"] = a.humid[start:]
	a.EChartsBar.Series[2]["data"] = a.wind[start:]

	// Pie: latest readings as proportions
	if n > 0 {
		a.EChartsPie.Series[0]["data"] = []M{
			{"value": a.temps[n-1], "name": "Temperature"},
			{"value": a.humid[n-1], "name": "Humidity"},
			{"value": a.wind[n-1] * 5, "name": "Wind Speed"},
			{"value": math.Abs(a.press[n-1] - 1013), "name": "Pressure Δ"},
		}
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.Use(plotly.Plugin, echarts.Plugin)

	root := &App{}
	root.Template = "ui/index.html"
	go root.startSimulation()

	fmt.Println("Chart Plugins Demo — Plotly + ECharts with live weather simulation")
	log.Fatal(eng.QuickServe(root))
}
