# godom

[![Tests](https://github.com/anupshinde/godom/actions/workflows/test.yml/badge.svg)](https://github.com/anupshinde/godom/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anupshinde/godom)](https://goreportcard.com/report/github.com/anupshinde/godom)
[![Go Reference](https://pkg.go.dev/badge/github.com/anupshinde/godom.svg)](https://pkg.go.dev/github.com/anupshinde/godom)

> **Experimental — work in progress.** APIs may change without notice.

godom is a framework for building **local apps** in Go that use the browser as the UI layer. It is not a web framework — there are no API endpoints, no frontend/backend split, no JavaScript to write. You build a Go struct, bind HTML to it with directives, and `go build` gives you a single binary. Run it, and the UI appears in your browser.

The browser is just a rendering engine. All state and logic live in your Go process. The JS bridge is a thin command executor that the framework injects — you never touch it.

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

### Events

| Directive | Example | Description |
|-----------|---------|-------------|
| `g-click` | `g-click="Save"` | Call a method on click |
| `g-click` | `g-click="Remove(i)"` | Call with arguments resolved from context |
| `g-keydown` | `g-keydown="Enter:Submit"` | Call method on specific key press |

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
app := godom.New()           // Create a new app
app.Port = 8081              // Set port (0 = random)
app.Component("tag", &T{})   // Register a stateful component (tag must contain a hyphen)
app.Mount(&MyApp{}, fsys)    // Mount root component with embedded filesystem
app.Start()                  // Start server, open browser, block forever
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

## Examples

- [examples/counter/](examples/counter/) — minimal example (the one shown above)
- [examples/clock/](examples/clock/) — analog clock with `Refresh()` and `g-attr` (server-pushed updates)
- [examples/todolist/](examples/todolist/) — presentational components with prop passing
- [examples/todolist-stateful/](examples/todolist-stateful/) — stateful components with props and emit
- [examples/monitor/](examples/monitor/) — live system monitor dashboard with `Refresh()`, `g-attr`, and presentational components

Run any example with:

```
go run ./examples/counter
```

This starts the server and opens your browser. To build a standalone binary instead:

```
go build -o counter ./examples/counter
./counter
```

## Design principles

- **No JavaScript authoring** — the JS bridge (~170 lines) is injected automatically
- **Dumb bridge** — the JS bridge is intentionally a pure command executor. It does not evaluate expressions, resolve data, diff state, or make decisions. Go computes everything and sends concrete DOM commands (`setText`, `setAttr`, `appendHTML`, etc.). This means all logic is testable in Go, the bridge never drifts out of sync with framework semantics, and debugging stays in one language. This also rules out client-side debouncing or throttling — timing decisions would push logic into JS. Instead, `g-bind` fires on every keystroke with no debounce, keeping two-way binding instant (see [docs/transport.md](docs/transport.md) for why this matters)
- **State in Go** — the browser is a rendering engine, not the source of truth
- **Fail fast** — all directives validated at startup against your struct
- **Single binary** — `go build` produces one executable, no node_modules
- **Local apps** — designed for local use and trusted networks, not the public internet. No auth, no HTTPS, no deployment ceremony. Also runs as a service on headless machines ([why?](docs/why.md))

## AI disclosure

This project was **coded with the help of [Claude](https://claude.ai)** (Anthropic). The architecture, design decisions, and all code were produced through human-AI collaboration using Claude Code.

> The documentation including this README is also maintained by AI.

## License

MIT — see [LICENSE](LICENSE).
