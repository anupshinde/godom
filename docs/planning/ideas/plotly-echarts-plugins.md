# godom — Plotly / ECharts Plugin Examples

## The idea

Build godom plugins for Plotly and/or ECharts — two of the most popular charting libraries alongside Chart.js. godom already has a Chart.js plugin and a "without plugin" ApexCharts example. Adding Plotly or ECharts would expand the charting options available to godom developers and test the plugin system against libraries with very different APIs and rendering approaches.

## Why Plotly and ECharts

Chart.js is good for simple, clean charts. But developers building data-heavy apps often reach for something more powerful:

**Plotly**
- Scientific and statistical charts (scatter matrices, 3D surfaces, contour plots, heatmaps)
- Built-in interactivity — zoom, pan, hover tooltips, lasso select — all without custom code
- Declarative JSON API (`data` + `layout` objects) — maps naturally to Go structs/maps
- Used heavily in data science, research, and dashboards
- Large library (~3.5MB minified), but can use partial bundles

**ECharts (Apache)**
- Extremely rich chart types — candlestick, treemap, sunburst, sankey, graph/network, geo/maps
- Excellent animation and transition support out of the box
- High performance with large datasets (canvas-based rendering, progressive loading)
- Option-driven API (one big config object) — also maps well to Go maps
- Smaller than Plotly (~1MB minified), very popular in enterprise dashboards

Both libraries use a declarative configuration model: you describe what you want as a data structure, the library renders it. This is exactly how godom's plugin system works — Go struct → JSON → plugin JS → library call.

## How they'd work as godom plugins

Same pattern as the existing Chart.js plugin:

### Plotly plugin

```go
import "godom/plugins/plotly"

func main() {
    app := godom.New()
    plotly.Register(app)
    app.Mount(&Dashboard{}, ui)
    log.Fatal(app.Start())
}
```

```go
type Dashboard struct {
    ScatterPlot plotly.Chart
}

func NewDashboard() *Dashboard {
    return &Dashboard{
        ScatterPlot: plotly.Chart{
            Data: []map[string]interface{}{
                {
                    "type": "scatter",
                    "mode": "markers",
                    "x":    []float64{1, 2, 3, 4},
                    "y":    []float64{10, 15, 13, 17},
                },
            },
            Layout: map[string]interface{}{
                "title": "Sensor Readings",
            },
        },
    }
}
```

```html
<div g-plugin:plotly="ScatterPlot"></div>
```

The plugin JS adapter:
- `init`: calls `Plotly.newPlot(el, data, layout)`
- `update`: calls `Plotly.react(el, data, layout)` — Plotly's efficient update path that diffs and transitions

### ECharts plugin

```go
import "godom/plugins/echarts"

func main() {
    app := godom.New()
    echarts.Register(app)
    app.Mount(&Dashboard{}, ui)
    log.Fatal(app.Start())
}
```

```go
type Dashboard struct {
    SalesChart echarts.Chart
}

func NewDashboard() *Dashboard {
    return &Dashboard{
        SalesChart: echarts.Chart{
            Option: map[string]interface{}{
                "title": map[string]interface{}{"text": "Monthly Sales"},
                "xAxis": map[string]interface{}{
                    "type": "category",
                    "data": []string{"Jan", "Feb", "Mar", "Apr", "May"},
                },
                "yAxis": map[string]interface{}{"type": "value"},
                "series": []map[string]interface{}{
                    {
                        "type": "bar",
                        "data": []int{120, 200, 150, 80, 70},
                    },
                },
            },
        },
    }
}
```

```html
<div g-plugin:echarts="SalesChart"></div>
```

The plugin JS adapter:
- `init`: creates `echarts.init(el)`, calls `chart.setOption(option)`
- `update`: calls `chart.setOption(option)` again — ECharts merges options by default and animates transitions

## What this tests in the plugin system

| Aspect | Chart.js (done) | Plotly | ECharts |
|--------|-----------------|--------|---------|
| Library size | ~200KB | ~3.5MB (or partial bundle) | ~1MB |
| Rendering | Canvas | SVG + Canvas (mixed) | Canvas (default) + SVG option |
| Update model | `chart.update()` | `Plotly.react()` (diff-based) | `setOption()` (merge-based) |
| Interactivity | Basic (hover, click) | Rich (zoom, pan, lasso, hover) | Rich (zoom, brush, dataZoom) |
| Resize handling | `chart.resize()` | `Plotly.Plots.resize()` | `chart.resize()` |
| Event callbacks | Limited | Extensive (click, hover, select, zoom) | Extensive |

The interesting test is whether godom's plugin update cycle (Go state change → JSON diff → plugin `update()`) works cleanly with libraries that have their own rich internal state (Plotly's zoom level, ECharts' animation state). Chart.js is relatively simple here. Plotly and ECharts are more opinionated about owning their state.

### Plugin events flowing back to Go

Both Plotly and ECharts support rich user interactions — clicking a data point, selecting a region, zooming into a range. These events should flow back to Go:

- User clicks a bar in an ECharts chart → plugin JS captures the event → sends it over WebSocket → Go handler receives the clicked data point
- User lasso-selects points in a Plotly scatter plot → selection event → Go gets the selected indices

This would exercise the plugin-to-Go event path, which the Chart.js plugin may not be testing heavily today.

## Example apps that would benefit

- **Data explorer** — load a CSV/JSON dataset in Go, render interactive Plotly scatter plots and histograms, click to filter, selections flow back to Go
- **System monitor upgrade** — the existing system-monitor-chartjs example, but with ECharts for richer visualizations (gauge charts for CPU, heatmap for process activity)
- **Financial dashboard** — ECharts candlestick charts with real-time data pushed from Go, dataZoom for time range selection
- **Scientific visualization** — Plotly 3D surface plots, contour maps, updated from Go computation

## Embedding vs CDN

The Chart.js plugin embeds the library (~200KB) into the Go binary. For Plotly at ~3.5MB, embedding is heavier but still reasonable for a single-binary goal. Options:

- **Embed full library** — largest binary, but zero external dependencies. Consistent with godom's philosophy.
- **Embed partial bundle** — Plotly supports custom bundles (e.g., `plotly-basic` at ~1MB with just scatter, bar, pie). Good tradeoff.
- **CDN fallback** — load from CDN if available, fall back to embedded. Adds complexity, breaks offline use.

ECharts at ~1MB is fine to embed. Same approach as Chart.js, just bigger.

## Status

Low-hanging fruit. The plugin system and the pattern are proven with Chart.js. Plotly and ECharts are both config-driven, which maps directly to Go maps/structs. The main work per plugin is a thin JS adapter (~50 lines) and a Go package with the `Register()` function and chart type definitions. A good candidate for expanding godom's library ecosystem.
