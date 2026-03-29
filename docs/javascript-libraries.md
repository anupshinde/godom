# Using JavaScript Libraries

godom is designed so that most apps need no JavaScript at all — your state, logic, and event handling all live in Go, and the browser is just a rendering engine.

But some things are better left to JS libraries that already exist — charts, maps, rich text editors, syntax highlighters. Reimplementing Chart.js or Leaflet in Go would be pointless. For these cases, godom provides a way to bridge Go data to any JS library while keeping Go as the source of truth.

### When to use plain JavaScript

Not everything needs to go through Go. For purely browser-side DOM manipulation that doesn't involve your app's state, a `<script>` tag in your HTML template is simpler and has zero latency. Good candidates:

- **Scroll synchronization** — syncing scroll positions between two panes requires reading live element dimensions (`scrollHeight`, `clientHeight`) that only the browser knows. A few lines of JS handles this instantly, while the Go round-trip adds complexity for no benefit. See `examples/markdown-editor/` for discussion.
- **Focus management** — programmatically focusing an input after a transition
- **CSS animations** — triggering or coordinating animations based on DOM state
- **Clipboard operations** — reading from or writing to the clipboard

These coexist with godom without conflict — just add a `<script>` tag to your template. Use Go for state, logic, and rendering. Use JS for browser micro-interactions that don't need state.

### When to use a JS library

You write a small JS adapter that receives data from your Go struct, and the framework handles serialization and lifecycle. There are two approaches:

