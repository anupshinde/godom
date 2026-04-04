# Nested Component Composition

godom supports nested component trees where a registered component's template contains `g-component` targets for other registered components. This works in both the QuickServe pattern (single root) and the Register-only (external hosting / embedded) pattern.

## How it works

A layout component's template declares `g-component` targets. When the bridge renders the layout, those targets appear in the DOM. The bridge then discovers them and renders the child components into them.

### Example: embedded mode with nested layout

**Go side** — all components registered, no root:

```go
eng := godom.NewEngine()
eng.SetFS(ui)

// Layout is registered, not a root — it renders into the external page
layout.TargetName = "layout"
layout.Template = "ui/layout/index.html"
eng.Register(layout)

counter.TargetName = "counter"
counter.Template = "ui/counter/index.html"
eng.Register(counter)

clock.TargetName = "clock"
clock.Template = "ui/clock/index.html"
eng.Register(clock)

sidebar.TargetName = "sidebar"
sidebar.Template = "ui/sidebar/index.html"
eng.Register(sidebar)

mux := http.NewServeMux()
eng.SetMux(mux, nil)
eng.NoBrowser = true
if err := eng.Run(); err != nil {
    log.Fatal(err)
}
log.Fatal(eng.ListenAndServe())
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

This pattern supports the same interactive complexity as the QuickServe-based mode. Drag-and-drop reordering of child components within the layout, shared state between siblings, and all other godom features work correctly through the nested chain.

The VDOM pipeline (tree build, diff, patches) operates the same way regardless of whether the outer shell is served by godom or by an external page. The `nodeMap` and `IDCounter` stay consistent through the full hierarchy.

## Gotchas

### Registration order

When a child component's init arrives before the parent has rendered, the bridge queues it and retries after the next successful init. This means registration order is flexible — the bridge handles out-of-order inits automatically.

For `document.body` components (via `QuickServe`), the server ensures the root is sent first.

### Examples

The `examples/multi-component/` example demonstrates nested composition: a layout component whose template contains `g-component` targets for child components (counter, clock, sidebar, etc.) using QuickServe. The `examples/embedded-widget/` example demonstrates external hosting with flat components.

### Future changes may affect this

This works because the bridge queues pending inits and retries them after each successful init render. Changes to the init queueing mechanism or target discovery could affect nested composition. See COR-76 for the planned pull-based init model.
