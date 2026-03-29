# Performance & Transport

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

### Go → Browser: VDomMessage

`VDomMessage` is the top-level message sent from Go to the browser. It has two modes:

- **init** (`type: "init"`): carries a `tree` field — the entire VDOM tree encoded as JSON bytes. The bridge builds the DOM from this description on first connect or reconnect.
- **patch** (`type: "patch"`): carries a list of `DomPatch` messages — minimal mutations computed by diffing the old and new VDOM trees.

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

### Browser → Go: Tagged binary messages

The browser sends binary messages with a one-byte tag prefix:

**Tag 0x01 — NodeEvent (Layer 1: input sync)**

Sent automatically on every `input` event for elements with `g-bind` (and unbound inputs). Contains:
- `node_id` (int32) — stable node ID of the input element
- `value` (string) — current DOM value (e.g., `input.value`)

Layer 1 updates the struct field without triggering a re-render. This keeps Go in sync with user typing cheaply.

**Tag 0x02 — MethodCall (Layer 2: event dispatch)**

Sent when the user triggers an event (click, keydown, mousedown, drop, etc.). Contains:
- `node_id` (int32) — stable node ID of the element that fired
- `method` (string) — Go method name (e.g., `"AddTodo"`, `"Toggle"`)
- `args` (repeated bytes) — JSON-encoded arguments

Layer 2 calls the method via reflection and triggers a full re-render (tree resolution, diff, broadcast patches).

The bridge constructs these messages directly from event data and the event handler information embedded in the VDOM tree's facts. See [architecture.md — Browser → Go: two layers](architecture.md#browser--go-two-layers) for the design rationale.

**Key files:**

- `internal/proto/protocol.proto` — schema defining all message types
- `internal/proto/protocol.pb.go` — generated Go types via `protoc`
- `internal/proto/protocol.js` — JS type definitions via protobuf.js reflection API (no CLI codegen needed)
- `internal/proto/protobuf.min.js` — protobuf.js light build (~68KB), embedded into the binary
- `internal/server/server.go` — binary WebSocket read/write with `proto.Marshal`/`Unmarshal`
- `internal/render/encode.go` — builds protobuf `DomPatch` types from VDOM patches
- `internal/render/tree_encode.go` — encodes VDOM trees to JSON wire format
- `internal/bridge/bridge.js` — decodes `VDomMessage`, builds DOM from tree, applies patches, encodes `NodeEvent`/`MethodCall`

**Design decisions:**

- **Tree-based init** — on connect, the bridge receives the entire tree as a JSON description and builds the DOM from scratch. This is simpler and more reliable than sending a sequence of individual commands
- **Patch-based updates** — after init, only minimal diffs are sent. The VDOM differ produces patches that map directly to DOM mutations
- **Stable node IDs** — every node gets a unique numeric ID from a monotonic counter that never resets. Patches reference the old tree's IDs because those are what the bridge has in `nodeMap`. New nodes in appends/redraws get fresh IDs
- **Facts as JSON** — the `FactsDiff` is JSON-encoded inside a protobuf `bytes` field. This keeps the protobuf schema simple while allowing arbitrary property/attribute/style changes
- **Plugin data stays JSON** — plugin data is JSON-encoded inside a `bytes` field (`plugin_data`). Plugin developers never see protobuf

## Transport: WebSocket today, WebTransport parked for future

**WebSocket** is the transport for godom:

- Bidirectional, low-latency, tiny frame overhead (2-6 bytes per message)
- One persistent connection handles everything — DOM updates, events, plugin data
- Works everywhere — every browser, no TLS requirement (token auth handles access control)
- The solar system example proves it: 60fps rendering + mouse drag + scroll, all smooth on one connection

**WebTransport** is parked for the future. It would run alongside WebSocket, not replace it:

- **WebSocket** — control messages (click, keydown, DOM updates). Reliable, ordered, guaranteed delivery
- **WebTransport** — high-frequency or lossy-tolerant data (mouse tracking, video frames). Supports unreliable datagrams where dropping a stale frame is better than queuing it

**When WebTransport becomes relevant:**

- Network/remote transport (not local) where congestion control matters
- Use cases where dropping stale data is preferable to delivering it late
- Multiple independent streams without head-of-line blocking

**Not implementing now because:**

- Go's WebTransport server support is experimental (`quic-go/webtransport-go`)
- Requires HTTP/3 and TLS certificates even locally
- Browser support is Chrome/Edge only, limited Firefox, no Safari
- No current godom use case that WebSocket can't handle

## Heavy media workloads (video, audio, binary streaming)

A media-heavy app (video editor, live preview, audio processing) would have two separate data flows:

**Control plane** (light) — timeline scrubbing, cut points, effect parameters, UI state. Small protobuf messages on the main WebSocket. This is what godom handles today.

**Media plane** (heavy) — preview frames, waveforms, thumbnails. Bulk binary data that should not share the control channel.

### Frame sizes

A single 1080p video frame:

| Format | Size per frame | At 30fps |
|--------|---------------|----------|
| Raw RGBA | ~8MB | 240MB/s — impossible over WebSocket |
| JPEG | 50-200KB | 3-6MB/s — feasible locally, tight over network |
| WebP | 30-150KB | 1-4MB/s — better, still heavy |
| H.264 chunk | 10-50KB | 0.3-1.5MB/s — best compression, hardware decode in browser |

### Architecture: separate the media channel

The key principle: **keep heavy media data off the main WebSocket so it doesn't block UI updates.**

The best approach for local godom is a **dedicated binary WebSocket** — a second WebSocket connection for bulk data. It works everywhere today, needs no TLS (token auth provides access control), and Go + browser both support it fully. For network transport in the future, this could be upgraded to WebTransport datagrams.

### What godom would need

A streaming API to open a dedicated binary channel:

```go
app.Stream("preview", func(w io.Writer) {
    // Go encodes and writes frames here
    // each Write() sends a binary WebSocket frame to the browser
})
```

On the JS side, the plugin system already provides the hook. A video plugin would subscribe to the named stream for frame data while control messages stay on the main WebSocket:

```js
godom.register("video-preview", {
    init: function(el, data) {
        // godom opens a binary WebSocket for the "preview" stream
        // plugin receives frames and draws to canvas
    }
});
```

### Rendering options on the browser side

Once frames arrive in the browser:

- **Canvas 2D** — decode JPEG/WebP with `createImageBitmap()`, draw to canvas. Simple, works everywhere
- **HTTP streaming** — MJPEG-style long-lived HTTP response. Works with `<img>` tags. One-directional, very simple
- **MediaSource Extensions (MSE)** — feed H.264/VP9 chunks into a `<video>` element. Best compression, hardware-accelerated decode. But encoding in Go is CPU-heavy (needs CGo + FFmpeg)
- **WebRTC** — Go as a WebRTC peer, hardware decode on browser. Lowest latency, most complex setup

For a godom video editor, Canvas 2D with JPEG frames over a binary WebSocket is the pragmatic starting point. MSE or WebRTC would be optimizations if compression or latency becomes a bottleneck.

## Summary

| Layer | Status | Future |
|-------|--------|--------|
| Wire format | **Protocol Buffers** (done) | — |
| VDOM pipeline | **Tree init + patch updates** (done) | — |
| Control transport | WebSocket | WebSocket |
| Media transport | — | Binary WebSocket (`app.Stream`), then WebTransport datagrams |
| Inter-app messaging | — | External broker (NATS etc.), not godom's job |
