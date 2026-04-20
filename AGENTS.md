# godom

Local GUI apps in Go using the browser as the rendering engine. Minimal JS — most apps need none, plugins bridge JS libraries when needed.

## Project scope
- All file edits must stay within this project directory (and `/tmp` if needed)
- Do not modify files outside this project directory

## Architecture
- Go package (`godom`) that developers import
- Developer owns the HTTP server and mux; godom registers /ws and /godom.js handlers on it
- Virtual DOM in Go: templates parsed once, resolved per render, diffed for minimal patches
- Binary WebSocket connection (Protocol Buffers) between browser and Go
- Go → browser: `ServerMessage` with tree init or diff patches (`DomPatch`)
- Browser → Go: `BrowserMessage` with kind enum — INPUT (input sync), METHOD (event dispatch), JSRESULT (ExecJS response)
- State lives in the Go process, survives browser close/reopen
- Single binary output via `go build`; QuickServe for simple apps, SetMux+Run+ListenAndServe for full control

## Terminology
- **Island** — a stateful unit registered with the engine (`godom.Island` embed, mounted via `g-island="name"`). Has its own goroutine, event queue, VDOM tree, and state.
- **Partial** — a stateless HTML fragment included by custom-element tag. Zero runtime cost. Source: either a sibling file in an island's `AssetsFS`, or the engine-wide registry (via `RegisterPartial`/`UsePartials`).
- "Component" is avoided as a godom term; see [docs/why-islands.md](docs/why-islands.md).

## Template source (Phase B)
Each island picks one of three sources for its entry HTML:
- `Island.AssetsFS` + `Island.Template` — per-island FS (typical for tool packages shipping their own HTML).
- `Island.TemplateHTML` — inline Go string; no filesystem.
- `Engine.SetFS` + `Island.Template` — engine-wide default FS; `SetFS` is optional.

## Partial resolution for `<my-tag>`
1. Island's own FS at `path.Dir(Template)` — sibling file.
2. Engine's registry (`RegisterPartial`/`UsePartials`).
3. Otherwise error listing every location searched.

Partials may contain `<g-slot/>` — replaced by children passed between the custom element's open/close tags.

## Internal packages
- `godom.go` — public API: Engine (SetFS, SetMux, Register, Run, QuickServe, ListenAndServe, SetAuth, Cleanup, RegisterPartial, UsePartials), Island (TargetName, Template, TemplateHTML, AssetsFS, Refresh, MarkRefresh, ExecJS)
- `internal/vdom/` — VDOM node types, template parsing, tree resolution, diffing, merging
- `internal/island/` — Island struct, Info, method dispatch, field access
- `internal/server/` — WebSocket handling, connection pool, init/update pipeline
- `internal/render/` — encode patches to protobuf DomPatch, encode trees to JSON wire format
- `internal/template/` — HTML parsing, partial expansion (`ExpandPartials`, `ExpandPartialsLayered`), `<g-slot/>` substitution, directive validation
- `internal/bridge/` — bridge.js (DOM construction, patch execution, event handling, Shadow DOM via `g-shadow`)
- `internal/proto/` — protocol.proto, generated Go types, protocol.js, protobuf.min.js
- `internal/env/` — environment config utilities (GODOM_* env var readers)
- `internal/middleware/` — pluggable auth (AuthFunc, TokenAuth)
- `internal/utils/` — shared helpers (LocalIP, PrintQR, OpenBrowser)

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
- `examples/multi-page-v2/` — reference example for Phase B features (AssetsFS, TemplateHTML, RegisterPartial, UsePartials, `<g-slot/>`, DirFS dev mode)
- `docs/transport.md` — WebSocket vs SSE+POST analysis
- `docs/protocol.md` — wire format (protobuf), transport decisions, media streaming
- `internal/proto/protocol.proto` — protobuf schema defining the wire format
- `internal/vdom/README.md` — VDOM package documentation with usage examples
