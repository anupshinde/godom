package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/plugins/chartjs"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

//go:embed ui
var ui embed.FS

const maxPoints = 60

var coreColors = []string{
	"#e94560", "#6366f1", "#10b981", "#f59e0b",
	"#ec4899", "#8b5cf6", "#14b8a6", "#f97316",
	"#ef4444", "#3b82f6", "#22c55e", "#eab308",
	"#d946ef", "#0ea5e9", "#84cc16", "#fb923c",
}

type M = map[string]interface{}

type App struct {
	godom.Component
	Hostname     string
	OS           string
	Platform     string
	Kernel       string
	Uptime       string
	CPUChart     chartjs.Chart
	CPUCoreChart chartjs.Chart
	MemoryChart  chartjs.Chart
	DiskChart    chartjs.Chart
	SwapChart    chartjs.Chart
	LoadChart    chartjs.Chart
}

func (a *App) startMonitor() {
	a.initHostInfo()
	a.initCharts()

	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		label := time.Now().Format("15:04:05")

		if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
			pushPoint(&a.CPUChart, label, round1(pcts[0]))
		}
		if pcts, err := cpu.Percent(0, true); err == nil {
			a.updateCoreChart(label, pcts)
		}
		if vm, err := mem.VirtualMemory(); err == nil {
			pushPoint(&a.MemoryChart, label, round1(vm.UsedPercent))
		}
		if du, err := disk.Usage("/"); err == nil {
			usedGB := float64(du.Used) / (1 << 30)
			freeGB := float64(du.Free) / (1 << 30)
			a.DiskChart.Datasets[0]["data"] = []float64{round1(usedGB), round1(freeGB)}
		}
		if sw, err := mem.SwapMemory(); err == nil {
			usedGB := float64(sw.Used) / (1 << 30)
			freeGB := float64(sw.Total-sw.Used) / (1 << 30)
			a.SwapChart.Datasets[0]["data"] = []float64{round1(usedGB), round1(freeGB)}
		}
		if la, err := load.Avg(); err == nil {
			pushPointMulti(&a.LoadChart, label, []float64{
				round1(la.Load1), round1(la.Load5), round1(la.Load15),
			})
		}
		if hi, err := host.Info(); err == nil {
			a.Uptime = formatUptime(hi.Uptime)
		}

		a.Refresh()
	}
}

func (a *App) initHostInfo() {
	if hi, err := host.Info(); err == nil {
		a.Hostname = hi.Hostname
		a.OS = hi.OS
		a.Platform = fmt.Sprintf("%s %s", hi.Platform, hi.PlatformVersion)
		a.Kernel = hi.KernelVersion
		a.Uptime = formatUptime(hi.Uptime)
	}
}

