# Nested Component Composition

godom supports nested component trees where a registered component's template contains `g-component` targets for other registered components. This works in both the standard Mount+Register pattern and the Register-only (external hosting / embedded) pattern.

## How it works

A layout component's template declares `g-component` targets. When the bridge renders the layout, those targets appear in the DOM. The bridge then discovers them and renders the child components into them.

### Example: embedded mode with nested layout

**Go side** — no `Mount()`, all components registered:

```go
eng := godom.NewEngine()
eng.SetFS(ui)

// Layout is registered, not mounted — it renders into the external page
eng.Register("layout", layout, "ui/layout/index.html")
eng.Register("counter", counter, "ui/counter/index.html")
eng.Register("clock", clock, "ui/clock/index.html")
eng.Register("sidebar", sidebar, "ui/sidebar/index.html")

eng.NoBrowser = true
log.Fatal(eng.Start())
```

**Layout template** (`ui/layout/index.html`) — contains child targets:

```html
<div class="app">
    <div g-component="sidebar"></div>
    <div class="main">
        <div g-component="counter"></div>
        <div g-component="clock"></div>
    </div>
</div>
```

**External HTML page** — only declares the layout entry point:

```html
<script>window.GODOM_WS_URL = "ws://localhost:9091/ws";</script>
<script src="http://localhost:9091/godom.js"></script>

<div g-component="layout"></div>
```

The external page declares a single `g-component` target. godom renders the full component hierarchy inside it — layout first, then its children. The external page doesn't need to know about the internal structure.

### Interactive features work fully

This pattern supports the same interactive complexity as the standard Mount-based mode. Drag-and-drop reordering of child components within the layout, shared state between siblings, and all other godom features work correctly through the nested chain.

The VDOM pipeline (tree build → diff → patches) operates the same way regardless of whether the outer shell is served by godom or by an external page. The `nodeMap` and `IDCounter` stay consistent through the full hierarchy.

## Gotchas

### Registration order matters

Components must be registered in parent-before-child order. The layout component must init before its children so that the `g-component` targets exist in the DOM when the children try to render. If a child component inits before its parent, the bridge won't find the target element.

```go
// Correct — layout first, then its children
eng.Register("layout", layout, "ui/layout/index.html")
eng.Register("counter", counter, "ui/counter/index.html")

// Wrong — counter inits before layout, target doesn't exist yet
eng.Register("counter", counter, "ui/counter/index.html")
eng.Register("layout", layout, "ui/layout/index.html")
```

There is currently no validation for this ordering. Getting it wrong results in child components silently not rendering (no error, no crash — the bridge just doesn't find a matching target element). See the roadmap for planned ordering validation.

### No dedicated example yet

This pattern has been tested (using the multi-component layout with the embedded-widget setup) but there is no dedicated example in the repo. The `examples/multi-component/` example demonstrates the layout with drag-drop reordering using the Mount pattern. The `examples/embedded-widget/` example demonstrates external hosting with flat components. The nested composition pattern combines both approaches.

### Future changes may affect this

This works because the bridge's target discovery (`querySelectorAll('[g-component="name"]')`) runs after each component init, finding targets regardless of whether they were in the original HTML or injected by another component. This behavior is not explicitly guaranteed by the framework — it's a natural consequence of how init ordering and DOM queries interact. We don't plan to break it, but changes to component init sequencing or target discovery could affect it.
