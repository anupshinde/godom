# Performance & Transport

## Wire format: JSON → Protocol Buffers

The bridge currently uses JSON for all WebSocket communication. This works but is not the right long-term choice.

**Why we're switching to protobuf:**

- **Binary format** — smaller wire size, faster encode/decode than JSON
- **Schema as contract** — the `.proto` file defines the protocol formally. No guessing message formats from source code
- **Multi-language support** — any language (Python, Rust, Java, etc.) can generate a client from the `.proto` file and talk to a godom app
- **No user-facing impact** — users write Go structs and HTML directives. They never see the wire format. All examples continue to work unchanged
- **Future-proof** — protobuf handles schema evolution cleanly. Adding fields is backward compatible

**Alternatives considered and rejected:**

| Option | Why not |
|--------|---------|
| FlatBuffers | Zero-copy advantage only matters for large messages where you read a few fields. godom reads every field in every message — no benefit over protobuf |
| Custom binary protocol | Protobuf already is a binary protocol, but with codegen, schemas, and multi-language support. No reason to hand-roll one |
| String concatenation | A hack to avoid JSON cloning overhead. Irrelevant once we're on protobuf |

**What changes internally:**

- `godom.go` — `ReadJSON`/`WriteJSON` → binary WebSocket read/write + `proto.Marshal`/`Unmarshal`
- `render.go` — `json.Marshal` → `proto.Marshal` for commands and events
- `bridge.js` — `JSON.parse`/`JSON.stringify` → protobuf encode/decode (using embedded `protobuf.js`)
- New `protocol.proto` schema file and `make proto` codegen step

**What doesn't change:** all examples, all HTML templates, all user-facing Go APIs, parser, validator, plugin JS files.

## Transport: WebSocket today, WebTransport future

**WebSocket** is the right transport for godom today:

- Bidirectional, low-latency, tiny frame overhead (2-6 bytes per message)
- One persistent connection handles everything — DOM updates, events, plugin data
- Works everywhere — every browser, no TLS requirement locally
- The solar system example proves it: 60fps rendering + mouse drag + scroll, all smooth on one connection

**WebTransport** is a future addition, not a replacement. It would run alongside WebSocket:

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

The best approach for local godom is a **dedicated binary WebSocket** — a second WebSocket connection for bulk data. It works everywhere today, needs no TLS, and Go + browser both support it fully. For network transport in the future, this could be upgraded to WebTransport datagrams.

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

| Layer | Current | Planned | Future |
|-------|---------|---------|--------|
| Wire format | JSON | **Protocol Buffers** | — |
| Control transport | WebSocket | WebSocket | WebSocket |
| Media transport | — | Binary WebSocket (`app.Stream`) | WebTransport datagrams |
| Inter-app messaging | — | — | External broker (NATS etc.), not godom's job |
