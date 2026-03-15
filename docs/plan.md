# godom — Plan

> Build local GUI apps in Go using the browser as the rendering engine.
> Write HTML for the UI, Go for the logic. No JavaScript authoring required.

---

## Layer 1: Transport — done

The WebSocket plumbing that everything else sits on.

- Go HTTP server serves HTML + injects JS bridge
- WebSocket connection between browser and Go
- Multi-connection broadcasting — all browser tabs stay in sync
- Auto-reconnect on disconnect
- Open default browser on startup

---

## Layer 2: Component System — done

Everything is a component. A component is a Go struct.

- Exported fields = state (serialized to JSON, sent to browser)
- Exported methods = event handlers (called from HTML directives)
- State diffing: before/after JSON snapshots, sends only changed fields
- Handler dispatch: browser sends `{method, args}`, Go calls via reflection
- Stateful components: `app.Component("tag", &T{})` with props (`godom:"prop"` tags) and `Emit()` for upward communication
- Presentational components: HTML includes with `:prop="expr"` template variables

---

## Layer 3: HTML Directives — done

Implemented: `g-text`, `g-bind`, `g-click`, `g-keydown`, `g-for`, `g-if`, `g-show`, `g-checked`, `g-class:name`.

- All expressions resolved in Go (bridge is a pure command executor)
- Per-item diffing for g-for lists (append/truncate/update, no full re-render)
- Startup validation: all directives validated against struct at Mount() time
- Expression support: field access, dotted paths, loop variables, literals

Not yet implemented: `g-attr:key`, `g-style:prop`.

---

## Layer 4: Todolist App — done

Two working examples:
- `examples/todolist/` — presentational components with prop passing
- `examples/todolist-stateful/` — stateful components with `Emit()` for parent communication

---

## Layer 5: Styling

### 5.1 CSS in HTML files — done
`<style>` tags in component HTML work naturally (no scoping yet).

### 5.2 Static file serving
- Serve a directory for images, fonts, external CSS

---

## Layer 6: JS Library Integration

Use JS libraries (charts, maps, editors) without writing JS.

### 6.1 Plugin system
- Go sends data + config, bridge delegates to a JS library
- Example: `godom.Chart("line", labels, values)` uses Chart.js internally

### 6.2 Charts
- Chart.js (or similar) behind a Go API
- Data updates via state change trigger chart re-render

---

## Layer 7: Complex App (Dashboard)

Prove the system works for real applications.

- Multiple components on one page
- Charts, tables, stats cards
- Real-time data updates from goroutines
- Routing between views

---

## Layer 8: Developer Experience

### 8.1 Hot reload
- Watch `.go` and `.html` files, rebuild and restart
- Browser auto-reconnects and gets fresh state
- `godom dev` command

### 8.2 Multi-component support — partially done
- ~~Component communication (props, events, shared state)~~ — done via props + Emit
- ~~Nested components~~ — done (presentational + stateful)
- Multiple components on one page (not in g-for context) — not yet

### 8.3 Debugging
- Log state changes to console
- Element inspector showing Go field bindings
- Clear error messages for missing methods, bad expressions, etc.

---

## Open Questions

- **Component lifecycle:** Init/Mount/Unmount hooks?
- **Computed properties:** Methods that derive from state (like `Remaining() int`)? Auto-called on render?
- **Concurrency:** Goroutines pushing state changes (timers, background tasks)?
- **Routing:** Single page with dynamic content, or URL-based routing?
- **Persistence:** Optional state save to disk?
- **Testing:** How to test components without a browser? (Unit tests exist for parsing, rendering, validation, and components — but no integration tests yet)
