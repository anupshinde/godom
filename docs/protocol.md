# Wire Protocol

## Wire format: Protocol Buffers (done)

The WebSocket wire format uses **Protocol Buffers** for all communication between Go and the browser. This replaced the original JSON-based protocol.

**Why protobuf:**

- **Binary format** — smaller wire size, faster encode/decode than JSON
- **Schema as contract** — `protocol.proto` defines the protocol formally. No guessing message formats from source code
- **Multi-language support** — any language (Python, Rust, Java, etc.) can generate a client from the `.proto` file and talk to a godom app
- **No user-facing impact** — users write Go structs and HTML directives. They never see the wire format. All examples work unchanged
- **Future-proof** — protobuf handles schema evolution cleanly. Adding fields is backward compatible

**Alternatives considered and rejected:**

| Option | Why not |
|--------|---------|
| FlatBuffers | Zero-copy advantage only matters for large messages where you read a few fields. godom reads every field in every message — no benefit over protobuf |
| Custom binary protocol | Protobuf already is a binary protocol, but with codegen, schemas, and multi-language support. No reason to hand-roll one |
| String concatenation | A hack to avoid JSON cloning overhead. Irrelevant with protobuf |

**How it works internally:**

The protocol has two message flows:

### Go → Browser: ServerMessage

`ServerMessage` is the single message type sent from Go to the browser. The `kind` field (a `ServerKind` enum) determines which payload fields are relevant:

**SERVER_INIT** — carries a `tree` field (JSON-encoded VDOM tree) and a `target` (island name). The bridge builds the DOM from this description on first connect or reconnect.

**SERVER_PATCH** — carries a list of `DomPatch` messages and a `target`. Minimal mutations computed by diffing the old and new VDOM trees.

Each `DomPatch` targets a node by its stable numeric `node_id` (assigned by Go during tree resolution, maps to `nodeMap` on the bridge) and carries an `op` field indicating the patch type:

| Op | Payload | Description |
|----|---------|-------------|
| `redraw` | `tree_content` (JSON) | Replace the entire node with a new tree |
| `text` | `text` (string) | Update text node content |
| `facts` | `facts` (JSON) | Apply a FactsDiff — changed properties, attributes, styles, events |
| `append` | `tree_content` (JSON) | Append new children to the node |
| `remove-last` | `count` (int) | Remove N children from the end |
| `reorder` | `reorder` (JSON) | Keyed child insert/remove/move operations |
| `plugin` | `plugin_data` (JSON) | Updated data for a plugin node |
| `lazy` | `sub_patches` (nested) | Patches inside a lazy node's subtree |

**SERVER_JSCALL** — sent when a island calls `ExecJS()`. Carries `call_id` (unique request ID) and `expr` (JavaScript expression). The bridge evaluates the expression, `JSON.stringify`'s the result, and sends a `BROWSER_JSRESULT` back.

### Browser → Go: BrowserMessage

`BrowserMessage` is the single message type sent from the browser to Go. The `kind` field (a `BrowserKind` enum) determines which payload fields are relevant:

**BROWSER_INPUT** — input value sync (Layer 1). Sent automatically on every `input` event for elements with `g-bind`. Contains `node_id` and `value`. Updates the struct field without triggering a re-render.

**BROWSER_METHOD** — method call (Layer 2). Sent when the user triggers an event (click, keydown, etc.) or when JavaScript calls `godom.call()`. Contains `node_id` (0 for `godom.call`), `method`, and `args`. Calls the Go method via reflection and triggers a full re-render.

When `node_id` is 0 (from `godom.call`), the server searches all islands for one that has the named method and dispatches to the first match.

**BROWSER_JSRESULT** — response to a `SERVER_JSCALL`. Contains `call_id` (matching the request), `result` (JSON-encoded), and `error`. The server dispatches to the island that made the ExecJS call.

**BROWSER_INIT_REQUEST** — pull-based init. The bridge sends this with a `island` name to request the init tree for a named island. The server responds with a `SERVER_INIT` targeting that island. Used after `document.body` renders (root mode) or on `ws.onopen` (embedded mode) to initialize child islands discovered via `[g-island]` attributes. Also used by `godom.mount()` for dynamic mounting.

### Wire format

Both directions use raw protobuf — no tag bytes, no framing. Each WebSocket message is a single protobuf-encoded `ServerMessage` or `BrowserMessage`. The `kind` enum (integer on the wire) enables fast dispatch via switch statement.

**Key files:**

- `internal/proto/protocol.proto` — schema defining all message types
- `internal/proto/protocol.pb.go` — generated Go types via `protoc`
- `internal/proto/protocol.js` — JS type definitions via protobuf.js reflection API (no CLI codegen needed)
- `internal/proto/protobuf.min.js` — protobuf.js light build (~68KB), embedded into the binary
- `internal/server/server.go` — binary WebSocket read/write with `proto.Marshal`/`Unmarshal`
- `internal/render/encode.go` — builds protobuf `DomPatch` types from VDOM patches
- `internal/render/tree_encode.go` — encodes VDOM trees to JSON wire format
- `internal/bridge/bridge.js` — decodes `ServerMessage`, builds DOM from tree, applies patches, encodes `BrowserMessage`

**Design decisions:**

- **Tree-based init** — on connect, the bridge receives the entire tree as a JSON description and builds the DOM from scratch. This is simpler and more reliable than sending a sequence of individual commands
- **Patch-based updates** — after init, only minimal diffs are sent. The VDOM differ produces patches that map directly to DOM mutations
- **Stable node IDs** — every node gets a unique numeric ID from a monotonic counter that never resets. Patches reference the old tree's IDs because those are what the bridge has in `nodeMap`. New nodes in appends/redraws get fresh IDs
- **Facts as JSON** — the `FactsDiff` is JSON-encoded inside a protobuf `bytes` field. This keeps the protobuf schema simple while allowing arbitrary property/attribute/style changes
- **Plugin data stays JSON** — plugin data is JSON-encoded inside a `bytes` field (`plugin_data`). Plugin developers never see protobuf

## Transport

**WebSocket** is the transport for godom:

- Bidirectional, low-latency, tiny frame overhead (2-6 bytes per message)
- One persistent connection handles everything — DOM updates, events, plugin data
- Works everywhere — every browser, no TLS requirement (token auth handles access control)
- The solar system example proves it: 60fps rendering + mouse drag + scroll, all smooth on one connection

## Summary

| Layer | Status |
|-------|--------|
| Wire format | Protocol Buffers |
| VDOM pipeline | Tree init + patch updates |
| Control transport | WebSocket |

For future protocol extension ideas (streaming, media transport, WebTransport), see [planning/future-protocol-extensions.md](planning/future-protocol-extensions.md).
