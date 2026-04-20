package monitor

import (
	"embed"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/examples/multi-page-v2/tools/counter"
	"github.com/anupshinde/godom/plugins/chartjs"
)

//go:embed monitor.html
var fsys embed.FS

type M = map[string]interface{}

const maxPoints = 30

type Monitor struct {
	godom.Island
	*counter.State // shared with Counter — displays Count/Step alongside chart
	CPUChart       chartjs.Chart
	cpuBase        float64
	memBase        float64
}

func (m *Monitor) initChart() {
	m.cpuBase = 30 + rand.Float64()*20
	m.memBase = 50 + rand.Float64()*15
	m.CPUChart = chartjs.Chart{
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
		}, {
			"label":           "Memory %",
			"borderColor":     "#6366f1",
			"backgroundColor": "rgba(99, 102, 241, 0.15)",
			"borderWidth":     2,
			"fill":            true,
			"tension":         0.3,
			"pointRadius":     0,
			"data":            []float64{},
		}},
		Options: M{
			"responsive":          true,
			"maintainAspectRatio": false,
			"animation":           M{"duration": 0},
			"scales": M{
				"x": M{
					"ticks": M{"maxTicksLimit": 6, "font": M{"size": 10}},
					"grid":  M{"display": false},
				},
				"y": M{
					"min": 0, "max": 100,
					"ticks": M{"font": M{"size": 10}},
					"grid":  M{"color": "rgba(0,0,0,0.05)"},
				},
			},
			"plugins": M{
				"legend": M{"display": true, "labels": M{"font": M{"size": 11}}},
			},
		},
	}
}

func (m *Monitor) Run() {
	m.initChart()
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		label := time.Now().Format("04:05")

		m.cpuBase += (rand.Float64() - 0.5) * 8
		m.cpuBase = math.Max(5, math.Min(95, m.cpuBase))
		cpu := math.Round(m.cpuBase*10) / 10

		m.memBase += (rand.Float64() - 0.48) * 2
		m.memBase = math.Max(30, math.Min(85, m.memBase))
		mem := math.Round(m.memBase*10) / 10

		m.CPUChart.Labels = append(m.CPUChart.Labels, label)
		cpuData := append(m.CPUChart.Datasets[0]["data"].([]float64), cpu)
		memData := append(m.CPUChart.Datasets[1]["data"].([]float64), mem)
		if len(m.CPUChart.Labels) > maxPoints {
			m.CPUChart.Labels = m.CPUChart.Labels[1:]
			cpuData = cpuData[1:]
			memData = memData[1:]
		}
		m.CPUChart.Datasets[0]["data"] = cpuData
		m.CPUChart.Datasets[1]["data"] = memData

		m.Refresh()
	}
}

func New(s *counter.State) *Monitor {
	return &Monitor{
		Island: godom.Island{
			TargetName: "monitor",
			Template:   "monitor.html",
			AssetsFS:   fsys,
		},
		State: s,
	}
}
