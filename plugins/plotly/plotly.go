// Package plotly provides a godom plugin for Plotly.js integration.
// Plotly's basic bundle is embedded and injected automatically — no CDN or
// manual <script> tag required. Users configure charts using plain
// Go structs that map to Plotly's declarative JSON API.
package plotly

import (
	_ "embed"

	"github.com/anupshinde/godom"
)

//go:embed plotly.min.js
var plotlyLibJS string

//go:embed plotly.js
var bridgeJS string

// Plugin registers Plotly with a godom Engine.
var Plugin godom.PluginFunc = func(eng *godom.Engine) {
	eng.RegisterPlugin("plotly", plotlyLibJS, bridgeJS)
}

// M is a shorthand for map[string]interface{}.
type M = map[string]interface{}

// Chart holds the configuration for a Plotly chart.
// Data holds the trace array and Layout controls appearance.
// Config is optional Plotly config (e.g. responsive, displayModeBar).
type Chart struct {
	Data   []M `json:"data"`
	Layout M   `json:"layout,omitempty"`
	Config M   `json:"config,omitempty"`
}
