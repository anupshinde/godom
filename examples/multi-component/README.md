# Multi-Component Demo

A 9-component dashboard demonstrating stateful components with `<g-slot>` composition.

## Running

```
go run ./examples/multi-component
```

## How it works

Each component is an independent Go struct with its own HTML template. The root **Layout** component provides the page HTML with `<g-slot>` placeholders. Child components render into those slots.

### Mounting components

```go
eng := godom.NewEngine()

// Root component — owns the page with <g-slot> tags
layout := &Layout{...}
eng.Mount(layout, ui, "ui/layout/index.html")

// Child components — each mounted separately, then placed into a slot
counter := &Counter{Step: 1}
eng.Mount(counter, ui, "ui/counter/index.html")
eng.AddToSlot(layout, "counter", counter)
```

`Mount` registers a component with its HTML template. `AddToSlot` places a child into a named `<g-slot>` in the parent's template. The root component (no `AddToSlot` call) provides the full page HTML; children provide fragments.

### Slots in HTML

The layout template declares insertion points:

```html
<g-slot name="navbar" />
<g-slot name="sidebar" />
<g-slot name="counter" />
```

Each child's HTML is a fragment (no `<html>`/`<head>`/`<body>`) that renders into its slot.

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
| **Layout** | Root component, `g-for` + `<g-slot>` composition, drag-to-reorder |
| **Navbar** | Static data binding |
| **Sidebar** | `g-for`, `g-click` with args, `g-class` conditional styling |
| **Counter** | Click events, two-way binding |
| **Clock** | SVG rendering, goroutine-driven `Refresh()` |
| **Monitor** | Chart.js plugin integration |
| **Ticker** | `g-for` over structs, goroutine-driven updates |
| **Toast** | Cross-component callbacks, CSS animations, auto-dismiss |
| **Tips** | Character-by-character typing animation |
