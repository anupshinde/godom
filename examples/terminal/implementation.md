# Terminal Example — Implementation Details

A browser-based terminal that gives you shell access to the machine running the Go app. You run `go run .`, a browser tab opens, and you're in a full terminal session — your shell, your dotfiles, your tools.

## The problem

godom's communication model is designed for structured UI state: Go fields get diffed, serialized to protobuf, and sent as concrete DOM commands. This works perfectly for text, numbers, booleans, lists, and even chart data via plugins.

A terminal is fundamentally different. It's a **raw byte stream** — the shell produces ANSI escape sequences (cursor movement, colors, clearing) and expects keystrokes back as raw bytes. There's no structured state to diff. The data is opaque, high-frequency, and bidirectional.

godom's plugin system (`g-plugin:name="Field"`) is one-way: Go pushes data to the browser via `init(el, data)` and `update(el, data)`. The browser has no plugin-level mechanism to send data back to Go. The existing event system (`g-click`, `g-keydown`, `g-bind`) passes through godom's protobuf envelope protocol, which adds encoding overhead and doesn't support arbitrary binary payloads.

## Architecture decision: two connections

This example uses **two separate connections**:

1. **godom's connection** — serves the HTML page, handles auth (token + cookie), injects scripts, manages the plugin lifecycle. This is the standard godom path.

2. **A dedicated terminal WebSocket** — a second, lightweight WebSocket server on a separate port that handles only raw PTY I/O. No protobuf, no state diffing, no DOM commands. Just bytes in, bytes out.

The godom plugin acts as the bridge: when the plugin initializes, it receives the terminal WebSocket's port and auth token via the plugin data, and opens the second connection from the browser.

