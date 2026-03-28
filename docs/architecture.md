# Architecture

## Overview

godom has three main pieces: a **Go process** that owns all state and logic, a **JS bridge** that renders DOM in the browser, and a **WebSocket** that connects the two. The Go process is the application. The browser handles rendering. Plugins can extend the bridge to integrate JS libraries (charts, maps, editors) while keeping Go as the source of truth for data.

The core abstraction is a **virtual DOM** — Go builds a tree of virtual nodes from HTML templates, diffs it against the previous tree, and sends only the minimal patches over the wire. The bridge applies these patches to the real DOM.

```
┌─────────────────────────────────┐       WebSocket        ┌──────────────────────┐
│           Go process            │ ◄───────────────────► │       Browser        │
│                                 │                        │                      │
│  App struct (your code)         │   VDomMessage ────►    │  bridge.js           │
│  ├─ exported fields = state     │   (init tree,          │  ├─ builds DOM       │
│  └─ exported methods = handlers │    diff patches)       │  ├─ applies patches  │
│                                 │                        │  └─ forwards events  │
│  godom framework                │   ◄────── events       │                      │
│  ├─ parse HTML templates        │   (NodeEvent,          │  HTML + CSS          │
│  ├─ resolve to VDOM tree        │    MethodCall)         │  (your templates)    │
│  ├─ diff old tree vs new tree   │                        │                      │
│  └─ encode patches (protobuf)   │                        │                      │
└─────────────────────────────────┘                        └──────────────────────┘
```

The bridge never evaluates expressions, resolves data, or makes decisions. It receives a tree description on init and minimal patches on updates, applying them to the DOM. See [why.md](why.md) for the motivation behind this and [transport.md](transport.md) for why WebSocket over SSE+POST.

## Startup sequence

```go
eng := godom.NewEngine()
eng.SetFS(ui)                                          // set shared UI filesystem
eng.Mount(&TodoApp{}, "ui/index.html")                 // parse, validate, prepare
log.Fatal(eng.Start())                                 // serve, open browser, block
```

`Mount()` does the heavy lifting before any HTTP traffic:

1. **Read the entry HTML** from the embedded filesystem at the given path (e.g., `"ui/index.html"`)
2. **Expand components** — custom element tags like `<todo-item>` are replaced with the contents of `todo-item.html`. `g-*` attributes from the custom tag are transferred to the component's root element
3. **Validate directives** — every `g-*` attribute is checked against the component struct via reflection. Unknown fields or methods cause `log.Fatal`. This happens at startup, not at runtime
4. **Parse templates** — the expanded HTML is parsed into a reusable `[]*vdom.TemplateNode` tree. Directives, text interpolations (`{{expr}}`), `g-for` loops, and plugin bindings are all extracted into structured template nodes. This tree is parsed once and reused on every render

`Mount()` also wires up the embedded `Component` struct's internal pointer (`ci`) so that `Refresh()` and `MarkRefresh()` work even if a goroutine starts before `Start()` is called.

`Start()` then:

1. Reads `GODOM_*` environment variables for any settings not already set in code (see [configuration.md](configuration.md))
2. Generates a random auth token (unless `NoAuth` is set or a fixed `Token` is provided)
3. Wires the `Refresh()` callback (needs the connection pool, which only exists at start time)
4. Injects scripts before `</body>`: `protobuf.min.js`, `protocol.js`, then `godom.register()` global (if plugins exist), then plugin scripts in order, then `bridge.js`
5. Starts an HTTP server on the configured host and port, with token auth middleware on `/` and `/ws`. Non-root paths are served as static files from the embedded UI filesystem (CSS, images, fonts, etc.)
6. Opens the default browser with the token URL
7. Blocks forever, handling WebSocket connections

## Internal packages

| Package | Responsibility |
|---------|----------------|
| `godom.go` | Engine, Mount, Start — the public API surface |
| `internal/vdom/` | Virtual DOM: node types, template parsing, tree resolution, diffing, merging, patch types |
| `internal/component/` | Component struct, Info, method dispatch, field access |
| `internal/server/` | HTTP server, WebSocket handling, connection pool, init/update pipeline, surgical refresh |
| `internal/render/` | Encode patches to protobuf `DomPatch`, encode VDOM trees to JSON wire format |
| `internal/template/` | HTML parsing, component expansion, directive validation |
| `internal/bridge/` | `bridge.js` — browser-side DOM construction, patch execution, event handling |
| `internal/proto/` | `protocol.proto`, generated Go types, `protocol.js`, `protobuf.min.js` |
| `internal/env/` | Environment detection utilities |
| `plugins/chartjs/` | Chart.js plugin — embeds library + thin bridge adapter |

