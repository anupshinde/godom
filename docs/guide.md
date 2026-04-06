# godom Guide

Build local GUI apps in Go using the browser as the rendering engine. Write HTML for the UI, Go for the logic. Single binary, no JavaScript required.

---

## Quick Start

**1. Create a project:**

```
mkdir myapp && cd myapp
go mod init myapp
go get github.com/anupshinde/godom
mkdir ui
```

**2. Write your HTML** (`ui/index.html`):

```html
<!DOCTYPE html>
<html>
<body>
    <h1>Count: <span g-text="Count"></span></h1>
    <button g-click="Increment">+</button>
    <button g-click="Decrement">-</button>
</body>
</html>
```

**3. Write your Go** (`main.go`):

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
}

func (a *App) Increment() { a.Count++ }
func (a *App) Decrement() { a.Count-- }

func main() {
    app := &App{}
    app.Template = "ui/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    log.Fatal(eng.QuickServe(app))
}
```

**4. Run:**

```
go run .
```

Your default browser opens. Click the buttons. Close the tab, reopen it — the count is still there. State lives in Go.

---

## How It Works

Your Go struct is the single source of truth. The HTML template declares how state maps to UI. godom:

1. Parses HTML once at `Register()` time
2. Resolves the template against your struct on every render
3. Diffs the old and new virtual DOM trees
4. Sends minimal patches to the browser over WebSocket

You never touch the DOM. You change struct fields, and the UI updates.

---

## Components

A component is a Go struct that embeds `godom.Component`:

```go
type App struct {
    godom.Component
    Name     string
    Items    []Item
    Selected int
}
```

**Rules:**
- Exported fields become template state — `Name` is accessible as `Name` in HTML
- Exported methods become event handlers — `func (a *App) Save()` is callable via `g-click="Save"`
- Unexported fields and methods are private — invisible to templates

---

## Directives

Directives are `g-*` attributes on HTML elements that bind them to your Go state.

### Text

```html
<!-- Set element text content -->
<span g-text="Name"></span>

<!-- Inline interpolation -->
<p>Hello, {{Name}}!</p>
```

### Two-Way Binding

```html
<!-- Input syncs with Go field on every keystroke -->
<input type="text" g-bind="Name" />

<!-- Checkbox binding -->
<input type="checkbox" g-checked="Active" />
```

### One-Way Binding

```html
<!-- Read-only value (Go → browser, no sync back) -->
<input type="text" g-value="DisplayName" />
```

### Conditional Rendering

```html
<!-- Remove from DOM if falsy -->
<div g-if="HasItems">...</div>

<!-- Negation -->
<div g-if="!HasItems">No items yet.</div>

<!-- Comparisons and logical operators -->
<div g-if="Status == 'active'">Active user</div>
<div g-show="Count > 0">Has items</div>
<div g-if="Score >= Threshold and IsVerified">Qualified</div>

<!-- Hide with display:none (stays in DOM) -->
<div g-show="IsVisible">...</div>
<div g-hide="IsVisible">...</div>
```

**Truthiness:** `nil`, `false`, `0`, `""`, empty slice/map are falsy. Everything else is truthy.

**Expressions:** Directives support comparisons (`==`, `!=`, `<`, `>`, `<=`, `>=`) and logical operators (`and`, `or`, `not`). Powered by [expr-lang/expr](https://github.com/expr-lang/expr) — see their docs for the full expression syntax.

### Loops

```html
<!-- Basic loop -->
<li g-for="item in Items">
    <span g-text="item.Name"></span>
</li>

<!-- With index -->
<li g-for="item, i in Items">
    <span>{{i}}: {{item.Name}}</span>
</li>

<!-- Keyed loop (stable identity for reordering) -->
<li g-for="item in Items" g-key="item.ID">
    <span g-text="item.Name"></span>
</li>
```

Nested loops work — inner loops access outer variables:

```html
<div g-for="group in Groups">
    <h2 g-text="group.Name"></h2>
    <span g-for="item in group.Items" g-text="item.Label"></span>
</div>
```

### Attributes and Styling

```html
<!-- Set any HTML attribute -->
<img g-attr:src="ImageURL" />
<svg><rect g-attr:transform="Rotation"></rect></svg>

