# godom — Plan

> Build local GUI apps in Go using the browser as the rendering engine.
> Write HTML for the UI, Go for the logic. Minimal JavaScript — most apps need none, plugins bridge JS libraries when needed.

---

## Layer 8: Developer Experience

### 8.1 Debugging
- Log state changes to console
- Element inspector showing Go field bindings
- Clear error messages for missing methods, bad expressions, etc.

---

## Known Issues

### Event ordering: mutex is not enough

`handleMethodCall` serializes access to component state with `ci.Mu`, but each WebSocket connection has its own read goroutine. The mutex prevents data corruption, but does **not** guarantee events execute in arrival order. Two events arriving 1ms apart may execute in either order depending on goroutine scheduling.

**Why it matters:** For single-user local apps (one browser tab) this is fine — one connection means one goroutine, so events are naturally ordered. But with multiple connections (multiple tabs, or a future multi-user scenario), concurrent method calls race for the lock. Example: counter at 5, tab A clicks increment, tab B clicks decrement at the same time — you get 5→6→5 or 5→4→5 nondeterministically. Worse for list reordering or form editing where interleaving produces invalid state.

**Fix (when needed):** Replace the per-goroutine lock acquisition with a single event loop — a channel that all connections send to, processed sequentially by one goroutine. This guarantees FIFO ordering regardless of connection count. The mutex can then be removed or reduced to protecting only the broadcast path.

Not urgent while godom targets single-user local apps, but required before any multi-client use is reliable.

---

## Open Questions

- **Persistence:** Optional state save to disk?
- **Testing:** How to test components without a browser? (Unit tests exist for parsing, rendering, validation, diffing, and merging — but no integration tests yet)
