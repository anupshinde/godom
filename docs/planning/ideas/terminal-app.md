# godom — Terminal App

## The idea

A web-based terminal that runs on your machine via godom. Open a browser tab, get a full terminal session — your actual shell, your dotfiles, your tools. Go allocates a PTY, pipes I/O over the WebSocket, xterm.js renders it in the browser.

This feels like the most immediately buildable idea on the list. The pieces already exist — PTY libraries in Go, xterm.js for rendering, godom's binary WebSocket for transport. It's a matter of wiring them together.

## Why this matters: access your terminal from anywhere

On its own, a terminal in a browser tab on the same machine is a curiosity. But combine it with something like Tailscale (or any mesh VPN / overlay network), and it becomes genuinely useful:

- Run the godom terminal app on your home machine or a server
- Tailscale puts it on your private network, accessible from any of your devices
- Open the browser on your phone, tablet, or another laptop — you have a terminal session on your machine
- No SSH client needed. No port forwarding. No public IP. Just a browser and a Tailscale connection.

This is "access your terminal from anywhere" with zero configuration beyond what Tailscale already provides. godom's existing LAN features (IP resolution, QR code) make the local case easy too — scan a QR code on your phone, get a terminal on your desktop.

## How it works

### Go side

1. **PTY allocation** — Use `creack/pty` (or `os/exec` + raw PTY syscalls) to spawn a shell process (`$SHELL` or `/bin/bash`) with a pseudo-terminal
2. **Read loop** — Goroutine reads PTY output and sends it as binary frames over the WebSocket
3. **Write loop** — Receives keystrokes from the browser via WebSocket events, writes them to the PTY input
4. **Resize handling** — Browser reports terminal dimensions (cols × rows), Go calls `pty.Setsize()` to update the PTY — so `vim`, `htop`, and TUI apps render correctly

```go
type Terminal struct {
    // godom component state
    Connected bool
    Cols      int
    Rows      int
}
```

The terminal component state is minimal — almost everything flows through the raw binary stream, not through godom's state diffing. The PTY output is opaque bytes (ANSI escape sequences), not structured data.

### Browser side (xterm.js plugin)

xterm.js is a mature, fast terminal emulator for the browser. It handles:

- ANSI escape sequence parsing and rendering
- Cursor positioning, colors, text attributes
- Scrollback buffer
- Selection and copy/paste
- Mouse events (for TUI apps that use mouse)
- Fit addon (auto-resize to container)

This would be a godom plugin, same pattern as Chart.js:

```go
import "github.com/anthropics/godom/plugins/xterm"

func main() {
    eng := godom.NewEngine()
    xterm.Register(eng) // registers plugin, embeds xterm.js
    eng.Mount(&Terminal{Cols: 80, Rows: 24})
    eng.Run()
}
```

In the HTML:
```html
<div g-plugin:xterm="Terminal"></div>
```

The plugin JS:
- On init: create an `xterm.Terminal` instance, attach to the element
- Receives binary data from Go (PTY output) → writes to xterm.js terminal
- Captures keystrokes from xterm.js → sends back to Go via WebSocket
- Reports resize events → Go adjusts PTY size

### Data flow

```
Keyboard → xterm.js → WebSocket → Go → PTY stdin → shell process
shell process → PTY stdout → Go → WebSocket → xterm.js → screen
```

Every keystroke round-trips through Go. On localhost this is imperceptible (<1ms). Over Tailscale it depends on the network path, but Tailscale's WireGuard tunnels are typically low-latency enough for interactive terminal use.

## What makes this a natural fit for godom

- **Go does the real work** — PTY allocation, process management, I/O piping. This is Go's home turf. The browser is purely a rendering surface.
- **Binary WebSocket is already there** — terminal I/O is raw bytes (ANSI sequences). godom's protobuf WebSocket handles binary natively. No encoding overhead.
- **Plugin system handles xterm.js** — same registration pattern as Chart.js. Embed the library, write a thin bridge adapter, done.
- **State survives browser close** — the shell process lives in the Go process, not the browser. Close the tab, reopen it, reconnect to the same session. This is a significant advantage over browser-only terminal emulators.
- **Multi-connection works** — godom already broadcasts to all connected browsers. Multiple tabs or devices could view the same terminal session (useful for pair programming or demonstrations).
- **Single binary** — `go build` produces one binary that embeds xterm.js, the bridge, and everything else. Copy it to a machine, run it, open a browser.

## Tailscale integration specifically

godom already binds to `0.0.0.0` (configurable) and supports token-based auth. On a Tailscale network:

- The godom terminal app binds to the Tailscale interface (or `0.0.0.0`)
- Tailscale handles authentication and encryption at the network layer
- godom's token auth adds an application-level access control layer on top
- The QR code feature could show the Tailscale IP URL for easy mobile access

No code changes needed in godom for this — it works today because Tailscale is transparent at the network level. The terminal app just needs to exist.

## Scope for a first version

### Must have
- Spawn a shell, render in xterm.js, handle keystrokes
- PTY resize when browser window changes
- Reconnect to existing session on page reload
- Token auth (already in godom) so the terminal isn't open to the network

### Nice to have
- Multiple terminal tabs (multiple PTY sessions)
- Scrollback buffer persistence in Go (survive reconnect without losing history)
- Copy/paste integration
- Custom shell command (run a specific command instead of default shell)
- Read-only mode for viewers (multi-connection with one writer, many readers)

### Out of scope
- File upload/download (scp/sftp exist)
- Split panes (tmux exists)
- Session recording/replay

## Status — implemented

Working example at `examples/terminal/`. See `examples/terminal/README.md` for usage and `examples/terminal/implementation.md` for the full architectural deep-dive.

### What was built
- PTY allocation with `creack/pty`, shell respawn on exit
- xterm.js 4.19.0 via godom plugin system for rendering
- Separate WebSocket for raw PTY I/O (godom's plugin system is one-way, so terminal data flows through a dedicated connection)
- Resize handling via `pty.Setsize()`
- Token auth on both godom and terminal WebSocket
- Multi-browser support (multiple tabs see the same session)
- Session survives browser close/reopen

### What diverged from the original idea
- **Not a godom plugin package** — the terminal is a standalone example with its own `go.mod`, not a reusable `plugins/xterm/` package. The bidirectional streaming requirement doesn't fit the plugin API cleanly enough for a generic package yet.
- **Separate WebSocket for terminal I/O** — the idea assumed godom's binary WebSocket would carry terminal data. In practice, the plugin system is Go→browser only (no browser→Go path), and piping opaque byte streams through protobuf state diffing adds overhead for no benefit. A dedicated WebSocket for raw I/O is simpler and faster.
- **xterm.js loaded from CDN** — not embedded yet. For true single-binary use, the JS/CSS would need to be embedded via `go:embed`.
