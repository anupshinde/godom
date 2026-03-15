package main

type M = map[string]interface{}

// Chart is a lightweight wrapper for ApexCharts config.
// No plugin package needed — this struct lives in the example itself.
type Chart struct {
	Chart      M        `json:"chart"`
	Series     []M      `json:"series"`
	Xaxis      M        `json:"xaxis"`
	Yaxis      M        `json:"yaxis,omitempty"`
	Colors     []string `json:"colors,omitempty"`
	Stroke     M        `json:"stroke,omitempty"`
	Fill       M        `json:"fill,omitempty"`
	Grid       M        `json:"grid,omitempty"`
	Theme      M        `json:"theme,omitempty"`
	Tooltip    M        `json:"tooltip,omitempty"`
	DataLabels M        `json:"dataLabels,omitempty"`
}

// PushPoint shifts the series left and appends a new value and label.
func (c *Chart) PushPoint(label string, value float64, maxPoints int) {
	data := c.Series[0]["data"].([]float64)
	data = append(data[1:], value)
	c.Series[0]["data"] = data

	cats := c.Xaxis["categories"].([]string)
	c.Xaxis["categories"] = append(cats[1:], label)
}
