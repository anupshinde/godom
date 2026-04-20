# Multi-Island Demo

A 9-island dashboard demonstrating `g-island` composition.

## Running

```
go run ./examples/multi-island
```

## How it works

Each island is an independent Go struct with its own HTML template. The root **Layout** island provides the page HTML with `g-island` placeholders. Child islands render into those elements.

### Mounting islands

```go
eng := godom.NewEngine()
eng.SetFS(ui)

// Root island — must be mounted first
layout := &Layout{...}
eng.Mount(layout, "ui/layout/index.html")

// Child islands — registered by name, auto-wired via g-island attributes
counter := &Counter{Step: 1}
counter.TargetName = "counter"
counter.Template = "ui/counter/index.html"
eng.Register(counter)
```

`SetFS` sets the shared filesystem for templates. `Mount` mounts the root island which provides the full page HTML. `Register` registers a named child island — it renders into elements with a matching `g-island` attribute in the parent's template.

### Island targets in HTML

The layout template declares where child islands render:

```html
<div g-island="navbar"></div>
<div g-island="sidebar"></div>
<div g-island="counter"></div>
```

Each child's HTML is a fragment (no `<html>`/`<head>`/`<body>`) that renders into its target element.

### Cross-island communication

Islands communicate through Go callbacks wired in `main.go`:

```go
sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
```

Islands don't import or know about each other's types.

### Background updates

Islands with goroutine-driven state (clock, monitor, ticker, tips) call `Refresh()` to push updates:

```go
go clock.startClock()   // ticks every second, calls Refresh()
go monitor.startMonitor()
go ticker.startTicker()
go tips.startTips()
```

## Islands

| Island | What it demonstrates |
|-----------|---------------------|
| **Layout** | Root island, `g-for` + `g-island` composition, drag-to-reorder |
| **Navbar** | Static data binding |
| **Sidebar** | `g-for`, `g-click` with args, `g-class` conditional styling |
| **Counter** | Click events, two-way binding |
| **Clock** | SVG rendering, goroutine-driven `Refresh()` |
| **Monitor** | Chart.js plugin integration |
| **Ticker** | `g-for` over structs, goroutine-driven updates |
| **Toast** | Cross-island callbacks, CSS animations, auto-dismiss |
| **Tips** | Character-by-character typing animation |
