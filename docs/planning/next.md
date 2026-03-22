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

