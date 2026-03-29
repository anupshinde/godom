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
| 6 | Fix LastChangedFields race with concurrent Refresh | Bug | — |
| 7 | Ordered shared-state refresh | Bug | Framework Improvements |
| 8 | Connection-agnostic engine: transport interface + custom server integration | Feature | Framework Improvements |

### Medium priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 9 | Enforce Mount-before-Register ordering (+ consider `MountToRoot` rename) | Improvement | Framework Improvements |
| 10 | Inactive component pausing — skip patches when no DOM targets | Improvement | Framework Improvements |
| 11 | Streaming / append-only updates (bypass VDOM) | Feature | Framework Improvements |
| 12 | Developer experience: debug logging and element inspector | Feature | Framework Improvements |
| 13 | Example: godom components embedded in a React app | Example | — |
| 14 | Example: godom with external JS component library (e.g. Shoelace) | Example | — |

### Low priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 15 | Dynamic mount from JS: `window.godom.mount()` | Feature | Framework Improvements |
| 16 | Shadow DOM isolation (optional per-component) | Feature | Framework Improvements |
| 17 | Multiple filesystem support (AddFS) | Improvement | Framework Improvements |
| 18 | Virtual scrolling for large lists | Feature | Framework Improvements |
| 19 | Nested field binding (`Fields[Selected].Label`) | Improvement | Framework Improvements |
| 20 | Tree version guard for stale patch detection | Improvement | Framework Improvements |
| 21 | Alternative transport implementations (SSE+POST, REST API, WebTransport) | Feature | Framework Improvements |

### Future phases

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 22 | Template compiler (compile HTML + directives to Go render functions) | Feature | Phase 2 |
| 23 | Two-way plugin communication (JS library events back to Go) | Feature | Phase 3 |
