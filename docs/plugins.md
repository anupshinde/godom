# Creating Plugins

A godom plugin bridges Go data to a JavaScript library. You write a small JS adapter that receives data from Go and passes it to the library. The framework handles serialization, injection, and lifecycle — you just implement two functions: `init` and `update`.

This guide walks through creating a plugin from scratch.

---

## How plugins work

When you use `g-plugin:name="Field"` in your HTML, godom:

1. JSON-serializes the Go struct field
2. Sends it to the browser as a `plugin` command with the plugin name
3. The bridge looks up the registered handler for that name
4. Calls `init(element, data)` the first time, `update(element, data)` on subsequent renders

```
Go struct field ──JSON serialize──► bridge.js ──plugin op──► your handler ──► JS library
```

The data is whatever your Go struct field serializes to. Use `map[string]interface{}` for flexibility, or a typed struct if you prefer — the plugin receives the JSON either way.

---

## Step by step: building a plugin

We'll build a plugin for a hypothetical "heatmap" JS library. The same pattern works for any library — Plotly, D3, Leaflet, CodeMirror, etc.

### 1. Create the package

```
plugins/heatmap/
├── heatmap.go       # Go package — Register function + optional types
├── heatmap.js       # JS bridge adapter — init/update handlers
└── heatmap.min.js   # (optional) embedded JS library
```

### 2. Write the JS adapter

This is the core of the plugin. It must call `godom.register(name, handler)` where handler has `init` and `update` methods.

```js
// heatmap.js
godom.register("heatmap", {
    init: function(el, data) {
        // Called once when the element first renders.
        // 'el' is the DOM element with g-plugin:heatmap="..."
        // 'data' is the JSON-parsed Go struct field value.
        //
        // Create the library instance and store it on the element
        // so update() can access it later.
        el.__heatmap = new HeatmapLib(el, {
            data: data.values,
            max: data.max,
            colorScheme: data.colorScheme
        });
    },
    update: function(el, data) {
        // Called on every subsequent render.
        // Update the existing instance with new data.
        // Do NOT recreate the library instance — just update it.
        var hm = el.__heatmap;
        if (!hm) return;
        hm.setData(data.values);
        hm.setMax(data.max);
    }
});
```

Key points:
- **Store the instance on the element** (`el.__heatmap`). The bridge calls `init` once and `update` for all subsequent renders. You need a way to access the library instance in `update`.
- **`init` creates, `update` mutates**. Never recreate the library instance in `update` — that causes flicker and memory leaks.
- **`data` is plain JSON**. Whatever your Go field serializes to is what you get. Use `console.log(data)` to inspect it during development.

### 3. Write the Go package

```go
// plugins/heatmap/heatmap.go
package heatmap

import (
    _ "embed"
    "godom"
)

//go:embed heatmap.js
var bridgeJS string

// Register adds the heatmap plugin to a godom App.
func Register(app *godom.App) {
    app.Plugin("heatmap", bridgeJS)
}
```

That's the minimal plugin. The `Plugin()` method accepts variadic scripts, so if you want to embed the JS library too:

```go
//go:embed heatmap.min.js
var libJS string

//go:embed heatmap.js
var bridgeJS string

func Register(app *godom.App) {
    app.Plugin("heatmap", libJS, bridgeJS)
}
```

Scripts are injected in order — library first, then bridge adapter. This way the user doesn't need a `<script>` tag in their HTML.

### 4. (Optional) Add Go types

If the data structure is simple and stable, you can add a typed struct:

```go
type Heatmap struct {
    Values      [][]float64 `json:"values"`
    Max         float64     `json:"max"`
    ColorScheme string      `json:"colorScheme,omitempty"`
}
```

If the library has a complex or evolving config, use maps instead:

```go
type Heatmap struct {
    Data    interface{}            `json:"data"`
    Options map[string]interface{} `json:"options,omitempty"`
}
```

Maps let any library property pass through without Go type definitions. This is the approach the Chart.js plugin uses — it keeps the Go side minimal and lets users refer to the JS library's own documentation for config options.

### 5. Use it

In your app:

```go
import "godom/plugins/heatmap"

type App struct {
    godom.Component
    MyHeatmap heatmap.Heatmap
}

func main() {
    app := godom.New()
    heatmap.Register(app)
    app.Mount(&App{}, ui)
    log.Fatal(app.Start())
}
```

In your HTML:

```html
<div g-plugin:heatmap="MyHeatmap"></div>
```

The field name in `g-plugin:heatmap="MyHeatmap"` must match an exported field on your Go struct. Whenever that field changes (via a method call or `Refresh()`), the bridge calls `update` with the new data.

---

## Without a plugin package

You don't need a Go package to use a JS library. You can register a plugin inline and include the library via CDN:

```go
app.Plugin("heatmap", `
godom.register("heatmap", {
    init: function(el, data) {
        el.__hm = new HeatmapLib(el, data);
    },
    update: function(el, data) {
        if (el.__hm) el.__hm.setData(data.values);
    }
});
`)
```

```html
<script src="https://cdn.example.com/heatmap.min.js"></script>
<div g-plugin:heatmap="MyHeatmap"></div>
```

When the library is loaded via CDN, the `<script>` tag in your HTML runs before the plugin's bridge code (which is injected before `</body>`). This works but couples you to a CDN and a specific version. For production use, embedding the library in a Go package is more reliable.

---

## Data flow and lifecycle

Understanding when `init` and `update` are called:

| Event | What happens |
|-------|-------------|
| Browser connects | Bridge receives `init` message, calls `init(el, data)` for each plugin element |
| User clicks a button | If the field changed, bridge calls `update(el, data)` |
| `Refresh()` from a goroutine | Bridge calls `update(el, data)` for all plugin elements with changed data |
| Browser reconnects | Bridge re-receives `init` message, calls `init(el, data)` again (fresh start) |

The bridge tracks which elements have been initialized using the element's `data-gid`. On reconnect, the tracking resets and `init` is called again.

---

## Tips

**Match JSON tags to the library's API.** If the JS library expects `colorScheme`, use `json:"colorScheme"` in your Go struct. This avoids field-mapping code in the JS adapter and keeps it a pure passthrough.

**Keep the adapter thin.** The JS adapter should do as little as possible — ideally just call the library's constructor in `init` and its update method in `update`. All data shaping should happen in Go.

**Use `console.log(data)` during development.** Add `console.log(data)` to your `init` and `update` functions to see exactly what Go is sending. Remove it before shipping.

**Handle missing data defensively.** Use `data.field || defaultValue` patterns in the JS adapter. Go's `omitempty` can omit fields, and the adapter should handle that gracefully.

**Don't recreate in update.** Creating a new library instance on every update causes flicker, memory leaks, and lost internal state (animations, scroll position, etc.). Always mutate the existing instance.

---

## Reference: the Chart.js plugin

The `plugins/chartjs/` package is the reference implementation. It consists of:

- `chart.min.js` — Chart.js 4.4.8 (embedded, ~200KB)
- `chartjs.js` — 21-line bridge adapter
- `chartjs.go` — Register function + Chart struct with `map[string]interface{}` for datasets and options

See `examples/system-monitor-chartjs/` for a full working example with line charts, doughnut charts, multi-dataset charts, and server-pushed updates.