## Data flow

### Initial render

When a browser tab connects via WebSocket:

1. Go resolves the template tree against the component struct's current state, producing a concrete VDOM node tree. Each node gets a unique stable ID from a monotonic counter
2. The tree is encoded as a JSON description — element tags, facts (properties, attributes, styles, events), and children
3. Go sends a `VDomMessage` with `type: "init"` containing the full tree as JSON bytes
4. The bridge builds the entire DOM from the tree description, registering each node's ID in `nodeMap` for O(1) lookup on subsequent patches
5. Event handlers in the tree are wired up as DOM listeners that send `MethodCall` messages back to Go

### User interaction

When the user clicks a button with `g-click="AddTodo"`:

1. The bridge sends a `MethodCall` message with `method: "AddTodo"` and the node ID
2. Go calls `AddTodo()` on the struct via reflection
3. Go resolves the template tree again against the new state, producing a new VDOM tree
4. Go diffs the old tree against the new tree, producing a minimal list of patches (text changes, fact changes, appends, removals, reorders)
5. The old tree is updated in place via `MergeTree` to preserve node IDs for the next render
6. Go sends a `VDomMessage` with `type: "patch"` containing the patches as protobuf `DomPatch` messages
7. The bridge applies each patch by looking up target nodes via `nodeMap[nodeID]`
8. All connected tabs receive the patches (broadcast)

### Two-way binding

`g-bind="InputText"` creates both a binding (Go → browser: set input value) and an event (browser → Go: send new value on every keystroke via `NodeEvent`). There is no debouncing — every keystroke is a full round trip. See [transport.md](transport.md) for why.

### Server-pushed updates (Refresh)

Not all state changes come from user interaction. Background goroutines (timers, sensors, network listeners) can mutate struct fields and call `Refresh()` to push the new state to all browsers:

1. Go locks the component mutex
2. Resolves the template tree, diffs against the old tree, produces patches
3. Broadcasts the patches to all connected tabs

This uses the same `VDomMessage` patch format as user-triggered changes — the bridge doesn't know the difference.

### Surgical refresh (MarkRefresh)

For large UIs where only a few fields changed, `MarkRefresh()` avoids a full tree diff:

1. Call `MarkRefresh("Field1", "Field2")` before `Refresh()`
2. The server rebuilds only the nodes bound to those fields
3. If the partial rebuild produces patches, they are sent. Otherwise, it falls back to a full rebuild

This is the primary optimization for dashboards and large lists where one item changed.

## Virtual DOM

The VDOM is the core abstraction. It lives in `internal/vdom/`.

### Node types

| Go type | Description |
|---------|-------------|
| `*TextNode` | Leaf node containing plain text |
| `*ElementNode` | HTML/SVG element with tag, facts, and ordered children |
| `*KeyedElementNode` | Like `ElementNode` but children have stable string keys for efficient reordering |
| `*PluginNode` | Opaque node whose rendering is delegated to a JS library |
| `*LazyNode` | Deferred computation — if function and args are reference-equal to last render, subtree is skipped |

Every node has a stable numeric ID assigned during `ResolveTree()` via a monotonic `IDCounter` that never resets across renders. Patches reference the old tree's IDs because those are what the bridge already has in its `nodeMap`.

### Facts (element metadata)

`Facts` groups everything about an element that isn't its tag or children:

```go
type Facts struct {
    Props   map[string]any          // DOM properties: className, value, checked, id
    Attrs   map[string]string       // HTML attributes: data-*, aria-*, role, etc.
    AttrsNS map[string]NSAttr       // Namespaced attributes (SVG): xlink:href, xml:lang
    Styles  map[string]string       // Inline CSS: background-color, width, etc.
    Events  map[string]EventHandler // Event listeners: click, input, keydown, etc.
}
```

The diff algorithm diffs all facts in one pass (`DiffFacts()`) and produces a `FactsDiff` with only the changed/added/removed entries.

### Diffing

`Diff(oldTree, newTree)` produces a minimal list of patches:

| Patch type | When emitted |
|------------|-------------|
| `PatchRedraw` | Node type or element tag changed — full replacement |
| `PatchText` | Text node content changed |
| `PatchFacts` | Properties, attributes, styles, or events changed |
| `PatchAppend` | New children added at the end |
| `PatchRemoveLast` | N children removed from the end |
| `PatchReorder` | Keyed children inserted, removed, or moved |
| `PatchPlugin` | Plugin data changed (JSON comparison) |
| `PatchLazy` | Wrapper for patches inside a lazy node's subtree |

