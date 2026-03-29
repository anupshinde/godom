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
4. Builds a JS bundle (protobuf + protocol + plugin bootstrap + plugin scripts + bridge), served at `/godom.js`
5. Injects `<script src="/godom.js"></script>` before `</body>` in the root HTML page
6. Starts an HTTP server on the configured host and port, with token auth middleware on `/` and `/ws`. Serves `/godom.js` for both the root page and external pages. Non-root paths are served as static files from the embedded UI filesystem (CSS, images, fonts, etc.)
7. Opens the default browser with the token URL
8. Blocks forever, handling WebSocket connections

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

### Browser → Go: two layers

The browser sends two types of messages to Go, each serving a different purpose. They use different wire tags (see [protocol.md](protocol.md)) and trigger different server-side behavior.

**Layer 1 — Input sync (`NodeEvent`, tag 0x01)**

Automatic and implicit. When an element has `g-bind="Name"`, the bridge sends a `NodeEvent` on every `input` event — every keystroke, every paste, every change. The server receives it, updates the struct field (`Name = "new value"`), and **does not re-render**. No tree resolution, no diff, no patches sent back.

This keeps Go's state perfectly in sync with what the user is typing, cheaply. A text field updating 10 times per second doesn't trigger 10 full render cycles.

The server does broadcast the new value to other connected tabs so they see the typing in real time, but this is a targeted value patch — not a tree diff.

**Layer 2 — Event dispatch (`MethodCall`, tag 0x02)**

Explicit and user-triggered. When the user clicks a button with `g-click="Save"`, the bridge sends a `MethodCall` with the method name and arguments. The server calls the method on the struct via reflection, then **re-renders**: resolves the template tree, diffs against the old tree, and broadcasts patches to all tabs.

This is the intentional state change path. Methods are where business logic lives — add an item, delete a row, toggle a flag. The re-render ensures the UI reflects the new state.

**Why two layers?**

Separating sync from action solves a fundamental tension: inputs change constantly, but re-renders are expensive.

Without Layer 1, you'd need a method for every input (`g-click="OnNameChange"`) or you'd re-render the entire tree on every keystroke. With Layer 1, the user types freely (Go stays in sync via cheap field updates), then clicks a button (Layer 2 calls the method, which reads the already-synced field). The method doesn't need to parse the input — it's already there.

```
User types "hello" into g-bind="Name"     User clicks g-click="Save"
          │                                          │
          ▼                                          ▼
   Layer 1: NodeEvent                        Layer 2: MethodCall
   5× "h","e","l","l","o"                    1× Save()
          │                                          │
          ▼                                          ▼
   Update Name field                         Call Save() via reflection
   Broadcast value to other tabs             Re-render, diff, broadcast patches
   No re-render                              All tabs update
```

**Unbound inputs**

Inputs without `g-bind` still participate in Layer 1. The bridge sends `NodeEvent` for any `<input>`, `<textarea>`, or `<select>` — even without a directive. The server stores the value and broadcasts it to other tabs. This means multi-tab sync works for all inputs, not just bound ones. The difference is that unbound input values don't map to a struct field — they're tracked by node ID in the VDOM tree.

### Server-pushed updates (Refresh)

Not all state changes come from user interaction. Background goroutines (timers, sensors, network listeners) can mutate struct fields and call `Refresh()` to push the new state to all browsers:

1. `Refresh()` sends a `RefreshKind` event to the component's event queue
2. The processor goroutine picks it up, resolves the template tree, diffs against the old tree, produces patches
3. Broadcasts the patches to all connected tabs

This uses the same `VDomMessage` patch format as user-triggered changes — the bridge doesn't know the difference.

### Event queue (concurrency model)

Each component instance has a buffered event channel (`EventCh`) and a single processor goroutine. All browser input changes (`NodeEventKind`), method calls (`MethodCallKind`), and background refreshes (`RefreshKind`) are sent to this channel and processed sequentially.

This eliminates race conditions between concurrent sources (multiple browser tabs, background goroutines) without requiring locks on the component's state. Two exceptions use `ci.Mu` directly: `findComponentByNodeID` (reads the VDOM tree from the WebSocket read loop to determine which component owns a node ID) and `handleInit` (writes the tree on new connection — must be synchronous to preserve mount order).

