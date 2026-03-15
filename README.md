# godom

[![Tests](https://github.com/anupshinde/godom/actions/workflows/test.yml/badge.svg)](https://github.com/anupshinde/godom/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anupshinde/godom)](https://goreportcard.com/report/github.com/anupshinde/godom)
[![Go Reference](https://pkg.go.dev/badge/github.com/anupshinde/godom.svg)](https://pkg.go.dev/github.com/anupshinde/godom)

> **Experimental — work in progress.** APIs may change without notice.

godom is a framework for building **local apps** in Go that use the browser as the UI layer. It is not a web framework — there are no API endpoints, no frontend/backend split, no JavaScript to author for typical use. You build a Go struct, bind HTML to it with directives, and `go build` gives you a single binary. Run it, and the UI appears in your browser.

The browser is the rendering engine. All state and logic live in your Go process. The JS bridge is a thin command executor that the framework injects. For most apps, you never touch JS — but when you need to integrate a JS library (charts, maps, editors), the plugin system lets you bridge Go data to any JS library.

godom also works as a local network service: run the binary on a headless machine and access the UI from any browser on the network. See [docs/why.md](docs/why.md) for the full rationale and how godom differs from Electron, Tauri, and Wails.

```go
package main

import (
    "embed"
    "log"
    "godom"
)

//go:embed ui
var ui embed.FS

type App struct {
    godom.Component
    Count int
    Step  int
}

func (a *App) Increment() {
    a.Count += a.Step
}

func (a *App) Decrement() {
    a.Count -= a.Step
}

func main() {
    app := godom.New()
    app.Mount(&App{Step: 1}, ui)
    log.Fatal(app.Start())
}
```

```html
<!-- ui/index.html -->
<h1><span g-text="Count">0</span></h1>
<button g-click="Decrement">−</button>
<button g-click="Increment">+</button>
<div>
    Step size: <input type="number" min="1" max="100" g-bind="Step"/>
</div>
```

Run `go build` and you get a single binary that opens the browser and shows a live counter. The HTML, CSS, and JS bridge are all embedded into the binary via Go's `embed` package — there are no external files to ship or manage.

## How it works

- Your Go struct holds all application state
- HTML templates use `g-*` directives to bind to struct fields and methods
- A WebSocket bridge keeps the browser in sync — no page reloads
- State lives in the Go process and survives browser close/reopen — close the tab, reopen it, and you're back where you left off
- Open the same app in multiple browser tabs and they stay in sync — type in one, see the update in the other. This falls out naturally from the architecture: Go owns the state and pushes DOM commands to every connected tab
- All directives are validated at startup — typos in field/method names cause `log.Fatal`, not silent runtime bugs

## Install

```
go get godom
```

Requires Go 1.21+ and a web browser.

## Directives reference

### Data binding

| Directive | Example | Description |
|-----------|---------|-------------|
| `g-text` | `g-text="Name"` | Set element's text content from a field |
| `g-bind` | `g-bind="InputText"` | Two-way bind an input's value to a field |
| `g-checked` | `g-checked="todo.Done"` | Bind checkbox checked state |
| `g-show` | `g-show="IsVisible"` | Toggle `display: none` based on truthiness |
| `g-if` | `g-if="HasItems"` | Same as `g-show` (conditional display) |
| `g-class:name` | `g-class:done="todo.Done"` | Add/remove a CSS class conditionally |
| `g-attr:name` | `g-attr:transform="Rotation"` | Set any HTML/SVG attribute from a field |
| `g-plugin:name` | `g-plugin:chartjs="MyChart"` | Send field data to a registered JS plugin |

### Events

| Directive | Example | Description |
|-----------|---------|-------------|
| `g-click` | `g-click="Save"` | Call a method on click |
| `g-click` | `g-click="Remove(i)"` | Call with arguments resolved from context |
| `g-keydown` | `g-keydown="Enter:Submit"` | Call method on specific key press |
| `g-keydown` | `g-keydown="ArrowUp:Up;ArrowDown:Down"` | Multiple key bindings (semicolon-separated) |
| `g-mousedown` | `g-mousedown="OnDown"` | Mouse button pressed — method receives `(x, y float64)` |
| `g-mousemove` | `g-mousemove="OnMove"` | Mouse moved — throttled to animation frame, receives `(x, y float64)` |
| `g-mouseup` | `g-mouseup="OnUp"` | Mouse button released — receives `(x, y float64)` |
| `g-wheel` | `g-wheel="OnScroll"` | Scroll wheel — receives `(deltaY float64)` |

### Lists

```html
<li g-for="todo, i in Todos">
    <span g-text="todo.Text"></span>
    <input type="checkbox" g-checked="todo.Done" g-click="Toggle(i)" />
    <button g-click="Remove(i)">&times;</button>
</li>
```

`g-for="item, index in ListField"` repeats the element for each item in a slice field. The index variable is optional (`g-for="item in Items"` works too).

List rendering uses per-item diffing — only changed items get DOM updates, new items are appended, removed items are truncated.

### Expressions

Directives support:
- Field access: `FieldName`
- Dotted paths: `todo.Text`, `item.Address.City`
- Loop variables: `todo`, `i` from `g-for`
- Literals: `true`, `false`, integers, quoted strings

All expressions are resolved in Go (the browser-side bridge is a pure command executor).

## Components

### Presentational components

Split HTML into reusable files. Any HTML file in your embedded filesystem can be used as a custom element:

```html
<!-- ui/todo-item.html -->
<li g-class:done="todo.Done">
    <input type="checkbox" g-checked="todo.Done" g-click="Toggle(index)" />
    <span g-text="todo.Text"></span>
    <button g-click="Remove(index)">&times;</button>
</li>
```

```html
<!-- ui/index.html -->
<ul>
    <todo-item g-for="todo, i in Todos" :todo="todo" :index="i"></todo-item>
</ul>
```

Props are passed with `:propName="expr"` and become template variables in the child HTML. The child's directives resolve against the parent's state.

### Stateful components

Register a Go struct as a component for scoped state and methods:

```go
type TodoItem struct {
    godom.Component
    Text  string `godom:"prop"`
    Done  bool   `godom:"prop"`
    Index int    `godom:"prop"`
}

func (t *TodoItem) Toggle() {
    t.Emit("ToggleTodo", t.Index)
}

func main() {
    app := godom.New()
    app.Component("todo-item", &TodoItem{})
    app.Mount(&TodoApp{}, ui)
    log.Fatal(app.Start())
}
```

Key differences from presentational components:
- **Own struct**: fields tagged `godom:"prop"` are set by the parent
- **Scoped methods**: `g-click="Toggle"` calls the child's `Toggle()`, not the parent's
- **Emit**: `t.Emit("MethodName", args...)` sends events up the component tree — each ancestor with a matching method gets called, bottom-up

## API

### App

```go
app := godom.New()                       // Create a new app
app.Port = 8081                          // Set port (0 = random)
app.Component("tag", &T{})              // Register a stateful component (tag must contain a hyphen)
app.Plugin("chartjs", libJS, bridgeJS)   // Register a plugin with one or more JS scripts
app.Mount(&MyApp{}, fsys)               // Mount root component with embedded filesystem
app.Start()                             // Start server, open browser, block forever
```

### Component

Embed `godom.Component` in your struct:

```go
type MyApp struct {
    godom.Component
    Name string        // exported fields = state
    Items []Item       // slices work with g-for
}

func (a *MyApp) DoSomething() {
    // exported methods = event handlers
    // mutate fields directly, framework handles sync
}
```

### Refresh

Push state to all connected browsers from a background goroutine:

```go
func (a *App) monitor() {
    for {
        time.Sleep(1 * time.Second)
        a.Value = readSensor()
        a.Refresh()  // broadcast to all browsers
    }
}
```

Call `Refresh()` after mutating fields outside of user-triggered events (clicks, input). This is how you build dashboards, monitors, and live-updating UIs.

### Emit

For stateful components, send events to parent components:

```go
func (t *TodoItem) Remove() {
    t.Emit("RemoveTodo", t.Index)  // calls parent's RemoveTodo(index)
}
```

### Plugins

Integrate JS libraries (charts, maps, editors) without authoring JS yourself. A plugin is a thin JS adapter that receives Go data via the `g-plugin:name` directive:

```html
<canvas g-plugin:chartjs="MyChart"></canvas>
```

```go
app.Plugin("chartjs", libraryJS, bridgeJS)  // register with one or more JS scripts
```

The plugin JS calls `godom.register(name, {init, update})` to handle data from Go. Scripts are injected in order — typically the library first, then the bridge. See `plugins/chartjs/` for a complete example.

See [docs/javascript-libraries.md](docs/javascript-libraries.md) for a detailed guide on using any JS library — with or without a plugin package.

godom ships a Chart.js plugin (`godom/plugins/chartjs`) that embeds Chart.js and provides a minimal Go struct for chart data. Charts are configured using plain `map[string]interface{}` — any Chart.js property passes straight through:

```go
import "godom/plugins/chartjs"

chartjs.Register(app)  // registers plugin + embeds Chart.js library
```

## Examples

- [examples/counter/](examples/counter/) — minimal example (the one shown above)
- [examples/clock/](examples/clock/) — analog clock with `Refresh()` and `g-attr` (server-pushed updates)
- [examples/todolist/](examples/todolist/) — presentational components with prop passing
- [examples/todolist-stateful/](examples/todolist-stateful/) — stateful components with props and emit
- [examples/system-monitor/](examples/system-monitor/) — live system monitor dashboard with `Refresh()`, `g-attr`, and presentational components
- [examples/system-monitor-chartjs/](examples/system-monitor-chartjs/) — system monitor with Chart.js plugin (CPU, memory, disk, swap, load charts)
- [examples/charts-without-plugin/](examples/charts-without-plugin/) — ApexCharts with inline bridge adapter (no plugin package)
- [examples/solar-system/](examples/solar-system/) — 3D solar system with a Go-built 3D engine and Canvas 2D rendering (mouse drag, scroll zoom, follow planets)

Run any example:

```
go run ./examples/counter
```

The `system-monitor` and `system-monitor-chartjs` examples have their own `go.mod` (for platform-specific dependencies), so run them from their directory:

```
cd examples/system-monitor && go run .
cd examples/system-monitor-chartjs && go run .
```

This starts the server and opens your browser. To build a standalone binary instead:

```
go build -o counter ./examples/counter
./counter
```

## Design principles

- **Minimal JavaScript** — the JS bridge is injected automatically. For most apps, you write zero JS. When you need a JS library (charts, maps, editors), the plugin system bridges Go data to it with a thin adapter
- **Thin bridge** — the JS bridge is a command executor. It does not evaluate expressions, resolve data, diff state, or make decisions. Go computes everything and sends concrete DOM commands (`setText`, `setAttr`, `appendHTML`, etc.). This means all logic is testable in Go, the bridge stays in sync with framework semantics, and debugging stays in one language. Plugins extend the bridge to delegate rendering to JS libraries when needed. `g-bind` fires on every keystroke with no debounce, keeping two-way binding instant (see [docs/transport.md](docs/transport.md) for why this matters)
- **State in Go** — the browser is a rendering engine, not the source of truth
- **Fail fast** — all directives validated at startup against your struct
- **Single binary** — `go build` produces one executable, no node_modules
- **Local apps** — designed for local use and trusted networks, not the public internet. No auth, no HTTPS, no deployment ceremony. Also runs as a service on headless machines ([why?](docs/why.md))

## AI disclosure

This project was **coded with the help of [Claude](https://claude.ai)** (Anthropic). The architecture, design decisions, and all code were produced through human-AI collaboration using Claude Code.

> The documentation including this README is also maintained by AI.

## License

MIT — see [LICENSE](LICENSE).
