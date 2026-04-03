# godom

Local GUI apps in Go using the browser as the rendering engine. Minimal JS — most apps need none, plugins bridge JS libraries when needed.

## Project scope
- All file edits must stay within this project directory (and `/tmp` if needed)
- Do not modify files outside this project directory

## Architecture
- Go package (`godom`) that developers import
- User owns the HTTP server and mux; godom registers /ws and /godom.js handlers on it
- Virtual DOM in Go: templates parsed once, resolved per render, diffed for minimal patches
- Binary WebSocket connection (Protocol Buffers) between browser and Go
- Go → browser: `VDomMessage` with tree init or diff patches (`DomPatch`)
- Browser → Go: `NodeEvent` (input sync) and `MethodCall` (event dispatch) with tagged binary format
- State lives in the Go process, survives browser close/reopen
- Single binary output via `go build`; QuickServe for simple apps, SetMux+Run+ListenAndServe for full control

## Internal packages
- `godom.go` — public API: Engine, SetFS, SetMux, Register, Run, QuickServe, ListenAndServe, SetAuth, Cleanup, Component, Refresh, MarkRefresh, ExecJS
- `internal/vdom/` — VDOM node types, template parsing, tree resolution, diffing, merging
- `internal/component/` — component struct, Info, method dispatch, field access
- `internal/server/` — WebSocket handling, connection pool, init/update pipeline
- `internal/render/` — encode patches to protobuf DomPatch, encode trees to JSON wire format
- `internal/template/` — HTML parsing, component expansion, directive validation
- `internal/bridge/` — bridge.js (DOM construction, patch execution, event handling)
- `internal/proto/` — protocol.proto, generated Go types, protocol.js, protobuf.min.js
- `internal/env/` — environment config utilities (GODOM_* env var readers)
- `internal/middleware/` — pluggable auth (AuthFunc, TokenAuth)
- `internal/utils/` — shared helpers (LocalIP, PrintQR, OpenBrowser)

## Critical invariants
- **IDCounter must never reset.** Each VDOM node gets a unique integer ID from `IDCounter`. The bridge's `nodeMap[id] → DOM node` depends on IDs being globally unique. Resetting the counter (e.g. `ci.IDCounter = &vdom.IDCounter{}` in `BuildUpdate`) causes new subtrees to reuse IDs of existing nodes, silently corrupting the bridge's nodeMap and breaking all subsequent patches. See `TestIDCounter_MustOnlyIncrement` in `internal/server/server_test.go`.
- **Prefer MarkRefresh + surgical patches over full BuildUpdate.** When the changed fields are known, use `MarkRefresh(fields...)` then `Refresh()`. This triggers surgical patches (only the bound nodes for those fields are updated) — no tree rebuild, no diff. Full `BuildUpdate` with tree diff is the expensive fallback for when specific changed fields aren't known. See `wireRefresh` in `internal/server/server.go`.
- **All component state access must go through the event queue.** Each component has a buffered `EventCh` channel and a single `processEvents` goroutine. Browser events and background refreshes are serialized through this queue. Never call `handleNodeEvent`, `handleMethodCall`, or `executeRefresh` directly from outside the processor goroutine — always send an event to `EventCh`. One exception uses `ci.Mu` directly: `handleInit` (writes the tree on new connection, must be synchronous to preserve mount order). See `processEvents` in `internal/server/server.go`.

## Key docs
- `docs/why.md` — project rationale and motivation
- `docs/architecture.md` — system design, VDOM pipeline, data flow, wire protocol
- `docs/configuration.md` — settings, environment variables, authentication, precedence rules
- `docs/plugins.md` — plugin system overview
- `docs/javascript-libraries.md` — guide for using JS libraries with godom
- `docs/drag-drop.md` — drag and drop design decisions and implementation
- `docs/nested-for.md` — nested g-for loop design
- `docs/nested-components.md` — nested component composition in embedded mode, gotchas
- `docs/planning/next.md` — prioritized roadmap (details tracked in Linear)
- `docs/transport.md` — WebSocket vs SSE+POST analysis
- `docs/protocol.md` — wire format (protobuf), transport decisions, media streaming
- `internal/proto/protocol.proto` — protobuf schema defining the wire format
- `internal/vdom/README.md` — VDOM package documentation with usage examples