| Approach | When to use | Example |
|----------|-------------|---------|
| [Keep it local](#keep-it-local) | You're integrating a library for your own app | `examples/charts-without-plugin/` |
| [Create a plugin package](#create-a-plugin-package) | You want a reusable package that others can import | `plugins/chartjs/` |

Both use the same mechanism under the hood: `eng.RegisterPlugin()` to register a JS adapter, and `g-plugin:name` in HTML to bind a Go struct field to it. The difference is just where the code lives.

---

## How it works

When you use `g-plugin:name="Field"` in your HTML, godom:

1. JSON-serializes the Go struct field
2. Sends it to the browser as a command
3. The bridge calls your adapter's `init(element, data)` the first time
4. On subsequent updates, it calls `update(element, data)`

```
Go struct field ──JSON──► bridge.js ──► your adapter ──► JS library
```

The data is whatever your Go field serializes to. Use `map[string]interface{}` for flexibility, or a typed struct — the adapter receives the JSON either way.

---

## Keep it local

This is the simpler approach. Everything lives in your application folder — no separate package needed.

### Project structure

```
myapp/
├── main.go              # app logic, calls eng.RegisterPlugin()
├── chart.go             # (optional) Go struct for the library's config
├── mylib-bridge.js      # JS adapter, embedded into the binary
└── ui/
    └── index.html       # includes the JS library via CDN
```

### Step 1: Write the JS adapter

Create a `.js` file alongside `main.go`. The adapter must call `godom.register(name, handler)` with `init` and `update` methods:

```js
// apexcharts-bridge.js
godom.register("apexcharts", {
    init: function(el, data) {
        // Called once when the element first renders.
        // Create the library instance and store it on the element.
        el.__chart = new ApexCharts(el, data);
        el.__chart.render();
    },
    update: function(el, data) {
        // Called on every subsequent render.
        // Update the existing instance — don't recreate it.
        var chart = el.__chart;
        if (!chart) return;
        chart.updateSeries(data.series, false);
    }
});
```

### Step 2: Embed and register

In `main.go`, embed the adapter file and register it with `eng.RegisterPlugin()`:

```go
//go:embed apexcharts-bridge.js
var apexBridgeJS string

func main() {
    eng := godom.NewEngine()
    eng.SetFS(ui)
    eng.RegisterPlugin("apexcharts", apexBridgeJS)
    eng.Mount(&App{}, "ui/index.html")
    log.Fatal(eng.Start())
}
```

### Step 3: Include the library in HTML

Load the JS library via CDN in your HTML. Pin the version for reproducibility:

```html
<script src="https://cdn.jsdelivr.net/npm/apexcharts@3.54.1"></script>
<div g-plugin:apexcharts="MyChart"></div>
```

The `<script>` tag runs before the adapter (which is injected before `</body>`), so the library is available when `init` is called.

### Step 4: (Optional) Add a Go struct

For cleaner data access, define a struct in a separate file:

```go
// chart.go
package main

type M = map[string]interface{}

type Chart struct {
    Chart      M        `json:"chart"`
    Series     []M      `json:"series"`
    Xaxis      M        `json:"xaxis"`
    Colors     []string `json:"colors,omitempty"`
    // ... add fields matching the library's config
}

func (c *Chart) PushPoint(label string, value float64) {
    data := c.Series[0]["data"].([]float64)
    c.Series[0]["data"] = append(data[1:], value)
    cats := c.Xaxis["categories"].([]string)
    c.Xaxis["categories"] = append(cats[1:], label)
}
```

Use it in your app struct:

```go
type App struct {
    godom.Component
    TempChart Chart
}
```

The struct's JSON tags should match the library's API so data passes straight through — no mapping needed in the JS adapter.

### Working example

See `examples/charts-without-plugin/` for a complete app using ApexCharts with this approach — two live-updating charts with no plugin package.

---

## Create a plugin package

When you want a reusable, importable package — something others can `go get` and use — create a plugin under `plugins/`.

### Project structure

```
plugins/mylib/
├── mylib.go       # Go package — Register function + optional types
├── mylib.js       # JS adapter
└── mylib.min.js   # (optional) embedded JS library
```

### Step 1: Write the JS adapter

Same as the local approach — `godom.register(name, {init, update})`:

```js
// mylib.js
godom.register("mylib", {
    init: function(el, data) {
        el.__instance = new MyLib(el, data);
    },
    update: function(el, data) {
        if (el.__instance) el.__instance.update(data);
    }
});
```

### Step 2: Write the Go package

```go
// plugins/mylib/mylib.go
package mylib

import (
    _ "embed"
    "github.com/anupshinde/godom"
)

//go:embed mylib.js
var bridgeJS string

func Register(eng *godom.Engine) {
    eng.RegisterPlugin("mylib", bridgeJS)
}
```

### Step 3: (Optional) Embed the JS library

If you want users to avoid the CDN `<script>` tag entirely, embed the library:

```go
//go:embed mylib.min.js
var libJS string

//go:embed mylib.js
var bridgeJS string

func Register(eng *godom.Engine) {
    eng.RegisterPlugin("mylib", libJS, bridgeJS)
}
```

`RegisterPlugin()` accepts variadic scripts — they're injected in order. Library first, then adapter.

### Step 4: (Optional) Add Go types

Keep types minimal. Use `map[string]interface{}` for config that mirrors the JS library's API:

```go
type Chart struct {
    Type     string                   `json:"type"`
    Labels   []string                 `json:"labels,omitempty"`
    Datasets []map[string]interface{} `json:"datasets"`
    Options  map[string]interface{}   `json:"options,omitempty"`
}
```

### Step 5: Use it

```go
import "github.com/anupshinde/godom/plugins/mylib"

func main() {
    eng := godom.NewEngine()
    eng.SetFS(ui)
    mylib.Register(eng)
    eng.Mount(&App{}, "ui/index.html")
    log.Fatal(eng.Start())
}
```

### Working example

See `plugins/chartjs/` + `examples/system-monitor-chartjs/` for the Chart.js plugin — embeds the library, provides a Go struct, and powers a full system monitor with live charts.

---

## Data flow and lifecycle

| Event | What happens |
|-------|-------------|
| Browser connects | `init(el, data)` called for each plugin element |
| User clicks a button | If the field changed, `update(el, data)` called |
| `Refresh()` from a goroutine | `update(el, data)` called for changed plugin fields |
| Browser reconnects | `init(el, data)` called again (fresh start) |

The bridge tracks which elements have been initialized using the element's `data-gid`. On reconnect, the tracking resets and `init` is called again.

---

## Tips

**Match JSON tags to the library's API.** If the JS library expects `colorScheme`, use `json:"colorScheme"` in Go. This avoids field-mapping code in the adapter.

**Keep the adapter thin.** It should just call the library's constructor in `init` and its update method in `update`. All data shaping happens in Go.

**Use `console.log(data)` during development.** See exactly what Go is sending. Remove before shipping.

**Handle missing data defensively.** Use `data.field || defaultValue` in the adapter. Go's `omitempty` can omit fields.

**Don't recreate in update.** Creating a new library instance on every update causes flicker, memory leaks, and lost internal state. Always mutate the existing instance.

**Store the instance on the element.** Use `el.__mylib = ...` in `init` so `update` can access it. The bridge doesn't track instances — your adapter does.