For non-keyed children, diffing is positional — same position, same identity. For keyed children (`KeyedElementNode`), the differ detects inserts, deletes, and moves, producing `PatchReorder` with minimal operations.

### Tree merging

After diffing, `MergeTree(oldTree, newTree)` updates the old tree in place: structurally matching nodes get their data updated while keeping their IDs. This ensures the next render cycle's diff operates against nodes with the correct IDs.

### Template resolution

The template system is a two-phase pipeline:

1. **Parse once** (`ParseTemplate`) — HTML with `g-*` directives is parsed into `[]*TemplateNode`. This happens at `Mount()` time and the result is reused
2. **Resolve per render** (`ResolveTree`) — the template tree is evaluated against the current component state, producing concrete `[]Node` with resolved values, unrolled loops, evaluated conditionals, and assigned IDs

Expression resolution uses a fast path for simple expressions (field access, dotted paths, loop variables, negation, zero-arg methods) via direct reflection. Complex expressions with comparisons (`==`, `!=`, `<`, `>`, `<=`, `>=`) and logical operators (`and`, `or`, `not`) are evaluated by [expr-lang/expr](https://github.com/expr-lang/expr) with compiled program caching.

## Component model

### Presentational components

An HTML file used as a custom element tag. No separate Go struct — directives inside the child template resolve against the parent's state. Loop variables from `g-for` are available inside the child template.

```
Parent struct         ──state──►    Child HTML template
(state + methods)                   (resolves against parent state)
```

### Stateful components

Each component is a self-contained unit: own Go struct, own HTML template, own VDOM tree, own diff cycle. They are like small independent applications that all run inside the same Go process and render through the same bridge.

```
eng.SetFS(ui)
eng.Register("counter", counter, "ui/counter/index.html")  // child component
eng.Mount(layout, "ui/layout/index.html")                  // root component
```

Components compose via `<g-slot>` — the parent declares named insertion points, children render into them. The root component provides the full HTML page (with `<body>`). Child components provide HTML fragments. On init, components are sent to the browser in topological order (parents before children). Each child targets a specific VDOM node ID in its parent's tree.

```
┌─────────────────── Go process ───────────────────┐
│                                                   │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐          │
│  │ Layout  │  │ Counter │  │  Clock  │  ...      │
│  │ struct  │  │ struct  │  │ struct  │           │
│  │ VDOM    │  │ VDOM    │  │ VDOM    │           │
│  │ diff    │  │ diff    │  │ diff    │           │
│  └────┬────┘  └────┬────┘  └────┬────┘           │
│       │            │            │                 │
│       └────────────┼────────────┘                 │
│                    │                              │
│              shared IDCounter                     │
│              event routing                        │
│                    │                              │
│              ┌─────┴─────┐                        │
│              │  server   │                        │
│              └─────┬─────┘                        │
└────────────────────┼─────────────────────────────┘
                     │  one WebSocket (or transport)
                     │
┌────────────────────┼─────────────────────────────┐
│  Browser           │                              │
│              ┌─────┴─────┐                        │
│              │  bridge   │                        │
│              │  one      │                        │
│              │  nodeMap  │                        │
│              └─────┬─────┘                        │
│                    │                              │
│    ┌───────────────┼───────────────┐              │
│    ▼               ▼               ▼              │
│  [body]     [slot:counter]   [slot:clock]         │
│  (layout)   (counter DOM)    (clock DOM)          │
└───────────────────────────────────────────────────┘
```

The bridge doesn't know there are multiple components. It sees one `nodeMap`, one WebSocket, and a sequence of init and patch messages — some targeting the body (root), others targeting slot elements via `targetNodeId`. All components share a single `IDCounter` so node IDs are globally unique. When a browser event arrives, the server searches each component's tree to find which one owns the target node ID, and dispatches the event to that component.

Cross-component communication uses Go callbacks wired in `main.go`:

```go
sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
```

Components don't know about each other's types — they communicate through function values.

## The bridge (bridge.js)

The bridge is vanilla JS with no dependencies. It:

- Connects to `/ws` and auto-reconnects on disconnect (shows a blurred dark overlay with "Disconnected" while retrying)
- On application crash (panic in a handler), shows an "Application Crashed" overlay with the error message; does not retry
- On `init`: builds the entire DOM from a JSON tree description, registering every node by ID in `nodeMap`
- On `patch`: applies patches by looking up target nodes via `nodeMap[nodeID]` — text changes, fact updates, appends, removals, keyed reorders, plugin updates, and full redraws
- Applies facts: DOM properties, HTML attributes, namespaced attributes (SVG), inline styles, and event listeners
- Layer 1 (input sync): sends `NodeEvent` with the element's current value on every `input` event
- Layer 2 (method calls): sends `MethodCall` with method name and JSON-encoded arguments
- Manages HTML5 drag-and-drop: `draggable` sets up `dragstart`/`dragend` with group-specific MIME types; drop handlers filter by group, apply CSS feedback classes (`.g-dragging`, `.g-drag-over`, `.g-drag-over-above`/`.g-drag-over-below`), and send drop data via `MethodCall`
- Defers plugin `init` calls until the element is actually in the DOM (for libraries that need to measure dimensions)
- Preserves focus and selection across patch application

It does not: evaluate expressions, access component state, make timing decisions, batch or debounce anything. Every decision is made in Go.

## Plugin system

Plugins extend the bridge to delegate rendering to JS libraries. A plugin is registered with `eng.RegisterPlugin(name, scripts...)` — one or more JS scripts injected in order before `bridge.js`.

The last script should call `godom.register(name, {init, update})`. When the bridge encounters a `PatchPlugin` (from `g-plugin:name="Field"` in HTML), it calls `init(element, data)` on first render and `update(element, data)` on subsequent updates. The data is the JSON-serialized value of the Go struct field.

```
Go struct field  ──JSON──►  bridge.js  ──plugin patch──►  plugin handler  ──►  JS library
```

Plugins can embed the JS library itself (e.g., `plugins/chartjs/` embeds Chart.js minified) so the user doesn't need a CDN `<script>` tag. `RegisterPlugin()` accepts variadic scripts — typically the library first, then the bridge adapter.

The plugin state is tracked per node ID. The bridge calls `init` once per element and `update` for all subsequent renders.

## Wire protocol

All messages are **binary Protocol Buffers** over WebSocket. The schema is defined in `internal/proto/protocol.proto`.

### Go → Browser (VDomMessage)

A `VDomMessage` contains a `type` (`"init"` or `"patch"`).

- **init**: carries a `tree` field — JSON-encoded tree description of the entire DOM. The bridge builds the DOM from this description.
- **patch**: carries a list of `DomPatch` messages, each targeting a node by its stable numeric ID with an operation type and type-specific payload (text content, facts diff, tree description for redraws/appends, count for removals, reorder operations, or plugin data).

### Browser → Go (tagged binary)

The browser sends binary messages with a one-byte tag prefix:

- **Tag 0x01 — `NodeEvent`**: Layer 1 auto-sync of input values. Contains `node_id` and `value` (the current DOM value of the input element). Sent on every `input` event for elements with `g-bind`.
- **Tag 0x02 — `MethodCall`**: Layer 2 event dispatch. Contains `node_id`, `method` name, and JSON-encoded `args`. Sent when the user triggers an event (click, keydown, drop, etc.).

The bridge constructs these messages directly from event data — it does not wrap pre-built bytes.

## Drag and drop

Drag-and-drop splits responsibility between the bridge and Go. The bridge handles the HTML5 DnD ceremony (event listeners, `preventDefault`, `dataTransfer`, CSS feedback classes). Go handles the semantics (what data is being dragged, what happens on drop).

This split exists because `dragover` fires continuously and requires synchronous `preventDefault()` — a round trip to Go would be too slow. But the final drop result is always sent to Go, where the method decides what to do (reorder a slice, add an item, delete an item).

Groups use `dataTransfer` MIME types (`application/x-godom-{group}`) for filtering, requiring zero JS-side state. CSS classes (`.g-dragging`, `.g-drag-over`, `.g-drag-over-above/below`) are applied directly by the bridge for instant visual feedback.

See [drag-drop.md](drag-drop.md) for the full design rationale and alternatives considered.

## Validation

At `Mount()` time, before the server starts, godom validates every directive in the HTML:

- `g-text="Name"` — checks that `Name` is an exported field (or a known loop variable with a valid path)
- `g-click="Save"` — checks that `Save` is an exported method
- `g-click="Remove(i)"` — checks that `Remove` is a method and `i` is a known variable
- `g-for="todo in Todos"` — checks that `Todos` is an exported field
- Dotted paths like `todo.Text` — validates `Text` exists on the element type of the slice

Invalid directives cause `log.Fatal` — the app won't start with a typo in the HTML.
