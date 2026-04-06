# godom — AI Agent Reference

> Complete reference for building godom applications. Read this file to build apps without reading source code.

godom is a Go framework for building local GUI apps that use the browser as the rendering engine. You write a Go struct (state + methods), bind HTML to it with `g-*` directives, and `go build` produces a single binary. No JavaScript required for typical apps.

**Import:** `github.com/anupshinde/godom`
**Requires:** Go 1.25+, a web browser

---

## Table of Contents

- [Minimal Complete App](#minimal-complete-app)
- [Engine API](#engine-api)
- [Component API](#component-api)
- [Template Directives](#template-directives)
- [Expressions](#expressions)
- [Event Handling](#event-handling)
- [Two-Way Binding](#two-way-binding)
- [Loops (g-for)](#loops-g-for)
- [Conditional Rendering](#conditional-rendering)
- [Attributes, Classes, Styles](#attributes-classes-styles)
- [Custom Elements (Template Includes)](#custom-elements-template-includes)
- [Multiple Components](#multiple-components)
- [Background Updates (Refresh)](#background-updates-refresh)
- [Surgical Updates (MarkRefresh)](#surgical-updates-markrefresh)
- [ExecJS (Go to Browser)](#execjs-go-to-browser)
- [godom.call (Browser to Go)](#godomcall-browser-to-go)
- [Plugins](#plugins)
- [Drag and Drop](#drag-and-drop)
- [Shadow DOM](#shadow-dom)
- [Configuration](#configuration)
- [Environment Variables](#environment-variables)
- [Developer-Owned Server](#developer-owned-server)
- [Embedded Mode (External Hosting)](#embedded-mode-external-hosting)
- [WebSocket Lifecycle Hooks](#websocket-lifecycle-hooks)
- [Dynamic Mounting](#dynamic-mounting)
- [CSS: Hiding Raw Templates](#css-hiding-raw-templates)
- [Common Patterns](#common-patterns)
- [Gotchas and Rules](#gotchas-and-rules)
- [Project Structure Convention](#project-structure-convention)
- [Examples Index](#examples-index)

---

## Minimal Complete App

Two files — a Go file and an HTML template:

**main.go:**
```go
package main

import (
    "embed"
    "log"
    "github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
    godom.Component
    Count int
    Step  int
}

func (a *App) Increment() { a.Count += a.Step }
func (a *App) Decrement() { a.Count -= a.Step }

func main() {
    app := &App{Step: 1}
    app.Template = "ui/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    log.Fatal(eng.QuickServe(app))
}
```

**ui/index.html:**
```html
<!DOCTYPE html>
<html>
<body>
    <h1><span g-text="Count">0</span></h1>
    <button g-click="Decrement">-</button>
    <button g-click="Increment">+</button>
    <div>Step: <input type="number" min="1" max="100" g-bind="Step" /></div>
</body>
</html>
```

Run: `go run .` — opens browser automatically. `go build -o myapp .` for a single binary.

---

## Engine API

```go
eng := godom.NewEngine()
```

### Engine Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Port` | `int` | `0` (random) | TCP port to listen on |
| `Host` | `string` | `"localhost"` | Bind address |
| `NoAuth` | `bool` | `false` | Disable token authentication |
| `FixedAuthToken` | `string` | `""` (random) | Fixed auth token (random 32-char hex if empty) |
| `NoBrowser` | `bool` | `false` | Don't auto-open browser on start |
| `Quiet` | `bool` | `false` | Suppress startup output |
| `DisableExecJS` | `bool` | `false` | Disable ExecJS server-side |
| `DisconnectHTML` | `string` | built-in | Custom disconnect overlay HTML (root mode) |
| `DisconnectBadgeHTML` | `string` | built-in | Custom disconnect badge HTML (embedded mode) |

### Engine Methods

| Method | Description |
|--------|-------------|
| `SetFS(fs.FS)` | Set shared filesystem for templates (typically `embed.FS`) |
| `SetMux(mux *http.ServeMux, opts *MuxOptions)` | Register godom handlers on a custom mux |
| `SetAuth(middleware.AuthFunc)` | Set custom auth function |
| `Register(components ...interface{})` | Register one or more components (variadic) |
| `Use(plugins ...PluginFunc)` | Register plugin functions |
| `RegisterPlugin(name string, scripts ...string)` | Register custom plugin with JS scripts |
| `Run() error` | Validate templates, register handlers, start event processors |
| `QuickServe(component interface{}) error` | All-in-one: sets TargetName="document.body", registers, runs, serves (blocks) |
| `ListenAndServe() error` | Bind port, wrap with auth, open browser, serve (blocks) |
| `AuthMiddleware(http.Handler) http.Handler` | Wrap handler with auth (call after Run) |
| `Cleanup()` | Close event channels on shutdown |

### MuxOptions

```go
type MuxOptions struct {
    WSPath     string // WebSocket path (default: "/ws")
    ScriptPath string // godom.js path (default: "/godom.js")
}
```

---

## Component API

Embed `godom.Component` in any Go struct to make it a component:

```go
type MyApp struct {
    godom.Component          // required embed
    Name    string           // exported fields = template state
    Items   []Item           // slices for g-for loops
    count   int              // unexported = private, invisible to templates
}
```

### Component Fields

| Field | Type | Description |
|-------|------|-------------|
| `TargetName` | `string` | Matches `g-component="name"` in parent template. Set to `"document.body"` for root (QuickServe does this automatically) |
| `Template` | `string` | Path to HTML template relative to SetFS filesystem |

### Component Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Refresh` | `func()` | Push current state to all connected browsers. Use from background goroutines. Do NOT call inside event handlers |
| `MarkRefresh` | `func(fields ...string)` | Mark specific fields for surgical refresh (accumulates). Next Refresh() only patches nodes bound to these fields |
| `ExecJS` | `func(expr string, cb func(result []byte, err string))` | Execute JS in all connected browsers. Callback fires once per browser |

### Rules

- **Exported fields** → accessible in templates as state
- **Exported methods** → callable as event handlers from `g-click`, `g-keydown`, etc.
- **Unexported fields/methods** → private, invisible to templates
- After an event handler method runs, godom **automatically** re-renders and pushes patches — do NOT call `Refresh()` inside event handlers
- `Refresh()` is only needed for background goroutine updates

---

## Template Directives

All directives are `g-*` attributes on HTML elements.

### Data Display

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-text` | `g-text="FieldName"` | Set element's text content from field |
| `g-html` | `g-html="FieldName"` | Set element's innerHTML (use with trusted content only) |
| `{{expr}}` | `<p>Hello, {{Name}}!</p>` | Inline text interpolation in text content |

### Input Binding

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-bind` | `g-bind="FieldName"` | Two-way bind: input syncs to Go field on every keystroke |
| `g-value` | `g-value="FieldName"` | One-way bind: Go → browser only, no sync back |
| `g-checked` | `g-checked="FieldName"` | Bind checkbox/radio checked state to bool field |

### Conditional Rendering

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-if` | `g-if="Expr"` | Remove element from DOM tree when falsy |
| `g-show` | `g-show="Expr"` | Hide with `display: none` when falsy (stays in DOM) |
| `g-hide` | `g-hide="Expr"` | Hide with `display: none` when truthy |

### Loops

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-for` | `g-for="item in Items"` | Loop over slice, render element per item |
| `g-for` | `g-for="item, i in Items"` | Loop with index variable |
| `g-key` | `g-key="item.ID"` | Stable key for efficient reordering (on same element as g-for) |

### Attributes and Styling

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-attr:name` | `g-attr:src="ImageURL"` | Set any HTML/SVG attribute |
| `g-class:name` | `g-class:active="IsActive"` | Conditionally add/remove CSS class |
| `g-style:prop` | `g-style:color="TextColor"` | Set inline CSS property |
| `g-prop:name` | `g-prop:scrollTop="ScrollPos"` | Set DOM property (kebab→camelCase) |

### Events

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-click` | `g-click="MethodName"` | Call method on click |
| `g-click` | `g-click="Remove(i)"` | Call method with arguments |
| `g-keydown` | `g-keydown="Submit"` | Call on any key press |
| `g-keydown` | `g-keydown="Enter:Submit"` | Call on specific key |
| `g-keydown` | `g-keydown="ArrowUp:Up;ArrowDown:Down"` | Multiple key bindings (semicolon-separated) |
| `g-mousedown` | `g-mousedown="OnDown"` | Mouse down — receives `(x, y float64)` |
| `g-mousemove` | `g-mousemove="OnMove"` | Mouse move — throttled to rAF, receives `(x, y float64)` |
| `g-mouseup` | `g-mouseup="OnUp"` | Mouse up — receives `(x, y float64)` |
| `g-wheel` | `g-wheel="OnScroll"` | Scroll wheel — receives `(deltaY float64)` |
| `g-scroll` | `g-scroll="OnScroll"` | Scroll event |

### Drag and Drop

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-draggable` | `g-draggable="payload"` | Make element draggable with payload value |
| `g-draggable:group` | `g-draggable:tasks="i"` | Draggable within named group |
| `g-dropzone` | `g-dropzone="'target'"` | Mark as drop target with named value |
| `g-drop` | `g-drop="HandleDrop"` | Drop handler — receives `(from, to)` or `(from, to, position)` |
| `g-drop:group` | `g-drop:tasks="Reorder"` | Group-filtered drop handler |

### Plugins

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-plugin:name` | `g-plugin:chartjs="ChartData"` | Delegate rendering to named JS plugin |

### Other

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `g-shadow` | `g-shadow` | Render component inside Shadow DOM for CSS isolation |
| `g-component` | `g-component="name"` | Declare insertion point for a child component |

---

## Expressions

Directive values are expressions resolved in Go. Supported:

| Expression | Example | Description |
|------------|---------|-------------|
| Field access | `Name` | Top-level struct field |
| Dotted path | `todo.Text`, `item.Address.City` | Nested field access |
| Loop variable | `item`, `i` | From enclosing `g-for` |
| Map access | `Inputs[key]` | Bracket notation |
| Negation | `!Active` | Boolean negation |
| String literal | `'active'` | Use single quotes in HTML attributes |
| Number literal | `42` | Integer literal |
| Boolean literal | `true`, `false` | |
| Comparison | `Count > 0`, `Status == 'active'` | `==`, `!=`, `<`, `>`, `<=`, `>=` |
| Logical | `IsAdmin and IsActive` | `and`, `or`, `not` (not `&&`, `||`) |
| Method call | `Summary()` | Zero-arg exported method returning a value |
| Interpolation | `Hello, {{Name}}!` | Mix static text with `{{expr}}` in text content |

**Truthiness:** `nil`, `false`, `0`, `""`, empty slice/map → falsy. Everything else → truthy.

Complex expressions use [expr-lang/expr](https://github.com/expr-lang/expr).

---

## Event Handling

### Method Signatures

Event handler methods are exported methods on the component struct. The framework calls them via reflection.

```go
// No arguments — for simple clicks, toggles
func (a *App) Save() { ... }

// Integer argument — typically loop index from g-click="Remove(i)"
func (a *App) Remove(i int) { ... }

// Multiple arguments — g-click="Move(i, todo.ID)"
func (a *App) Move(index int, id int) { ... }

// Mouse events — g-mousedown, g-mousemove, g-mouseup receive (x, y)
func (a *App) DragMove(x, y float64) { ... }

// Wheel event — g-wheel receives deltaY
func (a *App) Zoom(deltaY float64) { ... }

// Drag and drop — g-drop receives (from, to) or (from, to, position)
func (a *App) Reorder(from, to float64) { ... }
func (a *App) ReorderWithPos(from, to float64, position string) { ... }
// position is "above" or "below"
```

**Important:** After an event handler runs, godom automatically diffs and pushes patches. Do NOT call `Refresh()` inside event handlers.

---

## Two-Way Binding

```html
<input type="text" g-bind="Name" />
<input type="number" g-bind="Count" />
<textarea g-bind="Description"></textarea>
<input type="checkbox" g-checked="Active" />
```

- `g-bind` syncs on every keystroke (no debounce)
- Works with `<input>`, `<textarea>`, `<select>`
- `g-checked` is specifically for checkbox/radio boolean binding
- The Go field type must match: `string` for text, `int`/`float64` for number, `bool` for checkbox

---

## Loops (g-for)

### Basic loop
```html
<li g-for="item in Items">
    <span g-text="item.Name"></span>
</li>
```

### With index
```html
<li g-for="item, i in Items">
    <span>{{i}}: {{item.Name}}</span>
    <button g-click="Remove(i)">x</button>
</li>
```

### Keyed loop (for efficient reordering)
```html
<li g-for="item in Items" g-key="item.ID">
    <span g-text="item.Name"></span>
</li>
```

### Nested loops
```html
<div g-for="group in Groups">
    <h2 g-text="group.Name"></h2>
    <span g-for="item in group.Items" g-text="item.Label"></span>
</div>
```

Inner loops resolve fields from the outer loop variable. Works to arbitrary depth.

### Go side
```go
type App struct {
    godom.Component
    Items []Item
}

type Item struct {
    ID   int
    Name string
}
```

---

## Conditional Rendering

```html
<!-- Remove from DOM entirely -->
<div g-if="HasItems">Content here</div>
<div g-if="!HasItems">No items yet.</div>

<!-- Comparisons -->
<div g-if="Status == 'active'">Active</div>
<div g-if="Count > 0">Has items</div>
<div g-if="Score >= Threshold and IsVerified">Qualified</div>

<!-- Hide with display:none (stays in DOM) -->
<div g-show="IsVisible">Shown when truthy</div>
<div g-hide="IsLoading">Hidden when truthy</div>
```

---

## Attributes, Classes, Styles

```html
<!-- Dynamic attribute -->
<img g-attr:src="ImageURL" />
<a g-attr:href="Link">Click</a>
<svg><rect g-attr:transform="Rotation"></rect></svg>

<!-- Conditional CSS class -->
<li g-class:selected="IsActive">Tab</li>
<li g-class:done="todo.Done">Todo</li>

<!-- Inline style -->
<div g-style:background-color="BgColor"></div>
<div g-style:width="BarWidth" g-style:height="BarHeight"></div>
<div g-style:top="Box.Top" g-style:left="Box.Left"></div>

<!-- DOM property -->
<div g-prop:scrollTop="ScrollPos"></div>
```

---

## Custom Elements (Template Includes)

Split templates into reusable HTML files. Any HTML file in your embedded filesystem can be used as a custom element tag:

**ui/todo-item.html:**
```html
<li g-class:done="todo.Done">
    <input type="checkbox" g-checked="todo.Done" g-click="Toggle(index)" />
    <span g-text="todo.Text"></span>
    <button g-click="Remove(index)">x</button>
</li>
```

**ui/index.html:**
```html
<ul>
    <todo-item g-for="todo, i in Todos"></todo-item>
</ul>
```

- Custom elements are expanded inline at registration time
- Directives inside the child HTML resolve against the **parent** component's state
- Loop variables (`todo`, `i`) are available inside child templates
- The tag name maps to the filename: `<todo-item>` → `ui/todo-item.html`
- This is purely a template include mechanism — not a separate component

---

## Multiple Components

For apps with independent state sections, use separate components:

```go
type Counter struct {
    godom.Component
    Count int
}
func (c *Counter) Increment() { c.Count++ }

type Clock struct {
    godom.Component
    Time string
}

func main() {
    counter := &Counter{}
    counter.TargetName = "counter"
    counter.Template = "ui/counter/index.html"

    clock := &Clock{}
    clock.TargetName = "clock"
    clock.Template = "ui/clock/index.html"

    layout := &Layout{}
    layout.Template = "ui/layout/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    eng.Register(counter, clock)
    log.Fatal(eng.QuickServe(layout))
}
```

**ui/layout/index.html** (root template):
```html
<body>
    <h1>Dashboard</h1>
    <div g-component="counter"></div>
    <div g-component="clock"></div>
</body>
```

**ui/counter/index.html** (child template — HTML fragment, no `<html>`/`<body>`):
```html
<div>
    <span g-text="Count"></span>
    <button g-click="Increment">+</button>
</div>
```

### Cross-component communication

Wire Go callbacks in `main()`:

```go
counter.OnChange = func(n int) { toast.Show("Count changed", "info") }
```

### Shared state via embedded struct

```go
type SharedState struct { Count int }

type CompA struct {
    godom.Component
    *SharedState
}
type CompB struct {
    godom.Component
    *SharedState
}

shared := &SharedState{}
a := &CompA{SharedState: shared}
b := &CompB{SharedState: shared}
```

When one modifies shared state and calls `Refresh()`, both update.

---

## Background Updates (Refresh)

Use goroutines + `Refresh()` for live-updating UIs:

```go
func (a *App) startClock() {
    for range time.Tick(time.Second) {
        a.Time = time.Now().Format("15:04:05")
        a.Refresh() // pushes to all connected browsers
    }
}

func main() {
    app := &App{}
    app.Template = "ui/index.html"
    go app.startClock() // start background goroutine

    eng := godom.NewEngine()
    eng.SetFS(ui)
    log.Fatal(eng.QuickServe(app))
}
```

`Refresh()` does a full tree diff. For high-frequency updates, use `MarkRefresh`.

---

## Surgical Updates (MarkRefresh)

When you know which fields changed, mark them for surgical patches (skips full tree diff):

```go
func (a *App) UpdatePrice(i int) {
    a.Stocks[i].Price = fetchPrice()
    a.MarkRefresh("Stocks") // only rebuild nodes bound to Stocks
    a.Refresh()             // sends surgical patches, not full diff
}
```

- `MarkRefresh` accumulates across calls — call it multiple times before `Refresh()`
- If no fields are marked, `Refresh()` does a full tree diff (expensive fallback)
- Inside event handlers, the framework handles refresh automatically — don't call either

---

## ExecJS (Go to Browser)

Execute JavaScript in all connected browsers and receive results:

```go
// Query browser state
a.ExecJS("({url: location.href, vw: window.innerWidth})", func(result []byte, err string) {
    if err != "" {
        log.Println("error:", err)
        return
    }
    var info struct {
        URL string `json:"url"`
        VW  int    `json:"vw"`
    }
    json.Unmarshal(result, &info)
    a.BrowserURL = info.URL
    a.Refresh()
})

// Execute an action (no result needed)
a.ExecJS("document.title = 'Updated'", func(result []byte, err string) {})
```

- Callback fires **once per connected browser** (3 tabs = 3 callbacks)
- Result is JSON-serialized automatically by the bridge
- Can be disabled server-side: `eng.DisableExecJS = true`
- Can be disabled browser-side: `window.GODOM_DISABLE_EXEC = true`

---

## godom.call (Browser to Go)

JavaScript in the browser can call Go methods:

```html
<button onclick="godom.call('DoSomething', 42)">Click</button>
```

```js
// From plugin code or inline scripts
godom.call("SelectItem", itemId);
godom.call("UpdateConfig", JSON.stringify({key: "value"}));
```

Go side — just a normal exported method:
```go
func (a *App) DoSomething(n int) {
    a.Value = n
}

func (a *App) SelectItem(id string) {
    a.SelectedID = id
}
```

The server searches all registered components for the method name. First match wins. After the method runs, godom auto-refreshes.

---

## Plugins

Plugins bridge Go data to JavaScript libraries (charts, maps, editors).

### Using shipped plugins

```go
import (
    "github.com/anupshinde/godom/plugins/chartjs"  // Chart.js
    "github.com/anupshinde/godom/plugins/plotly"   // Plotly
    "github.com/anupshinde/godom/plugins/echarts"  // ECharts
)

eng.Use(chartjs.Plugin)   // register plugin
eng.Use(plotly.Plugin)
eng.Use(echarts.Plugin)
```

```html
<canvas g-plugin:chartjs="ChartData"></canvas>
<div g-plugin:plotly="PlotlyData"></div>
<div g-plugin:echarts="EChartsData"></div>
```

The field (e.g., `ChartData`) is any struct or map that serializes to the library's expected JSON config. When the field changes, the plugin updates automatically.

### Chart.js example

```go
type App struct {
    godom.Component
    ChartData map[string]interface{}
}

func main() {
    app := &App{
        ChartData: map[string]interface{}{
            "type": "bar",
            "data": map[string]interface{}{
                "labels": []string{"A", "B", "C"},
                "datasets": []map[string]interface{}{
                    {"label": "Values", "data": []int{10, 20, 30}},
                },
            },
        },
    }
    app.Template = "ui/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    eng.Use(chartjs.Plugin)
    log.Fatal(eng.QuickServe(app))
}
```

### Custom plugin (RegisterPlugin)

```go
eng.RegisterPlugin("myplugin", libraryJS, adapterJS)
```

The adapter JS must call `godom.register(name, {init, update})`:

```js
godom.register("myplugin", {
    init: function(element, data) {
        // Called once on first render — create the library instance
    },
    update: function(element, data) {
        // Called on every subsequent render — update with new data
    }
});
```

---

## Drag and Drop

### Making elements draggable

```html
<!-- Simple draggable — payload is the expression value -->
<div g-for="item, i in Items" g-draggable="i">{{item.Name}}</div>

<!-- Grouped draggable — only matching dropzones accept -->
<div g-draggable:palette="'red'">Red</div>
<div g-draggable:palette="'blue'">Blue</div>
```

### Drop targets

```html
<!-- Ungrouped drop -->
<div g-drop="HandleDrop">Drop here</div>

<!-- Grouped drop — only accepts matching g-draggable:palette -->
<div g-drop:palette="AddColor">Canvas</div>

<!-- Dropzone with named value (passed as 'to' argument) -->
<div g-dropzone="'trash'" g-drop="HandleDrop">Trash</div>
```

### Go handler signatures

```go
// from = draggable payload, to = dropzone value
func (a *App) HandleDrop(from, to float64) { ... }

// With position detection ("above" or "below")
func (a *App) Reorder(from, to float64, position string) { ... }
```

### Auto-applied CSS classes

- `.g-dragging` — on the element being dragged
- `.g-drag-over` — on drop zone when compatible draggable hovers
- `.g-drag-over-above` / `.g-drag-over-below` — cursor position indicators

---

## Shadow DOM

Add `g-shadow` to a component's target element for CSS isolation:

```html
<div g-component="widget" g-shadow></div>
```

The component's template renders inside a Shadow DOM, isolated from the host page's CSS.

---

## Configuration

### Engine fields (set in Go after NewEngine)

```go
eng := godom.NewEngine()
eng.Port = 8081
eng.Host = "0.0.0.0"
eng.NoAuth = true
eng.FixedAuthToken = "my-secret"
eng.NoBrowser = true
eng.Quiet = true
eng.DisableExecJS = true
```

### Environment Variables

Set before running the binary. `NewEngine()` reads these; code overrides env.

| Variable | Description |
|----------|-------------|
| `GODOM_PORT=8081` | TCP port |
| `GODOM_HOST=0.0.0.0` | Bind address |
| `GODOM_NO_AUTH=1` | Disable token auth |
| `GODOM_TOKEN=secret` | Fixed auth token |
| `GODOM_NO_BROWSER=1` | Don't open browser |
| `GODOM_QUIET=1` | Suppress output |
| `GODOM_VALIDATE_ONLY=1` | Validate templates and exit (for CI) |
| `GODOM_DEBUG=1` | Enable debug logging |

Boolean env vars accept Go's `strconv.ParseBool` values: `1`, `t`, `true`, `TRUE`, `0`, `f`, `false`, `FALSE`.

### Browser-side variables (set before loading godom.js)

```html
<script>
window.GODOM_WS_URL = "ws://localhost:9091/ws";  // Override WebSocket URL
window.GODOM_DISABLE_EXEC = true;                 // Block ExecJS
window.GODOM_NS = "myApp";                        // Rename window.godom to window.myApp
window.GODOM_INJECT_ALLOW_ROOT = true;             // Allow root component in injected mode
</script>
```

---

## Developer-Owned Server

For full control over the HTTP server and routing:

```go
mux := http.NewServeMux()
mux.HandleFunc("/", servePage)
mux.Handle("/static/", http.FileServer(http.FS(staticFS)))

eng := godom.NewEngine()
eng.SetFS(ui)
eng.Register(counter, clock)

eng.SetMux(mux, &godom.MuxOptions{
    WSPath:     "/app/ws",        // custom WebSocket path
    ScriptPath: "/assets/godom.js", // custom script path
})

eng.SetAuth(myAuthFunc) // optional custom auth

if err := eng.Run(); err != nil {
    log.Fatal(err)
}

log.Fatal(eng.ListenAndServe())
```

`Run()` registers godom's handlers on the mux and starts processors. `ListenAndServe()` binds and serves.

---

## Embedded Mode (External Hosting)

Use godom components in pages served by something else:

**Go server (headless):**
```go
eng := godom.NewEngine()
eng.SetFS(ui)
eng.Port = 9091
eng.NoBrowser = true

stock.TargetName = "stock"
stock.Template = "ui/stock/index.html"
eng.Register(stock)

mux := http.NewServeMux()
eng.SetMux(mux, nil)
eng.Run()
log.Fatal(eng.ListenAndServe())
```

**External HTML page:**
```html
<script>window.GODOM_WS_URL = "ws://localhost:9091/ws";</script>
<script src="http://localhost:9091/godom.js"></script>

<div g-component="stock"></div>
```

---

## WebSocket Lifecycle Hooks

```js
var godom = window.godom;

godom.onconnect = function() {
    console.log("Connected");
};

godom.ondisconnect = function(errorMsg) {
    // errorMsg is null for clean close (auto-reconnects)
    // non-null means fatal error
    console.log("Disconnected:", errorMsg);
};

godom.onerror = function(evt) {
    console.log("WebSocket error");
};
```

---

## Dynamic Mounting

Mount components dynamically from JavaScript:

```js
godom.mount("component-name", targetElement);
```

This sends a `BROWSER_INIT_REQUEST` for the named component. The Go side must have it registered.

---

## CSS: Hiding Raw Templates

Before godom initializes, raw template text (`{{Count}}`, placeholder values) is briefly visible. Hide it:

```css
/* Root mode (QuickServe) */
body:not(.g-ready) { visibility: hidden; }

/* Embedded mode (g-component) */
[g-component]:not(.g-ready) { visibility: hidden; }
```

The `.g-ready` class is added after the component's initial tree renders.

---

## Common Patterns

### Todo list with struct slice

```go
type App struct {
    godom.Component
    Todos []Todo
    Input string
}

type Todo struct {
    Text string
    Done bool
}

func (a *App) Add() {
    if a.Input != "" {
        a.Todos = append(a.Todos, Todo{Text: a.Input})
        a.Input = ""
    }
}

func (a *App) Toggle(i int) {
    a.Todos[i].Done = !a.Todos[i].Done
}

func (a *App) Remove(i int) {
    a.Todos = append(a.Todos[:i], a.Todos[i+1:]...)
}
```

```html
<input g-bind="Input" g-keydown="Enter:Add" />
<button g-click="Add">Add</button>
<li g-for="todo, i in Todos">
    <input type="checkbox" g-checked="todo.Done" g-click="Toggle(i)" />
    <span g-text="todo.Text" g-class:done="todo.Done"></span>
    <button g-click="Remove(i)">x</button>
</li>
```

### Dashboard with background goroutine

```go
type Monitor struct {
    godom.Component
    CPU    float64
    Memory float64
}

func (m *Monitor) startPolling() {
    for range time.Tick(2 * time.Second) {
        m.CPU = getCPU()
        m.Memory = getMemory()
        m.Refresh()
    }
}

func main() {
    mon := &Monitor{}
    mon.Template = "ui/index.html"
    go mon.startPolling()

    eng := godom.NewEngine()
    eng.SetFS(ui)
    log.Fatal(eng.QuickServe(mon))
}
```

### Multi-page app with custom mux

```go
func main() {
    mux := http.NewServeMux()

    // Serve different pages
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        tmpl.Execute(w, pageData{Title: "Home", Script: "/godom.js"})
    })
    mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
        tmpl.Execute(w, pageData{Title: "Settings", Script: "/godom.js"})
    })

    // Each page has its own component
    home := &Home{}
    home.TargetName = "home"
    home.Template = "ui/home/index.html"

    settings := &Settings{}
    settings.TargetName = "settings"
    settings.Template = "ui/settings/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    eng.Register(home, settings)
    eng.SetMux(mux, nil)
    eng.Run()
    log.Fatal(eng.ListenAndServe())
}
```

### Serving static files alongside godom

```go
mux := http.NewServeMux()
mux.Handle("/static/", http.FileServer(http.FS(staticFS)))
eng.SetMux(mux, nil)

// After Run(), wrap with auth if needed:
// mux routes are NOT auto-protected by godom auth
// Use eng.AuthMiddleware(handler) to protect custom routes
```

---

## Gotchas and Rules

1. **Do NOT call `Refresh()` inside event handlers.** The framework auto-refreshes after every event handler. Calling it manually causes double-renders.

2. **Do NOT reset IDCounter.** Node IDs must be globally unique and monotonically increasing. Resetting causes silent DOM corruption.

3. **Event handlers are serialized.** Each component has an event queue processed by a single goroutine. Events never run concurrently within a component.

4. **`Refresh()` is thread-safe.** Call it from any goroutine.

5. **State survives browser close.** Close the tab, reopen — the state is still there because it lives in the Go process.

6. **Multi-tab sync is automatic.** All connected browsers receive patches. Open in 2 tabs — type in one, see it in both.

7. **Templates are validated at startup.** Typos in field/method names cause `log.Fatal`, not silent runtime bugs.

8. **Custom elements are template includes, not components.** They resolve against the parent's state, not their own.

9. **Component instances cannot be registered twice.** Each pointer holds its own VDOM tree. Use shared state pattern for multiple views of same data.

10. **Use single quotes for strings in HTML attributes.** `g-if="Status == 'active'"` not `g-if="Status == "active""`.

11. **Use `and`/`or`/`not` for logical operators.** Not `&&`/`||`/`!` in multi-term expressions.

12. **`g-for` variable naming:** The item variable and index are positional: `g-for="item, i in Items"`. The item comes first, index second.

---

## Project Structure Convention

```
myapp/
├── main.go              # App entry point, component definitions
├── ui/
│   ├── index.html       # Root template (or layout template)
│   ├── style.css        # Styles (linked from HTML)
│   ├── counter/
│   │   └── index.html   # Child component template
│   ├── clock/
│   │   └── index.html   # Another child component
│   └── todo-item.html   # Custom element (template include)
├── go.mod
└── go.sum
```

- Templates go in a `ui/` directory, embedded with `//go:embed ui`
- Child component templates in subdirectories
- Custom element files at root of `ui/` (filename becomes tag name)
- CSS files in `ui/`, linked via `<link>` in HTML
- Static assets served via custom mux if needed

---

## Examples Index

All examples are in the `examples/` directory. Run with `go run ./examples/<name>`.

| Example | Key Concepts |
|---------|-------------|
| `counter` | Minimal app, g-click, g-bind, g-text |
| `todolist` | g-for, g-checked, custom elements, g-keydown |
| `clock` | Background goroutine, Refresh(), SVG, g-attr |
| `progress-bar` | Goroutine, Refresh(), g-style:width |
| `stock-ticker` | Fast updates, g-class, conditional styling, static file serving |
| `sync-demo` | Multi-tab sync, MarkRefresh, surgical updates, g-mousemove |
| `drag-demo` | Drag and drop with groups, g-draggable, g-drop, g-dropzone |
| `drag-tiles` | Drag-to-reorder, CSS animations |
| `basic-form-builder` | Complex state, nested g-for, conditionals, drag groups, JSON export |
| `solar-system` | Canvas 2D, Go 3D engine, g-mousedown/move/up, g-wheel |
| `breakout-game` | Canvas game, keyboard input, collision detection, shared state |
| `system-monitor` | Live dashboard, template includes, Refresh() |
| `system-monitor-chartjs` | Chart.js plugin, live charts |
| `charts-without-plugin` | ApexCharts via inline JS adapter (no plugin package) |
| `chart-plugins` | Plotly + ECharts plugins side by side |
| `terminal` | xterm.js integration, PTY, session respawn |
| `video-player` | Canvas rendering, ffmpeg integration |
| `markdown-editor` | Two-pane editor, plain JS for scroll sync |
| `multi-component` | 9 components, g-component, cross-component callbacks, Chart.js |
| `multi-page` | Developer-owned mux, page routing |
| `embedded-widget` | External hosting, GODOM_WS_URL, g-shadow |
| `same-component-repeated` | Same struct type in multiple DOM targets |
| `shared-state` | Shared state via embedded struct pointers |
| `dynamic-mount` | godom.mount() from JavaScript |
| `exec-and-call` | ExecJS (Go→browser) and godom.call (browser→Go) |
| `ws-lifecycle` | onconnect, ondisconnect, onerror hooks |
| `select-test` | Select/dropdown binding edge cases |
| `crash-test` | Disconnect UI exercise |

> Examples with separate `go.mod` (run from their directory): `system-monitor`, `system-monitor-chartjs`, `terminal`
