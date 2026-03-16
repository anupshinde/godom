# godom — Video Editor as a Stress Test

## The idea

Build a basic video editing app in godom — not to compete with real editors, but as a deliberate stress test that pushes godom to its limits on binary throughput, latency, and heavy DOM updates. The goal is to find performance ceilings and bottlenecks, and prove (or disprove) that godom can handle media-heavy workloads.

## Why a video editor

A video editor hits every hard problem at once:

- **Large binary transfers** — video frames, thumbnails, waveforms, preview streams
- **Latency sensitivity** — scrubbing a timeline needs instant feedback, not 200ms round-trips
- **Heavy DOM updates** — timeline UI with many clips, tracks, keyframes, all updating as you scrub
- **Continuous rendering** — preview playback is a sustained stream, not one-off state pushes
- **Mixed workload** — light UI interactions (button clicks, drag handles) competing with heavy data (frame decoding, audio waveforms) on the same connection

If godom can handle a video editor at any usable level, it can handle basically anything.

## What the app would look like

### MVP scope (stress test, not a real editor)

**Timeline panel**
- A horizontal track with clip thumbnails
- Drag a playhead to scrub through the video
- Click to set in/out points for a cut
- Multiple tracks (video + audio) displayed vertically

**Preview panel**
- Shows the current frame at the playhead position
- Play/pause for real-time preview playback
- Frame-by-frame stepping (arrow keys)

**Properties panel**
- Basic metadata: resolution, duration, frame rate, codec
- Simple trim controls: start time, end time

**Operations**
- Import a video file (read from disk in Go)
- Trim: cut a clip at the playhead
- Export: write the trimmed result to disk (FFmpeg in Go, no browser involvement)

### What it deliberately skips

- Effects, transitions, filters, color grading — not the point
- Multi-layer compositing — one or two tracks is enough to stress test
- Audio mixing — waveform display is the stress test, mixing is out of scope
- Undo/redo — nice to have but not a performance test

## Architecture: two-channel design

This follows the architecture already outlined in [protocol.md](../protocol.md) — separate control and media planes.

### Control plane (main WebSocket)

Everything that is normal godom — the UI state:

- Timeline position (playhead, zoom level, scroll offset)
- Clip metadata (start, end, track, name)
- UI state (selected clip, active panel, modal dialogs)
- User actions (click, drag, keydown)

Small protobuf messages, same as any godom app. This is the part that already works.

### Media plane (dedicated binary WebSocket)

The heavy data that must not block UI updates:

- **Preview frames** — when scrubbing or playing, Go decodes the video frame and sends it as JPEG/WebP over a second WebSocket. The browser draws it to a `<canvas>`.
- **Thumbnails** — on import, Go extracts thumbnails at regular intervals and streams them to the browser for the timeline strip. This is a batch operation, not real-time.
- **Audio waveform** — Go computes waveform data (peak amplitudes per time slice) and sends it as a compact binary array. Browser renders it on a canvas.

This uses the `app.Stream()` API concept from the protocol doc:

```go
app.Stream("preview", func(w io.Writer) {
    // Go decodes frames and writes JPEG bytes
    // each Write() sends one binary WebSocket frame
})
```

### Frame pipeline

When the user scrubs the timeline:

1. Browser sends `scrub` event with timestamp via control WebSocket (protobuf, fast)
2. Go receives the timestamp, seeks in the video file
3. Go decodes the frame at that timestamp (FFmpeg via CGo, or pure Go for simpler formats)
4. Go encodes the frame as JPEG (Go's `image/jpeg` package, ~2-5ms for 720p)
5. Go sends the JPEG bytes over the media WebSocket
6. Browser receives bytes, creates `ImageBitmap`, draws to canvas

**Target latency:** Scrub-to-frame should feel responsive at <100ms. On localhost, the WebSocket round-trip is ~1ms, so the bottleneck is decode + encode time in Go.

**Target throughput for playback:** 30fps at 720p with JPEG encoding means ~3-6MB/s. On localhost this is trivial (no network bottleneck). The question is whether Go can decode + encode fast enough to sustain it.

## Technology choices

### Video decoding in Go

This is the hardest part. Options:

| Approach | Pros | Cons |
|----------|------|------|
| **FFmpeg via CGo** (`github.com/giorgisio/goav` or raw CGo bindings) | Full codec support, hardware acceleration, battle-tested | CGo complexity, cross-compilation pain, not a single binary anymore |
| **FFmpeg as subprocess** (`exec.Command("ffmpeg", ...)`) | No CGo, single godom binary, FFmpeg installed separately | Process spawn overhead per frame, but can keep a pipe open |
| **Pure Go decoders** (VP8 via `golang.org/x/image/vp8`, GIF, etc.) | No dependencies, single binary | Very limited codec support, no H.264/H.265 |

**Pragmatic starting point:** FFmpeg as a subprocess with a persistent pipe. Go sends seek commands, FFmpeg outputs raw frames, Go encodes to JPEG and streams to browser. This keeps the godom binary clean and leverages FFmpeg's full codec support.

### Thumbnail generation

On import, spawn FFmpeg once to extract thumbnails at regular intervals:

```
ffmpeg -i input.mp4 -vf "fps=1,scale=160:-1" -f image2pipe -vcodec mjpeg pipe:1
```

Go reads the JPEG stream and sends thumbnails to the browser as they arrive. The timeline populates progressively.

### Audio waveform

FFmpeg can output raw PCM audio. Go reads the samples, computes peak values per time window (e.g., one peak per 10ms), and sends a compact float array to the browser. The browser renders peaks as vertical bars on a canvas — a standard waveform visualization.

## What this tests in godom

| Aspect | What we learn |
|--------|---------------|
| **Binary WebSocket throughput** | Can the second WebSocket sustain 3-6MB/s of JPEG frames for 30fps playback? |
| **Control + media isolation** | Do UI interactions (clicking, dragging) stay responsive while frames are streaming? |
| **DOM update frequency** | Timeline with 50+ thumbnail elements updating during scroll — does diffing keep up? |
| **Plugin system under load** | Canvas plugin receiving frames at 30fps — does the init/update cycle hold? |
| **State management** | Complex nested state (tracks → clips → keyframes) with frequent partial updates |
| **Memory pressure** | Holding decoded frames, thumbnail caches, waveform data in Go — GC pauses? |
| **Protobuf message size** | Large numbers of small control messages interleaved with normal UI events |

## Expected outcomes

**Likely to work well:**
- Control plane UI (timeline, buttons, panels) — this is normal godom territory
- Thumbnail generation and display — batch operation, not real-time
- Frame-by-frame stepping — one frame at a time, plenty of budget per frame
- Audio waveform — computed once, sent once, rendered on canvas

**Likely to hit limits:**
- Real-time 30fps preview playback — depends on Go's JPEG encode speed and WebSocket throughput. 720p might work, 1080p might not sustain 30fps
- Scrubbing latency — if FFmpeg seek is slow (keyframe distance), scrubbing will feel laggy
- Memory — holding many decoded frames in Go for fast scrubbing could pressure the GC

**The point is to find these limits**, measure them, and understand what godom needs to improve (or what developers need to work around) for media-heavy apps.

## Relation to existing godom capabilities

- **Protocol doc already covers this** — [protocol.md](../protocol.md) has a detailed section on heavy media workloads, frame sizes, the two-channel architecture, and rendering options. This idea is the concrete app that would exercise that design.
- **Plugin system** — the canvas-based video preview would be a godom plugin, same pattern as Chart.js but for frame rendering.
- **Solar system demo** — already proves godom can push continuous high-frequency updates to the browser. The video editor pushes the same pattern but with much larger payloads (frames vs. planet positions).

## Status

Idea stage. High effort, high value as a stress test. Not planned for immediate implementation — best attempted after the streaming API (`app.Stream()`) and a second binary WebSocket channel are available in godom.