```
┌─────────────────────────────────────────────────────────────────┐
│                         Go Process                              │
│                                                                 │
│  ┌──────────────┐         ┌──────────────────────────────────┐  │
│  │  godom server │         │  terminal WebSocket server       │  │
│  │  (port A)     │         │  (port B, random)                │  │
│  │               │         │                                  │  │
│  │  - serves HTML│         │  - raw binary I/O                │  │
│  │  - protobuf   │         │  - no protobuf                   │  │
│  │  - plugin data│         │  - token auth via query param    │  │
│  │  - auth/QR    │         │                                  │  │
│  └──────┬───────┘         └──────────┬───────────────────────┘  │
│         │                            │                          │
│         │ plugin init:               │ raw bytes:               │
│         │ {wsPort, token}            │ PTY stdout ──► browser   │
│         │                            │ browser ──► PTY stdin    │
│         │                            │ resize JSON ──► Setsize  │
│         │                            │                          │
│         │                     ┌──────┴──────┐                   │
│         │                     │  PTY (shell) │                   │
│         │                     │  /bin/zsh    │                   │
│         │                     └─────────────┘                   │
└─────────┼────────────────────────────┼──────────────────────────┘
          │                            │
          │                            │
┌─────────┴────────────────────────────┴──────────────────────────┐
│                          Browser                                │
│                                                                 │
│  godom bridge.js ◄── protobuf ──► godom WS (port A)            │
│       │                                                         │
│       │ plugin init()                                           │
│       ▼                                                         │
│  xterm-adapter.js                                               │
│       │                                                         │
│       ├── creates xterm.js Terminal instance                    │
│       ├── opens WebSocket to port B                             │
│       ├── pipes xterm.js onData ──► WS (keystrokes)            │
│       ├── pipes WS onmessage ──► xterm.js write (PTY output)   │
│       └── sends resize JSON on window/terminal resize           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Why not pipe terminal data through godom's protocol?

Several reasons:

1. **No browser-to-Go path in plugins.** The plugin API is `{init, update}` — both are Go-to-browser. There's no `send()` or callback mechanism for the plugin to push data back to Go.

2. **State diffing overhead.** godom diffs JSON snapshots of component fields to detect changes. Terminal output is a continuous stream of opaque bytes — diffing it against a previous snapshot is meaningless and wasteful.

3. **Protobuf encoding overhead.** Each godom message is a `ServerMessage` containing `Command` objects with typed values. Wrapping raw terminal bytes in this structure adds serialization cost for no benefit.

4. **Latency sensitivity.** Every keystroke round-trips through the terminal. Adding protobuf encode/decode and state-diff computation to this path would increase latency. On localhost it might be imperceptible, but over a network (e.g., via Tailscale) it matters.

5. **Transport independence.** godom may change its transport in the future (away from WebSockets). The terminal's raw byte stream has different requirements than UI state updates. Keeping them separate means the terminal can use whatever transport suits it best without depending on godom's internal transport choices.

### Why not a hidden input with g-bind?

An alternative considered was using `g-bind="TermInput"` on a hidden `<input>` element, where the xterm.js adapter would set its value and dispatch an `input` event to send keystrokes back through godom's binding mechanism. This was rejected because:

- `g-bind` calls `setField()` via reflection — it sets the Go struct field but doesn't trigger any side effect (like writing to PTY). There's no hook to say "when this field changes, do X."
- Terminal input includes escape sequences (arrow keys produce `\x1b[A`, etc.) that may not survive JSON string encoding cleanly.
- The round-trip path is longer: xterm.js → hidden input → bridge.js → protobuf → godom → field set → ??? → PTY, versus the direct path: xterm.js → terminal WS → PTY write.

## File-by-file walkthrough

### main.go

The entry point. Does three things in sequence:

1. **Generates a shared auth token.** This token is used by both the godom page (via plugin data) and the terminal WebSocket (via query parameter). It's generated here rather than relying on godom's internal token because godom's token is generated inside `Start()` and isn't exposed.

2. **Starts the terminal WebSocket server.** Calls `startTerminalServer(token)` which spawns the shell, allocates the PTY, and begins listening on a random port. The port number is returned so it can be passed to the browser.

3. **Sets up godom.** Registers the xterm plugin adapter, creates the root component with the terminal config (port + token), mounts it, and starts serving.

```go
type App struct {
    godom.Component
    Terminal TerminalConfig
}

type TerminalConfig struct {
    WSPort int    `json:"wsPort"`
    Token  string `json:"token"`
}
```

`TerminalConfig` is the struct that gets serialized to JSON and passed to the xterm plugin's `init(el, data)` function. The plugin uses `data.wsPort` and `data.token` to connect to the terminal WebSocket.

The `//go:embed xterm-adapter.js` directive embeds the plugin adapter JS at compile time. It's passed to `app.Plugin("xterm", xtermAdapterJS)` which injects it as a `<script>` tag before godom's bridge.js.

### terminal.go

The PTY and terminal WebSocket server. This is the core of the implementation.

#### The termSession struct

```go
type termSession struct {
    shell string
    mu    sync.Mutex
    ptmx  *os.File
    conns []*termConn
}
```

`termSession` manages the lifecycle of a PTY and all WebSocket clients connected to it. The key design decision: **when the shell exits, `termSession` automatically spawns a new one.** The browser connections stay alive — the user sees a message like:

```
[shell exited — this is a browser-based terminal powered by godom, starting new session...]
[to exit the terminal, close this tab or terminate the godom process]
```

...followed by a fresh shell prompt. This happens because the broadcast goroutine detects the PTY read error (shell exited), sends the notification to all connected browsers, closes the old PTY file descriptor, and calls `ts.spawn()` recursively to start a new shell. The PTY file pointer (`ts.ptmx`) is swapped under the mutex so concurrent `writeToPTY()` and `resize()` calls from WebSocket handlers always use the current PTY.

This matters because typing `exit` in a browser-based terminal is a natural reflex but shouldn't kill the whole application. The godom process is the host — the shell sessions are guests.

#### Shell spawning

```go
cmd := exec.Command(ts.shell)
cmd.Env = append(os.Environ(), "TERM=xterm-256color")

ptmx, err := pty.Start(cmd)
```

- Uses `$SHELL` so you get your preferred shell (zsh, fish, bash, etc.)
- Sets `TERM=xterm-256color` so the shell and programs running inside it know the terminal supports 256 colors and standard xterm escape sequences. Without this, programs like `vim`, `htop`, and `ls --color` won't render correctly.
- `pty.Start()` from `github.com/creack/pty` does the heavy lifting: creates a pseudo-terminal pair (master/slave), connects the slave to the command's stdin/stdout/stderr, and starts the process. Returns the master side (`ptmx`) which we read from and write to.

#### PTY output broadcast

```go
go func() {
    buf := make([]byte, 4096)
    for {
        n, err := ptmx.Read(buf)
        if err != nil {
            // Shell exited. Notify browsers and respawn.
            msg := []byte("...[shell exited message]...")
            ts.mu.Lock()
            for _, tc := range ts.conns {
                tc.writeBinary(msg)
            }
            ts.mu.Unlock()

            ptmx.Close()
            ts.spawn()
            return
        }

        data := make([]byte, n)
        copy(data, buf[:n])

        ts.mu.Lock()
        for _, tc := range ts.conns {
            tc.writeBinary(data)
        }
        ts.mu.Unlock()
    }
}()
```

A single goroutine sits in a read loop on the PTY master. Every time the shell produces output (command results, prompts, escape sequences), it's read into a buffer and broadcast to all connected WebSocket clients as binary frames.

The `copy(data, buf[:n])` is necessary because the buffer is reused across iterations. Without the copy, a slow WebSocket write could see the buffer overwritten by the next PTY read.

The 4096-byte buffer is a pragmatic choice — large enough to batch typical output (a prompt line, an `ls` listing) but small enough that output appears without noticeable delay.

When the PTY read returns an error, the shell has exited. Rather than disconnecting browsers, the goroutine notifies them with a styled message explaining what happened and that a new session is starting, then respawns the shell. The old goroutine returns and a new one takes its place via `ts.spawn()`.

#### Connection management

```go
type termConn struct {
    conn *websocket.Conn
    mu   sync.Mutex
}
```

Each WebSocket connection gets a write mutex. This is required because `gorilla/websocket` does not support concurrent writes — if the PTY broadcast goroutine and the resize response tried to write at the same time, the connection would corrupt. The read side doesn't need a mutex because only one goroutine reads from each connection.

Multiple browsers can connect simultaneously. They all see the same terminal session (the same PTY). This enables use cases like pair programming or screen sharing — one person types, everyone sees the output.

#### Message protocol on the terminal WebSocket

The terminal WebSocket uses a simple convention based on WebSocket message types:

- **Binary messages** = raw terminal data (both directions)
  - Browser → Go: keystrokes, encoded as UTF-8 bytes via `TextEncoder`
  - Go → Browser: PTY output, raw bytes (ANSI escape sequences, text, etc.)

- **Text messages** = control commands (browser → Go only)
  - Currently only resize: `{"cols": 80, "rows": 24}`
  - Parsed as JSON, used to call `pty.Setsize()` which updates the PTY's window size

This avoids the need for any framing protocol or message type headers. The WebSocket message type itself distinguishes data from control messages.

#### Resize handling

```go
if msgType == websocket.TextMessage {
    var resize struct {
        Cols int `json:"cols"`
        Rows int `json:"rows"`
    }
    if json.Unmarshal(data, &resize) == nil && resize.Cols > 0 && resize.Rows > 0 {
        pty.Setsize(ptmx, &pty.Winsize{
            Cols: uint16(resize.Cols),
            Rows: uint16(resize.Rows),
        })
    }
}
```

When the browser window resizes, xterm.js recalculates how many columns and rows fit. The fit addon handles this automatically. The new dimensions are sent to Go, which calls `pty.Setsize()` to update the PTY's window size. This sends a `SIGWINCH` signal to the shell process, which causes it (and any TUI programs running inside it) to redraw at the new size.

Without resize handling, programs like `vim`, `htop`, and `less` would render at the initial size regardless of browser window changes, causing visual corruption.

#### Auth

```go
if r.URL.Query().Get("token") != authToken {
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
    return
}
```

The terminal WebSocket validates a token passed as a query parameter. This is the same token passed to the browser via godom's plugin data. Without this, anyone who knows the port could connect to the terminal WebSocket directly — a significant security risk since it provides shell access to the machine.

### xterm-adapter.js

The godom plugin adapter. Registered via `godom.register("xterm", {init, update})`.

#### Plugin init

When godom's bridge receives the first `ServerMessage` with a plugin command for the xterm element, it calls `init(el, data)` where:
- `el` is the DOM element with `g-plugin:xterm="Terminal"`
- `data` is the JSON-parsed value of the Go `Terminal` field (the `TerminalConfig` struct)

The init function:

1. **Creates an xterm.js Terminal instance** with visual configuration (font, colors, cursor blink). The theme colors match the page background (`#1a1a2e`) so the terminal blends seamlessly.

2. **Loads the fit addon** which auto-sizes the terminal to fill its container element. `fitAddon.fit()` is called immediately after opening, and again on every window resize.

3. **Opens the terminal WebSocket** using the port and token from the plugin data:
   ```js
   var wsUrl = "ws://" + location.hostname + ":" + data.wsPort
             + "/terminal?token=" + encodeURIComponent(data.token);
   ```
   Uses `location.hostname` (not `location.host`) so it connects to the same machine but on the terminal's port.

4. **Wires up the data flow:**
   - `ws.onmessage` → `term.write()`: PTY output appears on screen
   - `term.onData` → `ws.send()`: keystrokes go to the PTY
   - `term.onResize` → `ws.send(JSON.stringify({cols, rows}))`: size changes go to the PTY
   - `window resize` → `fitAddon.fit()`: recalculates terminal dimensions

5. **Stores references** on the element (`el.__term`, `el.__ws`, `el.__fitAddon`) for potential future cleanup or debugging.

#### Plugin update

The `update()` function is empty. Terminal configuration (port, token) doesn't change after init. The actual terminal data flows through the separate WebSocket, not through godom's state update mechanism.

### ui/index.html

A minimal full-screen page. Key points:

- **Embedded xterm.js** (v4.19.0): The last version with UMD/global script support. xterm.js 5.x moved to ESM-only, which requires a bundler. The JS, CSS, and license are bundled in `ui/vendor/` and embedded into the binary via `go:embed` — no CDN, no npm, no build step, fully offline.

- **xterm-addon-fit** (v0.5.0): Companion addon that auto-sizes the terminal to fill its container. Without it, you'd need to manually calculate rows/cols from pixel dimensions and font metrics.

- **Full-screen layout**: `html, body { height: 100%; overflow: hidden }` ensures the terminal fills the entire viewport with no scrollbars. The `overflow: hidden` is important because xterm.js manages its own scrollback buffer — browser-level scrolling would interfere.

- **The godom directive**: `<div id="terminal" g-plugin:xterm="Terminal"></div>` connects the div to the `Terminal` field on the Go struct via the xterm plugin.

## Data flow in detail

### A keystroke's journey

1. User presses a key in the browser
2. xterm.js captures it via its internal key handler
3. `term.onData` fires with the character(s) — e.g., `"a"` for the letter a, `"\x1b[A"` for arrow up, `"\r"` for Enter
4. The adapter sends it as binary over the terminal WebSocket: `ws.send(new TextEncoder().encode(input))`
5. Go's read loop receives the binary message: `conn.ReadMessage()`
6. Go writes the bytes to the PTY master: `ptmx.Write(data)`
7. The shell process reads from its stdin (the PTY slave side) and processes the input
8. The shell produces output (echo of the character, command result, new prompt)
9. The output appears on the PTY master's read side
10. Go's broadcast goroutine reads it: `ptmx.Read(buf)`
11. Go sends it to all connected browsers as binary WebSocket frames
12. The adapter receives it: `ws.onmessage`
13. xterm.js renders it: `term.write(new Uint8Array(evt.data))`
14. The character appears on screen

On localhost, this entire round-trip is sub-millisecond. Over a network (Tailscale, etc.), latency depends on the network path but is typically fast enough for interactive use.

### What "raw bytes" means

The PTY produces and consumes raw bytes — not structured data. A simple `ls` command might produce output like:

```
\x1b[0m\x1b[01;34mDocuments\x1b[0m  \x1b[01;34mDownloads\x1b[0m  file.txt\r\n
```

These are ANSI escape sequences: `\x1b[01;34m` means "bold blue", `\x1b[0m` means "reset". xterm.js understands these natively and renders them as colored text. The Go side never interprets this data — it's opaque bytes flowing from the shell through the PTY to the browser.

## Session lifecycle

### Start
1. `main()` spawns the shell and PTY via `newTermSession()`
2. godom serves the page
3. Browser loads xterm.js, connects to terminal WebSocket
4. Initial terminal size is sent, PTY is resized
5. Shell prints its prompt

### Browser close and reopen
1. Browser closes → terminal WebSocket disconnects
2. The PTY and shell keep running (they're in the Go process, not the browser)
3. Browser reopens → new WebSocket connection
4. The shell is still running; new output appears immediately
5. Previous scrollback is lost (it was in xterm.js, which was destroyed)

### Shell exit
1. User types `exit` or presses Ctrl+D
2. PTY read returns an error (shell process ended)
3. The broadcast goroutine sends a notification to all connected browsers:
   - `[shell exited — this is a browser-based terminal powered by godom, starting new session...]`
   - `[to exit the terminal, close this tab or terminate the godom process]`
4. The old PTY is closed, a new shell is spawned via `ts.spawn()`
5. The new shell's prompt appears — the browser connections stay alive, no reconnect needed

### Application exit
The godom process itself only exits when:
- The user kills it (Ctrl+C in the terminal where `go run .` is running)
- The process is terminated externally (`kill`, task manager, etc.)
- `app.Start()` returns an error (port conflict, etc.)

## Security

### This is a shell on your machine

This application gives **full shell access** to the machine it runs on, with the permissions of the user who started the Go process. Anyone who can reach the terminal WebSocket and has the auth token can:

- Read and modify any file the user owns
- Install or remove software
- Access credentials, SSH keys, API tokens stored on the machine
- Start processes, open network connections, pivot to other machines
- Potentially escalate privileges if sudo is configured without a password

**This is equivalent to leaving an unlocked terminal session open.** Treat it with the same caution.

### Auth token

Both godom and the terminal WebSocket use token-based authentication:

- A random 32-character hex token is generated on each startup
- godom passes this token to the browser via plugin data (over its own authenticated connection)
- The browser sends the token as a query parameter when connecting to the terminal WebSocket
- The terminal WebSocket rejects connections without a valid token

This prevents casual/accidental access but has limitations:

- The token is transmitted in the WebSocket URL, which means it appears in server logs, browser history, and potentially proxy logs
- There's no session expiry — the token is valid for the lifetime of the process
- There's no rate limiting on auth attempts

### Binding to localhost vs. network interfaces

By default, both godom and the terminal WebSocket bind to `localhost` — only accessible from the same machine. This is the safest configuration.

**If you bind to `0.0.0.0` (all interfaces), the terminal becomes accessible from the network.** Anyone on the same LAN who discovers the port can attempt to connect. The auth token provides protection, but:

- Local networks are often less trusted than assumed (coffee shops, shared offices, conference Wi-Fi)
- Port scanning is trivial — an attacker on the same network can find your terminal in seconds
- The token is sent over unencrypted HTTP/WS — anyone on the network path can intercept it

**Do not expose this to the public internet directly.** No TLS, no robust auth, no rate limiting — this is not designed to be internet-facing.

### Safe remote access with Tailscale

If you need to access the terminal from another device (your phone, tablet, another laptop), **Tailscale** (or any WireGuard-based mesh VPN) is the recommended approach:

1. Install Tailscale on the machine running the terminal app and on the device you want to access it from
2. Run the terminal app bound to the Tailscale interface:
   ```bash
   go run . --host 0.0.0.0
   ```
   Or bind specifically to the Tailscale IP (usually `100.x.y.z`).
3. From your other device, open the browser and navigate to the godom URL using the Tailscale IP

This gives you:

- **End-to-end encryption** — Tailscale uses WireGuard, so all traffic (including the auth token) is encrypted in transit. No TLS certificate needed.
- **Authentication at the network layer** — only devices on your Tailscale network can reach the port. No port scanning from the internet, no random drive-by connections.
- **No port forwarding, no public IP** — Tailscale handles NAT traversal. Your terminal is never exposed to the public internet.
- **Access from anywhere** — as long as both devices are on your Tailscale network, you can reach your terminal from your phone on cellular, from a hotel Wi-Fi, from anywhere.
- **godom's QR code feature** — when bound to a network interface, godom shows a QR code with the URL. Scan it on your phone (on the same Tailscale network) and you have a terminal session instantly.

This is the sweet spot: zero-config remote access with strong security guarantees, without making the application itself responsible for TLS, certificate management, or user authentication.

### Recommended configurations

| Scenario | Host flag | Risk level | Notes |
|---|---|---|---|
| Local development | (default: `localhost`) | Low | Only accessible from the same machine |
| Tailscale/VPN access | `--host 0.0.0.0` | Low | Encrypted tunnel, authenticated network |
| Trusted home LAN | `--host 0.0.0.0` | Medium | Other devices on LAN can discover it |
| Shared/public network | `--host 0.0.0.0` | **High** | Token sent in cleartext, port scannable |
| Public internet | **Do not do this** | **Critical** | No TLS, no robust auth, full shell access |

## Dependencies

| Dependency | Purpose | Why |
|---|---|---|
| `godom` | Page serving, auth, plugin system | Framework this example is built on |
| `github.com/creack/pty` | PTY allocation and management | The standard Go library for pseudo-terminal operations. Handles the syscalls for creating PTY pairs, starting processes with a PTY, and resizing |
| `github.com/gorilla/websocket` | Terminal WebSocket server | godom doesn't expose its HTTP mux or WebSocket handling, so the terminal needs its own WebSocket server for raw I/O |
| xterm.js 4.19.0 (embedded) | Terminal emulation in the browser | Mature, fast terminal emulator. Handles ANSI parsing, cursor, colors, scrollback, selection, mouse events |
| xterm-addon-fit (embedded) | Auto-resize terminal to container | Calculates optimal rows/cols for the container size |

## What this doesn't do (yet)

- **Multiple terminal tabs** — currently one PTY session per application instance. Multiple sessions would need a session manager and UI for switching.
- **Scrollback persistence** — scrollback lives in xterm.js (browser). Closing the tab loses it. Could buffer recent output in Go and replay on reconnect.
- **TLS** — no HTTPS or WSS. Use Tailscale or a reverse proxy if you need encryption.
- **File upload/download** — out of scope; use scp/sftp.
- **Copy/paste** — xterm.js supports selection and Ctrl+C/Ctrl+V natively, but clipboard access requires HTTPS or localhost.