<!-- Conditional CSS class -->
<li g-class:selected="item.Active">...</li>
<li g-class:done="todo.Done">...</li>

<!-- Inline style property -->
<div g-style:background-color="BgColor"></div>
<div g-style:top="Box.Top" g-style:left="Box.Left"></div>
```

### Events

```html
<!-- Click -->
<button g-click="Save">Save</button>

<!-- With arguments -->
<button g-click="Remove(i)">Delete</button>
<button g-click="Move(i, todo.ID)">Move</button>

<!-- Keyboard (key filter before colon) -->
<input g-keydown="Enter:Submit" />
<input g-keydown="Escape:Cancel" />

<!-- Mouse (handler receives x, y as float64) -->
<div g-mousedown="DragStart" g-mousemove="DragMove" g-mouseup="DragEnd"></div>

<!-- Wheel (handler receives deltaY as float64) -->
<div g-wheel="Zoom"></div>
```

### Drag and Drop

```html
<!-- Make draggable (value is the payload) -->
<div g-draggable="i">Drag me</div>

<!-- Drop target (method receives from, to as float64) -->
<div g-drop="HandleDrop">Drop here</div>

<!-- Groups isolate drag sources from unrelated drop targets -->
<div g-draggable:palette="i">Color</div>
<div g-drop:palette="AddColor">Canvas</div>

<div g-draggable:list="i">Item</div>
<div g-drop:list="Reorder">List</div>
```

CSS classes `.g-dragging` and `.g-drag-over` are applied automatically during drag operations.

### Plugins

```html
<!-- Delegate rendering to a JS plugin -->
<canvas g-plugin:chartjs="ChartData"></canvas>
```

---

## Expressions

Directives accept expressions that reference your struct:

| Expression | Meaning |
|---|---|
| `Count` | Top-level field |
| `Address.City` | Nested struct field |
| `todo.Name` | Loop variable field |
| `i` | Loop index |
| `Inputs[key]` | Map value by key |
| `!Active` | Negation |
| `"literal"` | String literal |
| `42` | Number literal |
| `true` / `false` | Boolean literals |
| `Status == 'active'` | String comparison |
| `Count > 0` | Numeric comparison |
| `Score >= Threshold` | Field-to-field comparison |
| `IsAdmin and IsActive` | Logical AND |
| `not Done` | Logical NOT |
| `ComputedName()` | Zero-arg method call |

Complex expressions (comparisons, logical operators) are evaluated by [expr-lang/expr](https://github.com/expr-lang/expr). See their documentation for the full expression syntax. Note: use `and`/`or`/`not` (not `&&`/`||`/`!` for multi-term logic), and single quotes for strings in HTML attributes (`'active'` not `"active"`).

---

## Methods

Exported methods on your struct are event handlers. The framework calls them via reflection when events fire.

```go
// No arguments
func (a *App) Save() { ... }

// Loop index
func (a *App) Remove(i int) { ... }

// Mouse coordinates (float64)
func (a *App) DragMove(x, y float64) { ... }

// Drag and drop (from value, to value)
func (a *App) Reorder(from, to float64) { ... }

// Wheel
func (a *App) Zoom(deltaY float64) { ... }
```

After a method runs, godom automatically re-renders and pushes patches to all connected browsers. Do **not** call `Refresh()` inside event handlers.

---

## Background Updates

Use goroutines for live data (clocks, tickers, monitors). Call `Refresh()` to push state to browsers:

```go
type App struct {
    godom.Component
    Time string
}

func (a *App) startClock() {
    for range time.Tick(time.Second) {
        a.Time = time.Now().Format("15:04:05")
        a.Refresh()
    }
}

func main() {
    root := &App{}
    root.Template = "ui/index.html"
    go root.startClock()

    eng := godom.NewEngine()
    eng.SetFS(ui)
    log.Fatal(eng.QuickServe(root))
}
```

For high-frequency updates, use `MarkRefresh` for surgical patches that only update specific fields:

```go
func (a *App) onMouseMove(x, y float64) {
    a.PosX = x
    a.PosY = y
    a.MarkRefresh("Box") // only re-render nodes bound to Box
}
```

---

## Hiding Raw Templates (`.g-ready`)

When a page loads, there's a brief moment before godom initializes where raw template content (`{{Count}}`, placeholder text) is visible. The bridge adds a `.g-ready` CSS class to signal when a component is initialized:

- **Root mode** (`QuickServe`): added to `document.body` after the init tree renders
- **Embedded mode** (`g-component`): added to each `[g-component]` element after its init tree renders

Use this in your CSS to hide content until it's ready:

```css
/* Root mode */
body:not(.g-ready) { visibility: hidden; }

