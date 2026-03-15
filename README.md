# godom

> **Experimental — work in progress.** APIs may change without notice.

Build local GUI apps in Go using the browser as the rendering engine. Write HTML for the UI, Go for the logic. No JavaScript authoring required.

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
}

func (a *App) Increment() {
    a.Count++
}

func main() {
    app := godom.New()
    app.Mount(&App{}, ui)
    log.Fatal(app.Start())
}
```

```html
<!-- ui/index.html -->
<h1>Counter: <span g-text="Count">0</span></h1>
<button g-click="Increment">+1</button>
```

Run `go build` and you get a single binary that opens the browser and shows a live counter.

## How it works

- Your Go struct holds all application state
- HTML templates use `g-*` directives to bind to struct fields and methods
- A WebSocket bridge keeps the browser in sync — no page reloads
- State lives in the Go process and survives browser close/reopen
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

### Emit

For stateful components, send events to parent components:

```go
func (t *TodoItem) Remove() {
    t.Emit("RemoveTodo", t.Index)  // calls parent's RemoveTodo(index)
}
```

## Examples

See [cmd/todolist/](cmd/todolist/) for presentational components and [cmd/todolist-stateful/](cmd/todolist-stateful/) for stateful components with props and emit.

## Design principles

- **No JavaScript authoring** — the JS bridge (~170 lines) is injected automatically
- **Dumb bridge** — the JS bridge is intentionally a pure command executor. It does not evaluate expressions, resolve data, diff state, or make decisions. Go computes everything and sends concrete DOM commands (`setText`, `setAttr`, `appendHTML`, etc.). This means all logic is testable in Go, the bridge never drifts out of sync with framework semantics, and debugging stays in one language. This also rules out client-side debouncing or throttling — timing decisions would push logic into JS. Instead, `g-bind` fires on every keystroke with no debounce, keeping two-way binding instant (see [docs/transport.md](docs/transport.md) for why this matters)
- **State in Go** — the browser is a rendering engine, not the source of truth
- **Fail fast** — all directives validated at startup against your struct
- **Single binary** — `go build` produces one executable, no node_modules
- **Local apps** — this is not a web framework, it's for desktop-style local GUIs
