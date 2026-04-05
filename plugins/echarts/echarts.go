// Package echarts provides a godom plugin for Apache ECharts integration.
// ECharts is embedded and injected automatically — no CDN or
// manual <script> tag required. Users configure charts using plain
// Go maps that map to ECharts' option-driven API.
package echarts

import (
	_ "embed"

	"github.com/anupshinde/godom"
)

//go:embed echarts.min.js
var echartsLibJS string

//go:embed echarts.js
var bridgeJS string

// Plugin registers ECharts with a godom Engine.
var Plugin godom.PluginFunc = func(eng *godom.Engine) {
	eng.RegisterPlugin("echarts", echartsLibJS, bridgeJS)
}

// M is a shorthand for map[string]interface{}.
type M = map[string]interface{}

// Chart holds the ECharts option object.
// All fields map directly to ECharts' setOption API.
type Chart struct {
	Title   M   `json:"title,omitempty"`
	Tooltip M   `json:"tooltip,omitempty"`
	Legend  M   `json:"legend,omitempty"`
	XAxis   M   `json:"xAxis,omitempty"`
	YAxis   M   `json:"yAxis,omitempty"`
	Series  []M `json:"series"`
	Grid    M   `json:"grid,omitempty"`
	Color   []string `json:"color,omitempty"`
}