/* Embedded mode */
[g-component]:not(.g-ready) { visibility: hidden; }
```

The class is removed on cleanup (e.g. when a component is re-initialized after reconnect).

---

## Custom Elements

Split large templates by creating HTML files for sub-components:

**`ui/todo-item.html`:**
```html
<li>
    <input type="checkbox" g-checked="todo.Done" g-click="Toggle(index)" />
    <span g-text="todo.Text" g-class:done="todo.Done"></span>
    <button g-click="Remove(index)">x</button>
</li>
```

**`ui/index.html`:**
```html
<ul>
    <todo-item g-for="todo, i in Todos"></todo-item>
</ul>
```

Custom elements are template includes — directives inside the child HTML resolve against the parent component's state. Loop variables (`todo`, `i`) are available inside the child template.

---

## Multiple Components

When your app has independent pieces of state, split them into separate components. Each component is its own Go struct with its own HTML template and its own render cycle.

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
```

Set each component's target name and template, then register them:

```go
counter := &Counter{}
counter.TargetName = "counter"
counter.Template = "ui/counter/index.html"

clock.TargetName = "clock"
clock.Template = "ui/clock/index.html"

layout.Template = "ui/layout/index.html"

eng := godom.NewEngine()
eng.SetFS(ui)

eng.Register(counter, clock)
log.Fatal(eng.QuickServe(layout))
```

The layout template declares where each component renders using the `g-component` attribute:

```html
<!-- ui/layout/index.html -->
<body>
    <h1>Dashboard</h1>
    <div g-component="counter"></div>
    <div g-component="clock"></div>
</body>
```

Component templates are HTML fragments — no `<html>` or `<body>`:

```html
<!-- ui/counter/index.html -->
<div>
    <span g-text="Count"></span>
    <button g-click="Increment">+</button>
</div>
```

Each component diffs and patches independently. Updating the clock doesn't re-render the counter.

Components can communicate through Go callbacks wired in `main.go`:

```go
counter.OnChange = func(n int) { status.SetCount(n) }
```

Components can also share state by embedding a shared struct:

```go
type SharedState struct {
    Count int
}

type CounterA struct {
    godom.Component
    *SharedState
}

type CounterB struct {
    godom.Component
    *SharedState
}

shared := &SharedState{}
a := &CounterA{SharedState: shared}
a.TargetName = "a"
a.Template = "ui/a/index.html"

b := &CounterB{SharedState: shared}
b.TargetName = "b"
b.Template = "ui/b/index.html"

eng.Register(a, b)
```

When one component modifies the shared state and calls `Refresh()`, both components update. See `examples/shared-state/` and `examples/breakout-game/` for working examples.

> **Note:** Each component instance can only be registered once. The same pointer cannot be passed to `Register()` twice — each instance holds its own VDOM tree and bindings. To share state between components with different templates, use the embedded pointer pattern above.

### Without a layout (external hosting)

You can use only `Register()` without a root component. This is useful when the HTML page is served by something else (your own server, a CDN, a third-party site):

```go
stock.TargetName = "stock"
stock.Template = "ui/stock/index.html"

eng := godom.NewEngine()
eng.SetFS(ui)
eng.Port = 9091
eng.NoBrowser = true

eng.Register(stock)

mux := http.NewServeMux()
eng.SetMux(mux, nil)
eng.Run()
log.Fatal(eng.ListenAndServe())
```

The external page loads godom's JS bundle and declares the target:

```html
<script>window.GODOM_WS_URL = "ws://localhost:9091/ws";</script>
<script src="http://localhost:9091/godom.js"></script>

<div g-component="stock"></div>
```