Two filter hooks control event flow:
- `shouldEnqueue(event)` — called before sending to the channel (for future deduplication)
- `shouldProcess(event)` — called before processing from the channel (for future filtering)

Both return `true` for now.

### Surgical refresh (MarkRefresh)

For large UIs where only a few fields changed, `MarkRefresh()` avoids a full tree diff:

1. Call `MarkRefresh("Field1", "Field2")` before `Refresh()`
2. The processor rebuilds only the nodes bound to those fields
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

Components compose via `g-component` — the parent declares named insertion points using the `g-component` attribute, children render into them. The root component provides the full HTML page (with `<body>`). Child components provide HTML fragments. On init, each component is sent to the browser with an instance name. The bridge finds target elements via `querySelectorAll('[g-component="name"]')`.

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
│  [body]     [g-component=    [g-component=        │
│  (layout)    "counter"]       "clock"]            │
└───────────────────────────────────────────────────┘
```

The bridge doesn't know there are multiple components. It sees one `nodeMap`, one WebSocket, and a sequence of init and patch messages — some targeting the body (root), others targeting elements with a matching `g-component` attribute via `targetName`. All components share a single `IDCounter` so node IDs are globally unique. When a browser event arrives, the server searches each component's tree to find which one owns the target node ID, and dispatches the event to that component.

Cross-component communication uses Go callbacks wired in `main.go`:

```go
sidebar.OnNavigate = func(msg, kind string) { toast.Show(msg, kind) }
```

Components don't know about each other's types — they communicate through function values.

### External hosting (Register-only pattern)

Components can render into pages **not served by godom**. The host page includes godom's JS bundle via a script tag and declares `g-component` targets. No `Mount()` or layout component is needed — only `Register()` + `Start()`.

```go
// No Mount() — the external page provides the HTML shell
eng.Register("stock", stock, "ui/stock/index.html")
eng.Register("marquee", marquee, "ui/stock/marquee.html")
eng.NoBrowser = true
log.Fatal(eng.Start())
```

The external page loads `/godom.js` from the godom server and sets `GODOM_WS_URL` to connect cross-origin:

```html
<script>window.GODOM_WS_URL = "ws://localhost:9091/ws";</script>
<script src="http://localhost:9091/godom.js"></script>

<div g-component="stock"></div>
<div g-component="marquee"></div>
```

The bridge uses `GODOM_WS_URL` (if set) instead of deriving the WebSocket URL from the current page's host. This allows a static HTML page on one server to connect to a godom instance on another port/host.

See `examples/embedded-widget/` for a working example. For nested component trees in embedded mode (a layout component whose template contains further `g-component` targets), see [nested-components.md](nested-components.md).

### Namespace (GODOM_NS)

The bridge registers itself on `window.godom` by default. For embedding in third-party pages where `window.godom` might conflict, set `GODOM_NS` before loading the bundle:

```html
<script>window.GODOM_NS = "myApp";</script>
<script src="http://localhost:9091/godom.js"></script>
```

The bridge and plugin registration will use `window.myApp` instead of `window.godom`. Plugins that call `godom.register(...)` still work because the server injects a bootstrap snippet that creates a local `var godom` pointing to the configured namespace object.

## The bridge (bridge.js)

The bridge is vanilla JS with no dependencies. It:

- Connects to `/ws` and auto-reconnects on disconnect (shows a blurred dark overlay with "Disconnected" while retrying)
- On application crash (panic in a handler), shows an "Application Crashed" overlay with the error message; does not retry
- On `init`: builds the entire DOM from a JSON tree description, registering every node by ID in `nodeMap`
- On `patch`: applies patches by looking up target nodes via `nodeMap[nodeID]` — text changes, fact updates, appends, removals, keyed reorders, plugin updates, and full redraws
- Applies facts: DOM properties, HTML attributes, namespaced attributes (SVG), inline styles, and event listeners
- Layer 1 (input sync): sends `NodeEvent` — cheap field update, no re-render (see [two layers](#browser--go-two-layers))
- Layer 2 (method calls): sends `MethodCall` — calls Go method, triggers re-render (see [two layers](#browser--go-two-layers))
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
