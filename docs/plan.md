# godom — Plan

> Build local GUI apps in Go using the browser as the rendering engine.
> Write HTML for the UI, Go for the logic. Minimal JavaScript — most apps need none, plugins bridge JS libraries when needed.

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
- `Refresh()` — push state from background goroutines to all connected browsers
- Stateful components: `app.Component("tag", &T{})` with props (`godom:"prop"` tags) and `Emit()` for upward communication
- Presentational components: HTML includes with `:prop="expr"` template variables

---

## Layer 3: HTML Directives — done

Implemented: `g-text`, `g-bind`, `g-click`, `g-keydown`, `g-mousedown`, `g-mousemove`, `g-mouseup`, `g-wheel`, `g-for`, `g-if`, `g-show`, `g-checked`, `g-class:name`, `g-attr:name`, `g-style:prop`.

- All expressions resolved in Go (bridge is a pure command executor)
- Per-item diffing for g-for lists (append/truncate/update, no full re-render)
- Startup validation: all directives validated against struct at Mount() time
- Expression support: field access, dotted paths, loop variables, literals

Example: `examples/progress-bar/` — animated progress bar using `g-style:width` with `Refresh()` from a goroutine.

---

## Layer 4: Todolist App — done

Two working examples:
- `examples/todolist/` — presentational components with prop passing
- `examples/todolist-stateful/` — stateful components with `Emit()` for parent communication

---

## Layer 4.5: Wire Format — done

Switched the WebSocket wire format from JSON to Protocol Buffers.

- `protocol.proto` schema with `ServerMessage`, `Command` (oneof val), `EventCommand`, `Envelope`, `WSMessage`
- Go side: `proto.Marshal`/`Unmarshal` with binary WebSocket frames
- JS side: protobuf.js light build with reflection API (no CLI codegen)
- Envelope pattern: bridge wraps pre-built bytes without inspecting them
- Plugin data stays JSON inside protobuf `bytes` fields — plugin developers never see protobuf

See [protocol.md](protocol.md) for the full rationale and alternatives considered.

---

## Layer 3.5: Drag and Drop — done

HTML5 drag-and-drop with Go-side state management.

- `g-draggable` / `g-draggable.group` — make elements draggable with optional group isolation
- `g-dropzone` — mark elements as named drop targets
- `g-drop` / `g-drop.group` — handle drops with group filtering, receives `(from, to, position)`
- Group isolation via `dataTransfer` MIME types (`application/x-godom-{group}`)
- Automatic CSS classes: `.g-dragging`, `.g-drag-over`, `.g-drag-over-above`, `.g-drag-over-below`
- Drop data sent via `Envelope.value` as JSON array, preserving string and numeric types
- `callMethod` accepts extra args (position is optional)

Examples:
- `examples/drag-tiles/` — 24 tiles with drag-to-reorder and shine animation
- `examples/drag-demo/` — groups, dropzones, string data, position detection (palette → canvas → trash)
- `examples/todolist/` — drag-to-reorder todo items

---

## Layer 5: Styling

### 5.1 CSS in HTML files — done
`<style>` tags in component HTML work naturally (no scoping yet).

### 5.2 Static file serving
- Serve a directory for images, fonts, external CSS

---

## Layer 6: JS Library Integration — done

Use JS libraries (charts, maps, editors) with a thin bridge adapter.

### 6.1 Plugin system — done
- `app.Plugin(name, scripts...)` registers a plugin with one or more JS scripts
- `g-plugin:name="Field"` directive sends Go struct data to the plugin
- Plugin JS calls `godom.register(name, {init, update})` to handle data
- Scripts injected in order before `bridge.js` — library first, then adapter
- Plugin state tracked per element for init vs update

### 6.2 Charts (Chart.js) — done
- `plugins/chartjs/` — embeds Chart.js 4.4.8 + thin bridge adapter
- Go struct `chartjs.Chart` with `map[string]interface{}` for datasets and options — any Chart.js property passes through
- `chartjs.Register(app)` registers the plugin and embeds the library
- Example: `examples/system-monitor-chartjs/` — live CPU, memory, disk, swap, load charts

---

## Layer 7: Complex App (Dashboard) — done

Prove the system works for real applications.

- ~~Stats cards~~ — done (`examples/system-monitor/`)
- ~~Real-time data updates from goroutines~~ — done via `Refresh()`
- ~~Presentational components~~ — done (`stat-card`)
- ~~Charts~~ — done (`examples/system-monitor-chartjs/` — line, doughnut, multi-dataset)
- Tables
- Routing between views

---

## Layer 7.5: Terminal App — done

Browser-based terminal with full shell access via godom.

- `examples/terminal/` — standalone example with its own `go.mod`
- PTY allocation with `creack/pty`, xterm.js for rendering
- Separate WebSocket for raw PTY I/O (godom's plugin system is one-way; terminal needs bidirectional byte streaming)
- Shell respawns automatically on exit — typing `exit` doesn't kill the app
- Session survives browser close/reopen
- Multi-browser support (multiple tabs see the same session)
- Token auth, resize handling, Tailscale-friendly network access

See `examples/terminal/implementation.md` for the full architectural deep-dive.

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
- **Concurrency:** ~~Goroutines pushing state changes (timers, background tasks)?~~ — done via `Refresh()`
- **Routing:** Single page with dynamic content, or URL-based routing?
- **Persistence:** Optional state save to disk?
- **Testing:** How to test components without a browser? (Unit tests exist for parsing, rendering, validation, and components — but no integration tests yet)
