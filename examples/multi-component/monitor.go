package main

import (
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
	"github.com/anupshinde/godom/plugins/chartjs"
)

// M is a shorthand for Chart.js option maps.
type M = map[string]interface{}

const monitorMaxPoints = 30

// Monitor is a simulated system monitor with a Chart.js line chart.
type Monitor struct {
	godom.Component
	CPUChart chartjs.Chart
	cpuBase  float64
}

func (m *Monitor) initChart() {
	m.cpuBase = 30 + rand.Float64()*20
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

func (m *Monitor) startMonitor() {
	m.initChart()
	memBase := 50 + rand.Float64()*15
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		label := time.Now().Format("04:05")

		// Simulated CPU: random walk around base
		m.cpuBase += (rand.Float64() - 0.5) * 8
		if m.cpuBase < 5 {
			m.cpuBase = 5
		} else if m.cpuBase > 95 {
			m.cpuBase = 95
		}
		cpu := math.Round(m.cpuBase*10) / 10

		// Simulated Memory: slow drift
		memBase += (rand.Float64() - 0.48) * 2
		if memBase < 30 {
			memBase = 30
		} else if memBase > 85 {
			memBase = 85
		}
		mem := math.Round(memBase*10) / 10

		m.CPUChart.Labels = append(m.CPUChart.Labels, label)
		cpuData := m.CPUChart.Datasets[0]["data"].([]float64)
		cpuData = append(cpuData, cpu)
		memData := m.CPUChart.Datasets[1]["data"].([]float64)
		memData = append(memData, mem)
		if len(m.CPUChart.Labels) > monitorMaxPoints {
			m.CPUChart.Labels = m.CPUChart.Labels[1:]
			cpuData = cpuData[1:]
			memData = memData[1:]
		}
		m.CPUChart.Datasets[0]["data"] = cpuData
		m.CPUChart.Datasets[1]["data"] = memData

		m.Refresh()
	}
}
