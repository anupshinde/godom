# godom — What's Next

All planned features, improvements, and ideas are tracked in the GODOM project on Linear.

## Upcoming work

### High priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 1 | COR-76: Pull-based component init (bridge requests inits instead of push order) | Improvement | Framework Improvements |
| 2 | COR-73: Reduce duplication between Engine and server.Config | Improvement | Framework Improvements |
| 3 | COR-77: Showcase example: multi-page dashboard demonstrating all godom capabilities | Example | — |

### Medium priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 4 | Inactive component pausing — skip patches when no DOM targets | Improvement | Framework Improvements |
| 5 | Streaming / append-only updates (bypass VDOM) | Feature | Framework Improvements |
| 6 | Developer experience: debug logging and element inspector | Feature | Framework Improvements |
| 7 | Example: godom components embedded in a React app | Example | — |
| 8 | Example: godom with external JS component library (e.g. Shoelace) | Example | — |

### Low priority

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 9 | COR-75: Allow CSS selectors as component targets (RegisterAt) | Feature | Framework Improvements |
| 10 | COR-78: Customizable disconnect overlay and badge | Improvement | Framework Improvements |
| 11 | Dynamic mount from JS: `window.godom.mount()` | Feature | Framework Improvements |
| 12 | Shadow DOM isolation (optional per-component) | Feature | Framework Improvements |
| 13 | Virtual scrolling for large lists | Feature | Framework Improvements |
| 14 | Nested field binding (`Fields[Selected].Label`) | Improvement | Framework Improvements |
| 15 | Tree version guard for stale patch detection | Improvement | Framework Improvements |
| 16 | Alternative transport implementations (SSE+POST, REST API, WebTransport) | Feature | Framework Improvements |

### Completed (this cycle)

| Issue | Status |
|-------|--------|
| COR-44: Enforce Mount-before-Register ordering | Done — Mount removed, server reorders document.body first |
| COR-49: Connection-agnostic engine: custom server integration | Done — SetMux, MuxOptions, user owns server |
| COR-74: Simplify multi-component2 example to use default MuxOptions | Done — renamed to multi-page, uses nil opts |
| COR-50: Multiple filesystem support (AddFS) | Cancelled — not needed, user serves own static files |

### Future phases

| # | Issue | Type | Milestone |
|---|-------|------|-----------|
| 17 | Template compiler (compile HTML + directives to Go render functions) | Feature | Phase 2 |
| 18 | Two-way plugin communication (JS library events back to Go) | Feature | Phase 3 |
