# godom

Local GUI apps in Go using the browser as the rendering engine. No JS authoring required.

## Project scope
- All file edits must stay within `/Users/anup/mycode/newstuff/godom/` (and `/tmp` if needed)
- Do not modify files outside this project directory

## Architecture
- Go package (`godom`) that developers import
- Go HTTP server serves a minimal HTML page + injected JS bridge
- WebSocket connection between browser and Go for DOM operations
- All DOM operations are sequential and blocking by default
- State lives in the Go process, survives browser close/reopen
- Single binary output via `go build`, opens default browser on start

## Key docs
- `docs/why.md` — project rationale and motivation
- `docs/architecture.md` — system design, data flow, wire protocol
- `docs/plan.md` — phased roadmap
- `docs/next.md` — future work
- `docs/transport.md` — WebSocket vs SSE+POST analysis
