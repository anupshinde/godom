# godom — What's Next

All planned features, improvements, and ideas are tracked in the GODOM project on Linear.

## Upcoming work

### High priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 1 | View API: `eng.View()` for defining hash-based views | Feature | Phase 1: Navigation / Routing |
| 2 | Hash sync between browser and Go | Feature | Phase 1: Navigation / Routing |
| 3 | Per-connection view tracking and scoped broadcasts | Feature | Phase 1: Navigation / Routing |
| 4 | Go-side `Navigate()` call for programmatic routing | Feature | Phase 1: Navigation / Routing |
| 5 | Component lifecycle: unload on view switch, fix sharedPtrMaps memory leak | Feature | Phase 1: Navigation / Routing |
| 6 | Connection-agnostic engine: transport interface + custom server integration | Feature | Framework Improvements |
| 7 | Replace findComponentByNodeID tree traversal with node lookup map | Improvement | Framework Improvements |

### Medium priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 8 | Enforce Mount-before-Register ordering (+ consider `MountToRoot` rename) | Improvement | Framework Improvements |
| 9 | Inactive component pausing — skip patches when no DOM targets | Improvement | Framework Improvements |
| 10 | Streaming / append-only updates (bypass VDOM) | Feature | Framework Improvements |
| 11 | Developer experience: debug logging and element inspector | Feature | Framework Improvements |
| 12 | Example: godom components embedded in a React app | Example | — |
| 13 | Example: godom with external JS component library (e.g. Shoelace) | Example | — |

### Low priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 14 | Dynamic mount from JS: `window.godom.mount()` | Feature | Framework Improvements |
| 15 | Shadow DOM isolation (optional per-component) | Feature | Framework Improvements |
| 16 | Multiple filesystem support (AddFS) | Improvement | Framework Improvements |
| 17 | Virtual scrolling for large lists | Feature | Framework Improvements |
| 18 | Nested field binding (`Fields[Selected].Label`) | Improvement | Framework Improvements |
| 19 | Tree version guard for stale patch detection | Improvement | Framework Improvements |
| 20 | Alternative transport implementations (SSE+POST, REST API, WebTransport) | Feature | Framework Improvements |

### Future phases

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 21 | Template compiler (compile HTML + directives to Go render functions) | Feature | Phase 2 |
| 22 | Two-way plugin communication (JS library events back to Go) | Feature | Phase 3 |
