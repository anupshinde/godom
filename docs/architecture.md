# Architecture

## Overview

godom has three main pieces: a **Go process** that owns all state and logic, a **JS bridge** that executes DOM commands in the browser, and a **WebSocket** that connects the two. The Go process is the application. The browser is a dumb terminal.

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

`Start()` then:

1. Injects `bridge.js` before `</body>`
2. Starts an HTTP server on the configured port
3. Opens the default browser
4. Blocks forever, handling WebSocket connections

## Files

| File | Responsibility |
|------|----------------|
| `godom.go` | App, Mount, Start, WebSocket handling, connection pool, message dispatch |
| `component.go` | Component struct, componentInfo, Emit, state serialization, method/field reflection |
| `render.go` | Expression resolution, DOM command generation, init/update messages, list diffing |
| `parser.go` | HTML parsing, gid assignment, binding extraction, template expansion, component expansion |
| `validate.go` | Startup directive validation against struct types via reflection |
| `bridge.js` | Browser-side command executor (injected, never authored by the developer) |

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

The bridge is ~250 lines of vanilla JS with no dependencies. It:

- Connects to `/ws` and auto-reconnects on disconnect
- Caches elements by `data-gid` for O(1) lookup
- Executes commands: `text`, `value`, `checked`, `display`, `class`, `list`, `list-append`, `list-truncate`, `re-event`
- Registers event listeners that send pre-built messages back over the WebSocket
- Manages `g-for` anchor comments to insert/remove list items

It does not: evaluate expressions, access component state, make timing decisions, batch or debounce anything. Every decision is made in Go and sent as a concrete command.

## Wire protocol

All messages are JSON over WebSocket.

### Go → Browser

```json
{"type": "init", "commands": [...], "events": [...]}
{"type": "update", "commands": [...]}
```

Commands: `{op, id, val, name, items}` — each op maps to a single DOM mutation.

### Browser → Go

```json
{"type": "call", "method": "AddTodo", "args": [...]}
{"type": "call", "method": "Toggle", "args": [0], "scope": "g3:2"}
{"type": "bind", "field": "InputText", "value": "hello"}
```

The `scope` field routes to a child component instance within a `g-for`.

## Validation

At `Mount()` time, before the server starts, godom validates every directive in the HTML:

- `g-text="Name"` — checks that `Name` is an exported field (or a known loop variable with a valid path)
- `g-click="Save"` — checks that `Save` is an exported method
- `g-click="Remove(i)"` — checks that `Remove` is a method and `i` is a known variable
- `g-for="todo in Todos"` — checks that `Todos` is an exported field
- Dotted paths like `todo.Text` — validates `Text` exists on the element type of the slice

For stateful child components, if a directive doesn't match the parent, godom falls back to validating against registered child component structs.

Invalid directives cause `log.Fatal` — the app won't start with a typo in the HTML.
