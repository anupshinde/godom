# Examples

| Example | What it demonstrates |
|---------|---------------------|
| [counter](counter/) | Minimal starting point — `g-text`, `g-click`, `g-bind` |
| [clock](clock/) | SVG analog clock with `Refresh()` and `g-attr` (server-pushed updates) |
| [todolist](todolist/) | Presentational components with prop passing |
| [todolist-stateful](todolist-stateful/) | Stateful components with props and `Emit()` |
| [system-monitor](system-monitor/) | Live dashboard with `Refresh()`, `g-attr`, and presentational components |
| [system-monitor-chartjs](system-monitor-chartjs/) | Chart.js plugin — line, doughnut, and bar charts from Go structs |
| [charts-without-plugin](charts-without-plugin/) | ApexCharts with an inline JS bridge adapter (no plugin package) |
| [solar-system](solar-system/) | Go-built 3D engine with Canvas 2D rendering — mouse drag, scroll zoom, follow planets |

Run any example:

```
go run ./examples/counter
```

This starts the server and opens your browser. To build a standalone binary:

```
go build -o counter ./examples/counter
./counter
```
