# godom

Local GUI apps in Go using the browser as the rendering engine. Minimal JS — most apps need none, plugins bridge JS libraries when needed.

## Project scope
- All file edits must stay within this project directory (and `/tmp` if needed)
- Do not modify files outside this project directory

## Terminology
- **Island** — a stateful unit registered with the engine. Has its own goroutine, event queue, VDOM tree, and state. Declared with `godom.Island` embed, mounted with `g-island="name"`. Pattern name: islands architecture. See [docs/why-islands.md](docs/why-islands.md).
- **Partial** — a stateless HTML fragment included by custom-element tag (e.g. `<my-button>` resolves to partial content). Zero runtime cost — pure substitution at parse time. Partial content comes from either (a) a sibling file in an island's `AssetsFS`, or (b) an engine-wide registry populated via `RegisterPartial`/`UsePartials`.
- **Component** is intentionally avoided as a godom term because it implies the lightweight, stateless composition primitive from React/Vue/Angular; godom's islands are much heavier (goroutine-backed). Where "component" still appears in docs it refers to external frameworks (React components, Web Components API) — not to godom.

## Template source (Phase B)
An island picks exactly one of three sources for its entry HTML. Register() validates this at registration time.
- **`Island.AssetsFS` + `Island.Template`** — per-island embedded/disk filesystem. Template path is resolved against that FS. Local sibling partials (files next to the entry template) resolve automatically. Preferred for portable tool packages that ship Go code + HTML in one folder.
- **`Island.TemplateHTML`** — inline HTML string, no filesystem. For tiny islands (one template, no local partials). Shared partials from the registry still work.
- **`Engine.SetFS` + `Island.Template`** — engine-wide default FS. Used when an island has neither `AssetsFS` nor `TemplateHTML`. The original pre–Phase B pattern; still fully supported. `SetFS` is optional — if every island brings its own FS or uses inline HTML, skip it.

## Partial resolution order
For a custom tag like `<my-button>`:
1. Island's own FS at `path.Dir(Template)` — sibling file (no registration needed).
2. Engine's partial registry — `RegisterPartial(name, html)` or `UsePartials(fs, dir)`.
3. Not found → error listing every location searched.

Inline-HTML islands skip step 1 (no FS) and go straight to the registry.

## `<g-slot/>` children substitution
Partials may contain a `<g-slot/>` (or `<g-slot></g-slot>`) marker. When a consumer writes `<my-partial>INNER</my-partial>`, the inner content replaces every `<g-slot/>` in the partial's body. Partials without a slot silently discard children (backward compatible). Multiple slots in one partial each receive the same content.

## Architecture
- Go package (`godom`) that developers import
- User owns the HTTP server and mux; godom registers /ws and /godom.js handlers on it
- Virtual DOM in Go: templates parsed once, resolved per render, diffed for minimal patches
- Binary WebSocket connection (Protocol Buffers) between browser and Go
- Go → browser: `ServerMessage` with tree init, diff patches (`DomPatch`), or ExecJS calls
- Browser → Go: `BrowserMessage` for input sync, method calls, ExecJS results, and `godom.call`
- State lives in the Go process, survives browser close/reopen
- Single binary output via `go build`; QuickServe for simple apps, SetMux+Run+ListenAndServe for full control

## Internal packages
- `godom.go` — public API: Engine (SetFS, SetMux, Register, Run, QuickServe, ListenAndServe, SetAuth, Cleanup, RegisterPartial, UsePartials, Use, RegisterPlugin), Island (TargetName, Template, TemplateHTML, AssetsFS, Refresh, MarkRefresh, ExecJS)
- `internal/vdom/` — VDOM node types, template parsing, tree resolution, diffing, merging
- `internal/island/` — Island struct, Info, method dispatch, field access
- `internal/server/` — EngineConfig interface, BuildIslandInfo, WebSocket handling, connection pool, init/update pipeline
- `internal/render/` — encode patches to protobuf DomPatch, encode trees to JSON wire format
- `internal/template/` — HTML parsing, partial expansion (`ExpandPartials` for single-FS callers; `ExpandPartialsLayered` for layered FS + registry resolution), directive validation, `<g-slot/>` children substitution
- `internal/bridge/` — bridge.js (DOM construction, patch execution, event handling, Shadow DOM via `g-shadow`)
- `internal/proto/` — protocol.proto, generated Go types, protocol.js, protobuf.min.js
- `internal/env/` — environment config utilities (GODOM_* env var readers)
- `internal/middleware/` — pluggable auth (AuthFunc, TokenAuth)
- `internal/utils/` — shared helpers (LocalIP, PrintQR, OpenBrowser)
- `browser_extension/` — Chrome Manifest V3 extension that injects godom.js into arbitrary websites via configurable URL rules