See `examples/embedded-widget/` and [configuration.md](configuration.md#browser-side-settings) for details.

---

## Configuration

```go
eng := godom.NewEngine()
eng.Port = 8081          // default: random available port
eng.Host = "0.0.0.0"     // default: "localhost"
eng.NoAuth = true         // default: false (token auth enabled)
eng.FixedAuthToken = "my-secret"  // default: random 32-char hex
eng.NoBrowser = true      // default: false
eng.Quiet = true          // default: false
```

Environment variables also work — `GODOM_PORT=8081 GODOM_NO_BROWSER=1 go run .`. Code values take priority over env vars.

### Validate only

Validate templates without starting the server — catches unknown fields, invalid directives, and bad expressions at build time:

```
GODOM_VALIDATE_ONLY=1 go run .
```

Exits with code 0 if all `Register()` validations pass. Useful in CI pipelines and pre-commit hooks.

### Headless mode

Run on a server or Raspberry Pi without a local browser:

```
GODOM_NO_BROWSER=1 GODOM_HOST=0.0.0.0 GODOM_PORT=8081 GODOM_TOKEN=my-secret ./myapp
```

---

## Plugins

Plugins bridge JavaScript libraries for things Go can't render (charts, maps, rich editors).

**Built-in plugins: Chart.js, Plotly, ECharts**

```go
import "github.com/anupshinde/godom/plugins/chartjs"

func main() {
    app := &App{}
    app.Template = "ui/index.html"

    eng := godom.NewEngine()
    eng.SetFS(ui)
    eng.Use(chartjs.Plugin)  // or plotly.Plugin, echarts.Plugin
    log.Fatal(eng.QuickServe(app))
}
```

```html
<canvas g-plugin:chartjs="ChartData"></canvas>
```

The `ChartData` field is any struct or map that serializes to a valid Chart.js config. When the field changes, the plugin updates the chart.

See [plugins.md](plugins.md) for writing your own plugins, and [javascript-libraries.md](javascript-libraries.md) for using JS libraries without creating a reusable plugin.

> **Tip:** Not everything needs Go or a plugin. For purely browser-side micro-interactions like scroll sync, focus management, or animations, a plain `<script>` tag in your template is simpler and has zero latency. See [When to use plain JavaScript](javascript-libraries.md#when-to-use-plain-javascript).

---

## Executing JavaScript from Go (ExecJS)

Go components can execute arbitrary JavaScript in connected browsers and receive results back.

### Querying browser state

```go
func (a *App) FetchBrowserInfo() {
    a.ExecJS("({url: location.href, vw: window.innerWidth})", func(result []byte, err string) {
        if err != "" {
            log.Println("ExecJS error:", err)
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
}
```

The expression is evaluated in the browser via `eval()`. The result is `JSON.stringify`'d automatically — you receive raw JSON bytes in the callback. Unmarshal into whatever type you need.

### Executing actions

```go
func (a *App) NavigateTo(path string) {
    a.ExecJS(fmt.Sprintf("window.location.href = %q", path), func(result []byte, err string) {
        // Browser navigates — no result needed
    })
}
```

### Multiple browsers

The callback fires once per connected browser. If 3 tabs are open, you get 3 callbacks. Each response contains that browser's own state.

### Disabling ExecJS

For security, ExecJS can be disabled at two levels:

**Server-side** — prevents the server from sending ExecJS calls:

```go
eng.DisableExecJS = true
```

**Browser-side** — the page owner prevents execution regardless of server config:

```html
<script>window.GODOM_DISABLE_EXEC = true;</script>
<script src="/godom.js"></script>
```

When disabled, ExecJS callbacks receive `err = "ExecJS is disabled"`. See [configuration.md](configuration.md#godom_disable_exec) for details.

---

## Calling Go from JavaScript (godom.call)

JavaScript running in the browser — plugin code, inline scripts, third-party widgets — can call Go methods on components.

### From a plugin

```js
// In a Shoelace tree plugin's click handler:
tree.addEventListener("sl-selection-change", function(e) {
    var id = e.detail.selection[0].getAttribute("data-id");
    godom.call("SelectCategory", id);
});
```

### From inline JavaScript

```html
<button onclick="godom.call('DoSomething', 42)">Click me</button>
```

### Go side

The method is a normal exported method on the component struct:

```go
func (a *App) SelectCategory(id string) {
    // Update state — all browsers see the change
    a.SelectedID = id
}
```

Arguments are JSON-encoded. The server finds the component that has the method and dispatches the call. After the method runs, godom automatically refreshes all connected browsers.

### How godom.call finds the component

When `godom.call("MethodName", args...)` is sent, the server searches all registered components for one that has `MethodName` as an exported method. The first match wins. If no component has the method, an error is logged.

See the `examples/exec-and-call/` example for a working demo of both features.

---

## WebSocket Lifecycle Hooks

The bridge exposes three lifecycle callbacks for monitoring the WebSocket connection from JavaScript:

```js
var godom = window.godom;

godom.onconnect = function() {
    console.log("Connected to godom server");
};

godom.ondisconnect = function(errorMsg) {
    console.log("Disconnected:", errorMsg || "clean close");
};

godom.onerror = function(evt) {
    console.log("WebSocket error");
};
```

- **`onconnect`** — fires once per WebSocket connection, after the first init message. Fires again on reconnect.
- **`ondisconnect(errorMsg)`** — fires when the WebSocket closes. `errorMsg` is null for clean disconnects (server will auto-reconnect). Non-null means a fatal error.
- **`onerror(evt)`** — fires on WebSocket error, before the connection closes.

These hooks work even on pages with no `g-component` elements — useful for monitoring connection state or implementing heartbeat patterns. See `examples/ws-lifecycle/` for a working demo.

---

## Multi-Tab Sync

Open your app in two browser tabs. Type in one — both update instantly. godom broadcasts patches to all connected clients. State is always consistent because it lives in one place: your Go struct.

---

## Hot Reload

godom doesn't include a built-in file watcher. Use [Air](https://github.com/air-verse/air) for automatic rebuild and restart during development:

```
go install github.com/air-verse/air@latest
```

Create `.air.toml` in your project root:

```toml
[build]
  cmd = "go build -o ./tmp/main ."
  bin = "tmp/main"
  include_ext = ["go", "html", "css"]
  exclude_dir = ["vendor", ".git", "tmp"]
```

Then run `air` instead of `go run .`. When you save a `.go` or `.html` file, Air rebuilds and restarts the binary. The browser reconnects automatically — godom's bridge handles WebSocket reconnection out of the box.

---

## Examples

| Example | What it demonstrates |
|---|---|
| `counter` | Minimal app, click events, two-way binding |
| `todolist` | Lists, loops, custom elements, keyboard events |
| `clock` | Background goroutine, SVG, `Refresh()` |
| `progress-bar` | Animated progress bar with `Refresh()` and `g-style:width` |
| `stock-ticker` | Fast updates, conditional classes |
| `drag-demo` | Drag and drop with groups |
| `drag-tiles` | Drag-to-reorder colored tiles with animations |
| `sync-demo` | Mouse tracking, `MarkRefresh`, surgical updates |
| `basic-form-builder` | Complex state, conditionals, JSON export |
| `solar-system` | SVG animation, parameterless methods |
| `breakout-game` | Canvas game, keyboard input |
| `system-monitor` | System stats, template includes |
| `system-monitor-chartjs` | System monitor with Chart.js plugin |
| `charts-without-plugin` | ApexCharts with inline bridge adapter (no plugin package) |
| `video-player` | Canvas rendering, ffmpeg integration |
| `markdown-editor` | Two-pane editor, plain JS for scroll sync |
| `terminal` | Terminal emulation in the browser |
| `multi-component` | Stateful components, `g-component`, cross-component callbacks |
| `multi-page` | Multi-page app with user-owned mux and routing |
| `embedded-widget` | External hosting, `/godom.js`, `GODOM_WS_URL` |
| `same-component-repeated` | Same component in multiple DOM targets |
| `shared-state` | Shared state between components via embedded struct |
| `select-test` | Select/dropdown binding |
| `chart-plugins` | Plotly and ECharts plugins side by side |
| `dynamic-mount` | Dynamic component mounting via `godom.mount()` |
| `exec-and-call` | ExecJS (Go→browser) and `godom.call()` (browser→Go) |
| `ws-lifecycle` | WebSocket lifecycle hooks (`onconnect`, `ondisconnect`, `onerror`) |

Run any example:

```
cd examples/counter
go run .
```
