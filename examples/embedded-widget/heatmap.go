package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
)

type HeatmapTile struct {
	Symbol string
	Change string
	Color  string
	Weight string
	price  float64
}

type Heatmap struct {
	godom.Component
	Row1 []HeatmapTile
	Row2 []HeatmapTile
}

func newHeatmap() *Heatmap {
	type entry struct {
		symbol string
		price  float64
		weight float64 // relative size in the heatmap
	}
	stocks := []entry{
		// Tech (large weights)
		{"AAPL", 189.50, 3.0},
		{"MSFT", 415.20, 3.0},
		{"GOOGL", 141.80, 2.5},
		{"NVDA", 875.30, 2.5},
		{"META", 502.60, 2.0},
		{"AMZN", 178.90, 2.0},
		{"TSLA", 175.40, 1.5},
		// Finance
		{"JPM", 198.70, 1.5},
		{"V", 279.30, 1.5},
		{"BRK.B", 412.50, 2.0},
		// Healthcare / Consumer
		{"UNH", 527.10, 1.5},
		{"JNJ", 156.20, 1.0},
		{"WMT", 168.40, 1.0},
		{"DIS", 112.30, 1.0},
		{"XOM", 108.90, 1.0},
	}

	var row1, row2 []HeatmapTile
	for i, s := range stocks {
		pct := (rand.Float64() - 0.5) * 4 // initial -2% to +2%
		tile := HeatmapTile{
			Symbol: s.symbol,
			price:  s.price,
			Change: fmt.Sprintf("%+.2f%%", pct),
			Color:  heatColor(pct),
			Weight: fmt.Sprintf("%d", int(s.weight*10)),
		}
		if i < 7 {
			row1 = append(row1, tile)
		} else {
			row2 = append(row2, tile)
		}
	}

	h := &Heatmap{Row1: row1, Row2: row2}
	h.TargetName = "heatmap"
	h.Template = "ui/heatmap/index.html"
	return h
}

func (h *Heatmap) startTicker() {
	ticker := time.NewTicker(800 * time.Millisecond)
	for range ticker.C {
		allTiles := [][]HeatmapTile{h.Row1, h.Row2}
		for _, row := range allTiles {
		for i := range row {
			t := &row[i]
			changePct := (rand.Float64() - 0.48) * 0.03
			t.price = math.Round(t.price*(1+changePct)*100) / 100
			// cumulative drift from a baseline gives realistic-looking variation
			drift := (rand.Float64() - 0.48) * 0.5
			old := 0.0
			fmt.Sscanf(t.Change, "%f%%", &old)
			pct := old + drift
			// clamp to reasonable range
			if pct > 8 {
				pct = 8
			} else if pct < -8 {
				pct = -8
			}
			t.Change = fmt.Sprintf("%+.2f%%", pct)
			t.Color = heatColor(pct)
		}
		}
		h.Refresh()
	}
}

// heatColor returns a CSS color for a percent change.
// Green for positive, red for negative, darker for larger moves.
func heatColor(pct float64) string {
	intensity := math.Min(math.Abs(pct)/5.0, 1.0)
	if pct >= 0 {
		// green: from #2d6a4f (mild) to #1b4332 (strong)
		r := int(45 - intensity*20)
		g := int(106 - intensity*40)
		b := int(79 - intensity*30)
		return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
	}
	// red: from #c1121f (mild) to #6d1a1a (strong)
	r := int(193 - intensity*84)
	g := int(18 + intensity*8)
	b := int(31 - intensity*5)
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}
