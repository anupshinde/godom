# godom — What's Next

Potential features, roughly ordered by value.

---

## Pluggable transport layer

The server currently hardcodes gorilla/websocket. The goal is to make the transport pluggable so users can choose what fits their deployment context.

**Candidates:**

| Transport | Use case | Status |
|-----------|----------|--------|
| WebSocket (gorilla) | Default — low latency, bidirectional, works everywhere | Current, stays as default |
| SSE + POST | Proxy-friendly — works behind corporate proxies that block WebSocket upgrades | Not implemented |
| REST API | Non-browser clients — CLI tools, scripts, other Go programs can interact with godom apps | Not implemented |
| WebTransport | High-frequency media — unreliable datagrams, multiple streams, no head-of-line blocking | Not stable yet (experimental Go support, limited browser support) |

**Why this is feasible:** The VDOM pipeline doesn't care how bytes move. The server sends `VDomMessage` and receives `NodeEvent`/`MethodCall` — all protobuf. The abstraction boundary already exists; the server just needs a transport interface instead of direct gorilla calls.

See [transport.md](../transport.md) for the WebSocket vs SSE+POST analysis and [protocol.md](../protocol.md) for the wire format details.

---

## Previously completed

These items were on the next list and have been implemented:

- ~~Static File Serving~~ ✅ — Non-root HTTP paths served from embedded UI filesystem via `http.FileServer`
- ~~Style Binding~~ ✅ — `g-style:prop="expr"` binds inline style properties to Go struct fields
- ~~Disconnect Handling~~ ✅ — Bridge shows overlay on disconnect/crash with auto-reconnect or error message
- ~~Publish module path~~ ✅ — Module path changed to `github.com/anupshinde/godom`
- ~~Child-local state lost when scoped event also changes root state~~ ✅ — Fixed with dual-update approach
- ~~bridge.js g-for innerHTML parsing is context-sensitive~~ ✅ — No longer applicable: VDOM rewrite builds DOM from tree descriptions, not innerHTML
- ~~Keyed identity for g-for~~ ✅ — Implemented via `g-key="item.ID"` with `KeyedElementNode` and `PatchReorder`
- ~~Virtual DOM~~ ✅ — Full VDOM pipeline: tree-based init, diff-based patches, stable node IDs
- ~~Computed properties~~ ✅ — `ResolveExpr` tries field first, falls back to calling zero-arg methods with one return value
