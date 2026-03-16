# Architecture

## Overview

godom has three main pieces: a **Go process** that owns all state and logic, a **JS bridge** that executes DOM commands in the browser, and a **WebSocket** that connects the two. The Go process is the application. The browser handles rendering. Plugins can extend the bridge to integrate JS libraries (charts, maps, editors) while keeping Go as the source of truth for data.

```
┌─────────────────────────────────┐       WebSocket        ┌──────────────────────┐
│           Go process            │ ◄───────────────────► │       Browser        │
│                                 │                        │                      │
│  App struct (your code)         │   commands ──────►     │  bridge.js           │
│  ├─ exported fields = state     │   (setText, setAttr,   │  ├─ executes DOM ops │
│  └─ exported methods = handlers │    appendHTML, ...)    │  └─ forwards events  │
│                                 │                        │                      │
│  godom framework                │   ◄────── events       │  HTML + CSS          │
│  ├─ parse HTML directives       │   (call, bind)         │  (your templates)    │
│  ├─ resolve expressions         │                        │                      │
│  ├─ diff state                  │                        │                      │
│  └─ emit DOM commands           │                        │                      │
└─────────────────────────────────┘                        └──────────────────────┘
```

The bridge never evaluates expressions, resolves data, or makes decisions. It receives concrete commands like "set the text of element g3 to 'hello'" and executes them. See [why.md](why.md) for the motivation behind this and [transport.md](transport.md) for why WebSocket over SSE+POST.

## Startup sequence

```
app := godom.New()
app.Component("todo-item", &TodoItem{})   // optional: register stateful components
app.Mount(&TodoApp{}, ui)                 // parse, validate, prepare
log.Fatal(app.Start())                    // serve, open browser, block
```

`Mount()` does the heavy lifting before any HTTP traffic:

1. **Find index.html** in the embedded filesystem (checks root, then one level of subdirs)
2. **Expand components** — custom element tags like `<todo-item>` are replaced with the contents of `todo-item.html`. Props (`:todo="todo"`) are encoded as `g-props` attributes. Stateful components get a `data-g-component` marker
3. **Validate directives** — every `g-*` attribute is checked against the component struct via reflection. Unknown fields or methods cause `log.Fatal`. This happens at startup, not at runtime
4. **Parse HTML** — assign `data-gid` identifiers to every element with a directive, extract bindings and events into lookup tables, replace `g-for` elements with anchor comments

`Mount()` also wires up the embedded `Component` struct's internal pointer (`ci`) so that `Refresh()` and `Emit()` work even if a goroutine starts before `Start()` is called.

`Start()` then:

1. Parses CLI flags (`--port`, `--host`, `--no-auth`, `--token`, `--no-browser`, `--quiet`) — these override framework defaults but not values set explicitly in code (see [configuration.md](configuration.md))
2. Generates a random auth token (unless `NoAuth` is set or a fixed `Token` is provided)
3. Wires the `Refresh()` callback (needs the connection pool, which only exists at start time)
4. Injects scripts before `</body>`: `protobuf.min.js`, `protocol.js`, then `godom.register()` global (if plugins exist), then plugin scripts in order, then `bridge.js`
5. Starts an HTTP server on the configured host and port, with token auth middleware on `/` and `/ws`
6. Opens the default browser with the token URL
7. Blocks forever, handling WebSocket connections

## Files

| File | Responsibility |
|------|----------------|
| `godom.go` | App, Mount, Start, WebSocket handling, connection pool, message dispatch |
| `component.go` | Component struct, componentInfo, Emit, state serialization, method/field reflection |
| `render.go` | Expression resolution, DOM command generation, init/update messages, list diffing |
| `parser.go` | HTML parsing, gid assignment, binding extraction, template expansion, component expansion |
| `validate.go` | Startup directive validation against struct types via reflection |
| `protocol.proto` | Protobuf schema defining all wire message types |
| `protocol.pb.go` | Generated Go types from `protocol.proto` |
| `protocol.js` | JS protobuf type definitions (protobuf.js reflection API) |
| `protobuf.min.js` | Protobuf.js light build (~68KB), embedded into the binary |
| `bridge.js` | Browser-side command executor (injected, never authored by the developer) |
| `plugins/chartjs/` | Chart.js plugin — embeds library + thin bridge adapter |

## Data flow

### Initial render

When a browser tab connects via WebSocket:

1. Go serializes the component struct to a JSON state map
2. For each binding (e.g., `g-text="Name"` on element `g3`), Go resolves the expression against the state and produces a command: `{op: "text", id: "g3", val: "Alice"}`
3. For each event (e.g., `g-click="Save"` on element `g5`), Go pre-builds the message the bridge should send back when clicked: `{type: "call", method: "Save"}`
4. For each `g-for` list, Go renders all items, producing HTML + per-item commands + per-item events
5. Everything is sent as a single `init` message over the WebSocket

### User interaction

When the user clicks a button with `g-click="AddTodo"`:

1. The bridge sends `{type: "call", method: "AddTodo"}` — it doesn't know what AddTodo does
2. Go snapshots the current state (JSON)
3. Go calls `AddTodo()` on the struct via reflection
4. Go snapshots the new state, diffs against the old snapshot
5. Only bindings that reference changed fields produce commands
6. For lists, per-item JSON comparison determines which items changed, which were appended, and which were removed
7. Go sends an `update` message with the minimal set of commands
8. All connected tabs receive the update (broadcast)

### Two-way binding

`g-bind="InputText"` creates both a binding (Go → browser: set input value) and an event (browser → Go: send new value on every keystroke). There is no debouncing — every keystroke is a full round trip. See [transport.md](transport.md) for why.

