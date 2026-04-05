# Plugins

The `plugins/` directory contains reusable Go packages that integrate JavaScript libraries with godom. Each plugin embeds its JS adapter (and optionally the library itself) and exports a `Plugin` variable for use with `eng.Use()`.

## Shipped plugins

| Plugin | Library | Description |
|--------|---------|-------------|
| `plugins/chartjs/` | [Chart.js](https://www.chartjs.org/) 4.4.8 | Charts — line, bar, pie, doughnut, etc. Library embedded, no CDN needed. |
| `plugins/plotly/` | [Plotly.js](https://plotly.com/javascript/) 3.4.0 | Scientific/statistical charts — scatter, bar, heatmaps, dual-axis. Basic bundle embedded. |
| `plugins/echarts/` | [Apache ECharts](https://echarts.apache.org/) 6.0.0 | Rich interactive charts — line, bar, pie, candlestick, treemap, geo. Library embedded. |

## Using a plugin

```go
import (
    "github.com/anupshinde/godom/plugins/chartjs"
    "github.com/anupshinde/godom/plugins/plotly"
    "github.com/anupshinde/godom/plugins/echarts"
)

func main() {
    app := &App{}
    app.Template = "ui/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    eng.Use(chartjs.Plugin, plotly.Plugin, echarts.Plugin)
    log.Fatal(eng.QuickServe(app))
}
```

```html
<canvas g-plugin:chartjs="MyChart"></canvas>
```

See `examples/system-monitor-chartjs/` for Chart.js, and `examples/chart-plugins/` for Plotly + ECharts side by side.

## Using a JS library without a plugin

You don't need a plugin package to use a JS library. You can keep the JS adapter and Go types in your own application folder, include the library via CDN, and register the adapter inline. This is simpler when you're integrating a library for your own app and don't need a reusable package.

See `examples/charts-without-plugin/` for a working example using ApexCharts.

## Creating your own

For a detailed guide on both approaches — local integration and reusable plugin packages — see **[JavaScript Libraries](javascript-libraries.md)**.
