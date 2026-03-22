# godom

Local GUI apps in Go using the browser as the rendering engine. Minimal JS — most apps need none, plugins bridge JS libraries when needed.

## Project scope
- All file edits must stay within this project directory (and `/tmp` if needed)
- Do not modify files outside this project directory

## Architecture
- Go package (`godom`) that developers import
- Go HTTP server serves HTML + injected JS bridge
- Virtual DOM in Go: templates parsed once, resolved per render, diffed for minimal patches
- Binary WebSocket connection (Protocol Buffers) between browser and Go
- Go → browser: `VDomMessage` with tree init or diff patches (`DomPatch`)
- Browser → Go: `NodeEvent` (input sync) and `MethodCall` (event dispatch) with tagged binary format
- State lives in the Go process, survives browser close/reopen
- Single binary output via `go build`, opens default browser on start

## Internal packages
- `godom.go` — public API: Engine, Mount, Start, Component, Refresh, MarkRefresh
- `internal/vdom/` — VDOM node types, template parsing, tree resolution, diffing, merging
- `internal/component/` — component struct, Info, method dispatch, field access
- `internal/server/` — HTTP server, WebSocket handling, connection pool, init/update pipeline
- `internal/render/` — encode patches to protobuf DomPatch, encode trees to JSON wire format
- `internal/template/` — HTML parsing, component expansion, directive validation
- `internal/bridge/` — bridge.js (DOM construction, patch execution, event handling)
- `internal/proto/` — protocol.proto, generated Go types, protocol.js, protobuf.min.js
- `internal/env/` — environment detection utilities

## Key docs
- `docs/why.md` — project rationale and motivation
- `docs/architecture.md` — system design, VDOM pipeline, data flow, wire protocol
- `docs/planning/plan.md` — phased roadmap
- `docs/planning/next.md` — future work
- `docs/planning/ideas/` — longer-term ideas
- `docs/transport.md` — WebSocket vs SSE+POST analysis
- `docs/protocol.md` — wire format (protobuf), transport decisions, media streaming
- `internal/proto/protocol.proto` — protobuf schema defining the wire format
- `internal/vdom/README.md` — VDOM package documentation with usage examples