### Server-pushed updates (Refresh)

Not all state changes come from user interaction. Background goroutines (timers, sensors, network listeners) can mutate struct fields and call `Refresh()` to push the new state to all browsers:

1. Go locks the component mutex
2. Computes update commands for all exported fields
3. Broadcasts the update to all connected tabs

This uses the same `update` message as user-triggered changes — the bridge doesn't know the difference. The `Refresh()` callback is wired up in `Start()` because it needs the connection pool; before `Start()`, calls to `Refresh()` are no-ops (no clients to send to).

## Component model

### Presentational components

An HTML file used as a custom element tag. The parent's state flows in via `:prop="expr"` attributes. No separate Go struct — directives resolve against the parent's state with prop aliases as context variables.

```
Parent struct         ───props───►    Child HTML template
(state + methods)                     (resolves against parent state)
```

### Stateful components

A Go struct registered with `app.Component("tag", &T{})`. Has its own state, methods, and lifecycle. Props are struct fields tagged `godom:"prop"` — the parent sets them, the child owns them after that.

```
Parent struct         ───props───►    Child struct
(state + methods)     ◄───Emit────    (own state + methods)
```

`Emit("MethodName", args...)` walks up the component tree and calls the first ancestor with a matching method. This is how children communicate with parents without knowing about them.

### Child instances in g-for

When a `g-for` iterates over a list with a stateful component, godom creates one `componentInfo` per list item. These are stored in `parent.children[forGID]` and indexed by position. The scope string (e.g., `"g3:2"`) routes incoming events to the correct child instance.

## State diffing

godom uses JSON snapshot comparison, not field-level tracking:

1. Before a method call: `oldState = json.Marshal(struct)`
2. After: `newState = json.Marshal(struct)`
3. Compare top-level keys in the two JSON objects
4. Only keys with different values trigger re-renders

For `g-for` lists, each item is individually JSON-marshaled and compared against the previous render. Changed items get update commands, new items get append commands, removed items get truncate commands. No virtual DOM — the diff operates on serialized Go data, not DOM trees.

## The bridge (bridge.js)

The bridge is vanilla JS with no dependencies. It:

- Connects to `/ws` and auto-reconnects on disconnect (shows a blurred dark overlay with "Disconnected" while retrying)
- On application crash (panic in a handler), shows an "Application Crashed" overlay with the error message; does not retry
- Caches elements by `data-gid` for O(1) lookup
- Executes commands: `text`, `value`, `checked`, `display`, `class`, `attr`, `plugin`, `list`, `list-append`, `list-truncate`
- Registers event listeners that wrap pre-built protobuf bytes in an Envelope and send over the WebSocket
- Manages `g-for` anchor comments to insert/remove list items

It does not: evaluate expressions, access component state, make timing decisions, batch or debounce anything. Every decision is made in Go and sent as a concrete command.

## Plugin system

Plugins extend the bridge to delegate rendering to JS libraries. A plugin is registered with `app.Plugin(name, scripts...)` — one or more JS scripts injected in order before `bridge.js`.

The last script should call `godom.register(name, {init, update})`. When the bridge encounters a `plugin` command (from `g-plugin:name="Field"` in HTML), it calls `init(element, data)` on first render and `update(element, data)` on subsequent updates. The data is the JSON-serialized value of the Go struct field.

```
Go struct field  ──JSON──►  bridge.js  ──plugin op──►  plugin handler  ──►  JS library
```

Plugins can embed the JS library itself (e.g., `plugins/chartjs/` embeds Chart.js minified) so the user doesn't need a CDN `<script>` tag. The `Plugin()` method accepts variadic scripts — typically the library first, then the bridge adapter.

The plugin state is tracked per element by `data-gid`. The bridge calls `init` once per element and `update` for all subsequent renders.

## Wire protocol

All messages are **binary Protocol Buffers** over WebSocket. The schema is defined in `protocol.proto`.

### Go → Browser (ServerMessage)

A `ServerMessage` contains a type (`"init"` or `"update"`), a list of `Command` messages, and optionally a list of `EventCommand` messages.

Each `Command` has an `op`, target element `id`, optional `name`, and a `oneof val` that carries the value as the appropriate type (`str_val`, `bool_val`, `num_val`, or `raw_val` for JSON bytes). List operations use the `items` field.

Each `EventCommand` carries a pre-built `WSMessage` (serialized to bytes in `msg`) that the bridge sends back untouched when the event fires.

### Browser → Go (Envelope)

The bridge wraps events in an `Envelope`:

- `msg` — the pre-built `WSMessage` bytes from the `EventCommand`, forwarded untouched
- `args` — browser-side doubles (mouse coordinates, wheel delta)
- `value` — input value for `g-bind` events (JSON-encoded bytes)

The bridge never inspects `msg`. Go unpacks the `WSMessage` to determine the type (`"call"` or `"bind"`), method name, pre-resolved args, and scope.

The `scope` field in `WSMessage` routes to a child component instance within a `g-for`.

## Validation

At `Mount()` time, before the server starts, godom validates every directive in the HTML:

- `g-text="Name"` — checks that `Name` is an exported field (or a known loop variable with a valid path)
- `g-click="Save"` — checks that `Save` is an exported method
- `g-click="Remove(i)"` — checks that `Remove` is a method and `i` is a known variable
- `g-for="todo in Todos"` — checks that `Todos` is an exported field
- Dotted paths like `todo.Text` — validates `Text` exists on the element type of the slice

For stateful child components, if a directive doesn't match the parent, godom falls back to validating against registered child component structs.

Invalid directives cause `log.Fatal` — the app won't start with a typo in the HTML.
