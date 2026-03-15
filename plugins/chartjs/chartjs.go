// Package chartjs provides a godom plugin for Chart.js integration.
// Chart.js is embedded and injected automatically — no CDN or
// manual <script> tag required. Users configure charts using plain
// Go maps that pass straight through to Chart.js.
package chartjs

import (
	_ "embed"

	"godom"
)

//go:embed chart.min.js
var chartLibJS string

//go:embed chartjs.js
var bridgeJS string

// Register adds the Chart.js plugin to a godom App.
func Register(app *godom.App) {
	app.Plugin("chartjs", chartLibJS, bridgeJS)
}

// Chart holds the configuration and data for a Chart.js chart.
// Options and Datasets use maps so any Chart.js property can be
// passed through without needing Go type definitions.
type Chart struct {
	Type     string                   `json:"type"`
	Labels   []string                 `json:"labels,omitempty"`
	Datasets []map[string]interface{} `json:"datasets"`
	Options  map[string]interface{}   `json:"options,omitempty"`
}
