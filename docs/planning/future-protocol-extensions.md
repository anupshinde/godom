# Future Protocol Extensions

Speculative designs and ideas for extending godom's wire protocol. None of this is implemented.

---

## Future message kinds

### ServerMessage

- **SERVER_STREAM** — append-only data that bypasses the VDOM pipeline. For streaming scenarios like AI chat token-by-token, log tailing, or video frames.
- **SERVER_BROADCAST** — cross-app messaging between independent godom instances.

### BrowserMessage

- **BROWSER_PAGE_INFO** — sends page path info to the server.
- **BROWSER_BROADCAST** — cross-app messaging from browser to Go.

---

## WebTransport

WebTransport would run alongside WebSocket, not replace it:

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

---

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

---

## Reducing WebSocket dependencies

We currently use `github.com/gorilla/websocket` for WebSocket and `google.golang.org/protobuf` for the binary wire format.

Options:

1. **`golang.org/x/net/websocket`** — already in our dependency tree for HTML parsing. Basic but sufficient for godom's needs (send/receive binary messages). The most pragmatic path to drop gorilla.

2. **Wait for stdlib WebSocket** — Go may add native WebSocket support in a future release. Monitor this.

3. **SSE + POST** — eliminates all WebSocket code. Viable if we add client-side throttling for high-frequency events. The overhead concern is real but small on localhost.

See [../transport.md](../transport.md) for the full analysis of WebSocket vs SSE + POST.
