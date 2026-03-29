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
| 6 | Fix LastChangedFields race with concurrent Refresh | Bug | Framework Improvements |
| 7 | Ordered shared-state refresh | Bug | Framework Improvements |

### Medium priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 8 | Enforce Mount-before-Register ordering | Improvement | Framework Improvements |
| 9 | Inactive component pausing — skip patches when no DOM targets | Improvement | Framework Improvements |
| 10 | Expose `eng.Handler()` for custom mux integration | Feature | Framework Improvements |
| 11 | Pluggable transport layer | Feature | Framework Improvements |
| 12 | Streaming / append-only updates (bypass VDOM) | Feature | Framework Improvements |
| 13 | Developer experience: debug logging and element inspector | Feature | Framework Improvements |

### Low priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 14 | Dynamic mount from JS: `window.godom.mount()` | Feature | Framework Improvements |
| 15 | Shadow DOM isolation (optional per-component) | Feature | Framework Improvements |
| 16 | Multiple filesystem support (AddFS) | Improvement | Framework Improvements |
| 17 | Virtual scrolling for large lists | Feature | Framework Improvements |
| 18 | Nested field binding (`Fields[Selected].Label`) | Improvement | Framework Improvements |
| 19 | Tree version guard for stale patch detection | Improvement | Framework Improvements |

### Future phases

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 20 | Template compiler (compile HTML + directives to Go render functions) | Feature | Phase 2 |
| 21 | Two-way plugin communication (JS library events back to Go) | Feature | Phase 3 |
