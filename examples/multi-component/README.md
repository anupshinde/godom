# Multi-Component Demo

A 9-component dashboard demonstrating stateful components with `g-component` composition.

## Running

```
go run ./examples/multi-component
```

## How it works

Each component is an independent Go struct with its own HTML template. The root **Layout** component provides the page HTML with `g-component` placeholders. Child components render into those elements.

### Mounting components

```go
eng := godom.NewEngine()
eng.SetFS(ui)

// Root component — must be mounted first
layout := &Layout{...}
eng.Mount(layout, "ui/layout/index.html")

// Child components — registered by name, auto-wired via g-component attributes
counter := &Counter{Step: 1}
counter.TargetName = "counter"
counter.Template = "ui/counter/index.html"
eng.Register(counter)
```

`SetFS` sets the shared filesystem for templates. `Mount` mounts the root component which provides the full page HTML. `Register` registers a named child component — it renders into elements with a matching `g-component` attribute in the parent's template.

### Component targets in HTML

The layout template declares where child components render:

```html
<div g-component="navbar"></div>
<div g-component="sidebar"></div>
<div g-component="counter"></div>
```

Each child's HTML is a fragment (no `<html>`/`<head>`/`<body>`) that renders into its target element.

### Cross-component communication

Components communicate through Go callbacks wired in `main.go`:

```go
sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
```

Components don't import or know about each other's types.

### Background updates

Components with goroutine-driven state (clock, monitor, ticker, tips) call `Refresh()` to push updates:

```go
go clock.startClock()   // ticks every second, calls Refresh()
go monitor.startMonitor()
go ticker.startTicker()
go tips.startTips()
```

## Components

| Component | What it demonstrates |
|-----------|---------------------|
| **Layout** | Root component, `g-for` + `g-component` composition, drag-to-reorder |
| **Navbar** | Static data binding |
| **Sidebar** | `g-for`, `g-click` with args, `g-class` conditional styling |
| **Counter** | Click events, two-way binding |
| **Clock** | SVG rendering, goroutine-driven `Refresh()` |
| **Monitor** | Chart.js plugin integration |
| **Ticker** | `g-for` over structs, goroutine-driven updates |
| **Toast** | Cross-component callbacks, CSS animations, auto-dismiss |
| **Tips** | Character-by-character typing animation |
