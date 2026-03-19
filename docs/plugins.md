# Plugins

The `plugins/` directory contains reusable Go packages that integrate JavaScript libraries with godom. Each plugin embeds its JS adapter (and optionally the library itself) so users can import and use it with a single `Register()` call.

## Shipped plugins

| Plugin | Library | Description |
|--------|---------|-------------|
| `plugins/chartjs/` | [Chart.js](https://www.chartjs.org/) 4.4.8 | Charts — line, bar, pie, doughnut, etc. Library embedded, no CDN needed. |

## Using a plugin

```go
import "github.com/anupshinde/godom/plugins/chartjs"

func main() {
    eng := godom.NewEngine()
    chartjs.Register(eng)  // registers the plugin + injects Chart.js
    eng.Mount(&App{}, ui)
    log.Fatal(eng.Start())
}
```

```html
<canvas g-plugin:chartjs="MyChart"></canvas>
```

See `examples/system-monitor-chartjs/` for a full working example.

## Using a JS library without a plugin

You don't need a plugin package to use a JS library. You can keep the JS adapter and Go types in your own application folder, include the library via CDN, and register the adapter inline. This is simpler when you're integrating a library for your own app and don't need a reusable package.

See `examples/charts-without-plugin/` for a working example using ApexCharts.

## Creating your own

For a detailed guide on both approaches — local integration and reusable plugin packages — see **[JavaScript Libraries](javascript-libraries.md)**.