## Critical invariants
- **IDCounter must never reset.** Each VDOM node gets a unique integer ID from `IDCounter`. The bridge's `nodeMap[id] → DOM node` depends on IDs being globally unique. Resetting the counter (e.g. `ci.IDCounter = &vdom.IDCounter{}` in `BuildUpdate`) causes new subtrees to reuse IDs of existing nodes, silently corrupting the bridge's nodeMap and breaking all subsequent patches. See `TestIDCounter_MustOnlyIncrement` in `internal/server/server_test.go`.
- **Prefer MarkRefresh + surgical patches over full BuildUpdate.** When the changed fields are known, use `MarkRefresh(fields...)` then `Refresh()`. This triggers surgical patches (only the bound nodes for those fields are updated) — no tree rebuild, no diff. Full `BuildUpdate` with tree diff is the expensive fallback for when specific changed fields aren't known. See `wireRefresh` in `internal/server/server.go`.
- **All island state access must go through the event queue.** Each island has a buffered `EventCh` channel and a single `processEvents` goroutine. Browser events and background refreshes are serialized through this queue. Never call `handleNodeEvent`, `handleMethodCall`, or `executeRefresh` directly from outside the processor goroutine — always send an event to `EventCh`. One exception uses `ci.Mu` directly: `handleInit` (writes the tree on new connection, must be synchronous so the browser receives the tree before subsequent patches). See `processEvents` in `internal/server/server.go`.
- **Init is pull-based.** On WebSocket connect, the server only pushes `document.body` init (root mode). All other islands are initialized on demand: the bridge scans for `[g-island]` elements and sends `BROWSER_INIT_REQUEST` for each. In embedded mode (no `document.body`), the bridge scans on `ws.onopen`. The server injects `window.GODOM_ROOT=true` into the JS bundle so the bridge knows which mode it's in. See `scanAndRequestIslands` in `internal/bridge/bridge.js` and the `BROWSER_INIT_REQUEST` handler in `internal/server/server.go`.

## Key docs
- `docs/llm-reference.md` — **complete API reference for AI agents** — read this to build godom apps without digging into source code
- `docs/why.md` — project rationale and motivation
- `docs/why-islands.md` — why godom calls its stateful units "islands", not "components"
- `docs/architecture.md` — system design, VDOM pipeline, data flow, wire protocol
- `docs/configuration.md` — settings, environment variables, authentication, precedence rules
- `docs/plugins.md` — plugin system overview
- `docs/javascript-libraries.md` — guide for using JS libraries with godom
- `docs/drag-drop.md` — drag and drop design decisions and implementation
- `docs/nested-for.md` — nested g-for loop design
- `docs/nested-islands.md` — nested island composition in embedded mode, gotchas
- `docs/planning/next.md` — prioritized roadmap (details tracked in Linear)
- `examples/multi-page-v2/` — reference example covering Phase B features end-to-end (AssetsFS, TemplateHTML, RegisterPartial, UsePartials, `<g-slot/>`, per-tool Go packages, html/template chrome, DirFS dev mode)
- `docs/transport.md` — WebSocket vs SSE+POST analysis
- `docs/protocol.md` — wire protocol (protobuf message types, transport)
- `internal/proto/protocol.proto` — protobuf schema defining the wire format
- `internal/vdom/README.md` — VDOM package documentation with usage examples