func (a *App) initCharts() {
	lineOpts := func(yMax interface{}) M {
		yAxis := M{
			"min":   0,
			"grid":  M{"color": "rgba(255,255,255,0.06)"},
			"ticks": M{"color": "#666"},
		}
		if yMax != nil {
			yAxis["max"] = yMax
		}
		return M{
			"responsive":          true,
			"maintainAspectRatio": false,
			"animation":           M{"duration": 0},
			"scales": M{
				"x": M{
					"grid":  M{"color": "rgba(255,255,255,0.06)"},
					"ticks": M{"color": "#666", "maxTicksLimit": 6},
				},
				"y": yAxis,
			},
			"plugins": M{
				"legend": M{
					"display": true,
					"labels":  M{"color": "#999"},
				},
			},
		}
	}

	doughnutOpts := M{
		"responsive":          true,
		"maintainAspectRatio": false,
		"cutout":              "70%",
		"plugins": M{
			"legend": M{
				"display": true,
				"labels":  M{"color": "#999"},
			},
		},
	}

	a.CPUChart = chartjs.Chart{
		Type: "line",
		Datasets: []M{{
			"label":           "CPU %",
			"borderColor":     "#e94560",
			"backgroundColor": "rgba(233, 69, 96, 0.15)",
			"borderWidth":     2,
			"fill":            true,
			"tension":         0.3,
			"pointRadius":     0,
			"data":            []float64{},
		}},
		Options: lineOpts(100),
	}

	numCores := runtime.NumCPU()
	coreDatasets := make([]M, numCores)
	for i := 0; i < numCores; i++ {
		coreDatasets[i] = M{
			"label":       fmt.Sprintf("Core %d", i),
			"borderColor": coreColors[i%len(coreColors)],
			"borderWidth": 1.5,
			"fill":        false,
			"tension":     0.3,
			"pointRadius": 0,
			"data":        []float64{},
		}
	}
	coreOpts := lineOpts(100)
	coreOpts["plugins"] = M{"legend": M{"display": false}}
	a.CPUCoreChart = chartjs.Chart{
		Type:     "line",
		Datasets: coreDatasets,
		Options:  coreOpts,
	}

	a.MemoryChart = chartjs.Chart{
		Type: "line",
		Datasets: []M{{
			"label":           "Memory %",
			"borderColor":     "#6366f1",
			"backgroundColor": "rgba(99, 102, 241, 0.15)",
			"borderWidth":     2,
			"fill":            true,
			"tension":         0.3,
			"pointRadius":     0,
			"data":            []float64{},
		}},
		Options: lineOpts(100),
	}

	a.DiskChart = chartjs.Chart{
		Type:   "doughnut",
		Labels: []string{"Used", "Free"},
		Datasets: []M{{
			"data":            []float64{0, 0},
			"backgroundColor": []string{"#e94560", "rgba(30, 58, 95, 0.5)"},
			"borderWidth":     0,
		}},
		Options: doughnutOpts,
	}

	a.SwapChart = chartjs.Chart{
		Type:   "doughnut",
		Labels: []string{"Used", "Free"},
		Datasets: []M{{
			"data":            []float64{0, 0},
			"backgroundColor": []string{"#8b5cf6", "rgba(30, 58, 95, 0.5)"},
			"borderWidth":     0,
		}},
		Options: doughnutOpts,
	}

	a.LoadChart = chartjs.Chart{
		Type: "line",
		Datasets: []M{
			{"label": "1 min", "borderColor": "#10b981", "backgroundColor": "rgba(16, 185, 129, 0.1)", "borderWidth": 2, "fill": true, "tension": 0.3, "pointRadius": 0, "data": []float64{}},
			{"label": "5 min", "borderColor": "#f59e0b", "backgroundColor": "rgba(245, 158, 11, 0.1)", "borderWidth": 2, "fill": true, "tension": 0.3, "pointRadius": 0, "data": []float64{}},
			{"label": "15 min", "borderColor": "#ef4444", "backgroundColor": "rgba(239, 68, 68, 0.1)", "borderWidth": 2, "fill": true, "tension": 0.3, "pointRadius": 0, "data": []float64{}},
		},
		Options: lineOpts(nil),
	}
}

func (a *App) updateCoreChart(label string, pcts []float64) {
	a.CPUCoreChart.Labels = append(a.CPUCoreChart.Labels, label)
	if len(a.CPUCoreChart.Labels) > maxPoints {
		a.CPUCoreChart.Labels = a.CPUCoreChart.Labels[1:]
	}
	for i, ds := range a.CPUCoreChart.Datasets {
		val := 0.0
		if i < len(pcts) {
			val = round1(pcts[i])
		}
		data := ds["data"].([]float64)
		data = append(data, val)
		if len(data) > maxPoints {
			data = data[1:]
		}
		a.CPUCoreChart.Datasets[i]["data"] = data
	}
}

func pushPoint(c *chartjs.Chart, label string, value float64) {
	c.Labels = append(c.Labels, label)
	data := c.Datasets[0]["data"].([]float64)
	data = append(data, value)
	if len(c.Labels) > maxPoints {
		c.Labels = c.Labels[1:]
		data = data[1:]
	}
	c.Datasets[0]["data"] = data
}

func pushPointMulti(c *chartjs.Chart, label string, values []float64) {
	c.Labels = append(c.Labels, label)
	if len(c.Labels) > maxPoints {
		c.Labels = c.Labels[1:]
	}
	for i, v := range values {
		if i < len(c.Datasets) {
			data := c.Datasets[i]["data"].([]float64)
			data = append(data, v)
			if len(data) > maxPoints {
				data = data[1:]
			}
			c.Datasets[i]["data"] = data
		}
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func formatUptime(secs uint64) string {
	days := secs / 86400
	hours := (secs % 86400) / 3600
	mins := (secs % 3600) / 60
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", mins))
	return strings.Join(parts, " ")
}

func main() {
	eng := godom.NewEngine()
	chartjs.Register(eng)

	root := &App{}
	go root.startMonitor()

	fmt.Println("System monitor — CPU, memory, disk, swap, load with Chart.js")
	eng.Mount(root, ui)
	log.Fatal(eng.Start())
}
