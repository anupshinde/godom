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
- A binary WebSocket bridge (Protocol Buffers) keeps the browser in sync — no page reloads
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

### Drag and drop

| Directive | Example | Description |
|-----------|---------|-------------|
| `g-draggable` | `g-draggable="i"` | Make element draggable, with the given value as drag data |
| `g-draggable.group` | `g-draggable.palette="'red'"` | Draggable with a named group — only matching dropzones accept the drop |
| `g-dropzone` | `g-dropzone="'canvas'"` | Mark element as a drop zone with a named value (used as `to` in drop handler) |
| `g-drop` | `g-drop="Reorder"` | Call method on drop — receives `(from, to)` or `(from, to, position)` |
| `g-drop.group` | `g-drop.palette="Add"` | Drop handler filtered by group — only fires for matching `g-draggable.group` |

**Groups** isolate drag interactions. A `g-draggable.palette` element can only be dropped on a `g-drop.palette` handler. Without a group, all draggables and drop handlers interact freely.

**Drop data** is passed as method arguments: `from` (the draggable's value), `to` (the dropzone's value or the target's drag data), and optionally `position` (`"above"` or `"below"` based on cursor position). String and numeric values are preserved automatically.

**CSS classes** are applied automatically during drag operations:
- `.g-dragging` — on the element being dragged
- `.g-drag-over` — on a drop zone when a compatible draggable hovers over it
- `.g-drag-over-above` / `.g-drag-over-below` — on sortable items indicating cursor position

See [docs/drag-drop.md](docs/drag-drop.md) for the full design rationale — why this split between bridge and Go, why MIME types for groups, and alternatives considered.

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

#### Nested lists

`g-for` loops can be nested. Inner loops iterate over fields of the outer item:

```html
<div g-for="field, i in Fields">
    <label g-text="field.Label"></label>
    <select g-show="field.IsSelect" style="display:none">
        <option g-for="opt in field.Options" g-text="opt"></option>
    </select>
</div>
```

The inner `g-for` resolves `field.Options` from the outer loop variable. This works to arbitrary nesting depth. See [docs/nested-for.md](docs/nested-for.md) for the design details.

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
app.Host = "0.0.0.0"                    // Bind to all interfaces (default "localhost")
app.NoAuth = true                       // Disable token auth (default false = auth enabled)
app.Token = "my-secret"                 // Fixed token (default: random per startup)
app.NoBrowser = true                    // Don't auto-open browser
app.Quiet = true                        // Suppress startup output
app.Component("tag", &T{})              // Register a stateful component (tag must contain a hyphen)
app.Plugin("chartjs", libJS, bridgeJS)   // Register a plugin with one or more JS scripts
app.Mount(&MyApp{}, fsys)               // Mount root component with embedded filesystem
app.Start()                             // Start server, open browser, block forever
```

Every godom app also supports CLI flags:

```
./myapp --port=8081 --host=0.0.0.0 --no-auth --no-browser --quiet --token=my-secret
```

See [docs/configuration.md](docs/configuration.md) for the full reference on settings, CLI flags, authentication, and precedence rules.

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
- [examples/progress-bar/](examples/progress-bar/) — animated progress bar with `Refresh()` and `g-style:width` from a goroutine
- [examples/clock/](examples/clock/) — analog clock with `Refresh()` and `g-attr` (server-pushed updates)
- [examples/todolist/](examples/todolist/) — presentational components with prop passing
- [examples/todolist-stateful/](examples/todolist-stateful/) — stateful components with props and emit
- [examples/system-monitor/](examples/system-monitor/) — live system monitor dashboard with `Refresh()`, `g-attr`, and presentational components
- [examples/system-monitor-chartjs/](examples/system-monitor-chartjs/) — system monitor with Chart.js plugin (CPU, memory, disk, swap, load charts)
- [examples/charts-without-plugin/](examples/charts-without-plugin/) — ApexCharts with inline bridge adapter (no plugin package)
- [examples/drag-tiles/](examples/drag-tiles/) — 24 colored tiles with drag-to-reorder and a periodic shine animation sweep
- [examples/drag-demo/](examples/drag-demo/) — drag-and-drop demo with groups, dropzones, string data, and position detection (palette → canvas → trash)
- [examples/basic-form-builder/](examples/basic-form-builder/) — drag-and-drop form builder with palette, canvas, config panel, preview mode, and JSON export (uses drag groups, nested g-for, conditional rendering)
- [examples/stock-ticker/](examples/stock-ticker/) — live stock ticker dashboard with 30 simulated stocks, per-stock tick intervals, table with color-coded gainers/losers, and external CSS via static file serving
- [examples/solar-system/](examples/solar-system/) — 3D solar system with a Go-built 3D engine and Canvas 2D rendering (mouse drag, scroll zoom, follow planets)
- [examples/terminal/](examples/terminal/) — browser-based terminal with full shell access via PTY and xterm.js (session respawn, resize, multi-tab, Tailscale-friendly)

Run any example:

```
go run ./examples/counter
```

The `system-monitor`, `system-monitor-chartjs`, and `terminal` examples have their own `go.mod` (for platform-specific or extra dependencies), so run them from their directory:

```
cd examples/system-monitor && go run .
cd examples/system-monitor-chartjs && go run .
cd examples/terminal && go run .
```

This starts the server and opens your browser. To build a standalone binary instead:

```
go build -o counter ./examples/counter
./counter
```

## Design principles

- **Minimal JavaScript** — the JS bridge is injected automatically. For most apps, you write zero JS. When you need a JS library (charts, maps, editors), the plugin system bridges Go data to it with a thin adapter
- **Thin bridge** — the JS bridge is a command executor. It does not evaluate expressions, resolve data, diff state, or make decisions. Go computes everything and sends concrete DOM commands (`setText`, `setAttr`, `appendHTML`, etc.) as binary Protocol Buffers over WebSocket. This means all logic is testable in Go, the bridge stays in sync with framework semantics, and debugging stays in one language. Plugins extend the bridge to delegate rendering to JS libraries when needed. `g-bind` fires on every keystroke with no debounce, keeping two-way binding instant (see [docs/transport.md](docs/transport.md) for why this matters)
- **State in Go** — the browser is a rendering engine, not the source of truth
- **Fail fast** — all directives validated at startup against your struct
- **Single binary** — `go build` produces one executable, no node_modules
- **Local apps** — designed for local use and trusted networks, not the public internet. Token-based auth is on by default to prevent other local users from accessing your app. No HTTPS, no deployment ceremony. Also runs as a service on headless machines ([why?](docs/why.md))

## AI disclosure

This project was **coded with the help of [Claude](https://claude.ai)** (Anthropic). The architecture, design decisions, and all code were produced through human-AI collaboration using Claude Code.

> The documentation including this README is also maintained by AI.

See [docs/AI_USAGE.md](docs/AI_USAGE.md) for the full philosophy on how AI was used, what has and hasn't been reviewed, and what that means if you use this project.

## License

MIT — see [LICENSE](LICENSE).
