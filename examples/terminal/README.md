# godom terminal

A browser-based terminal powered by [godom](../../). Run a single Go binary, open a browser tab, get full shell access to the machine — your shell, your dotfiles, your tools.

## Quick start

```bash
cd examples/terminal
go run .
```

A browser tab opens with a full terminal session. That's it.

To build a standalone binary:

```bash
cd examples/terminal
go build -o terminal .
./terminal
```

Everything — HTML, CSS, xterm.js — is embedded. The binary runs anywhere with no external files or network dependencies.

## What it does

- Spawns your default shell (`$SHELL`) with a pseudo-terminal (PTY)
- Renders it in the browser using [xterm.js](https://xtermjs.org/)
- Full color support (256 colors), cursor movement, TUI apps (`vim`, `htop`, `top`, `less`) all work
- Resize-aware — resizing the browser window resizes the terminal
- Session survives browser close — close the tab, reopen it, the shell is still running
- Multiple browser tabs can view the same session simultaneously
- Shell respawns automatically on exit — typing `exit` starts a fresh session, it doesn't kill the app

## How it works

godom serves the page and handles authentication. A separate WebSocket carries raw PTY I/O between the browser and the Go process. xterm.js in the browser does all the terminal emulation (ANSI parsing, rendering, input handling).

See [implementation.md](implementation.md) for the full architectural deep-dive.

## Remote access with Tailscale

This becomes genuinely useful when combined with [Tailscale](https://tailscale.com/) (or any mesh VPN):

```bash
GODOM_HOST=0.0.0.0 go run .
```

Then from any device on your Tailscale network — phone, tablet, another laptop — open the browser and navigate to the Tailscale IP shown in the output. You have a terminal on your machine. No SSH client needed, no port forwarding, no public IP.

Tailscale provides end-to-end WireGuard encryption and network-level authentication, so the terminal is never exposed to the public internet.

godom's built-in QR code feature makes mobile access even easier — scan the QR code on your phone and you're in.

## Security

**This application provides full shell access to the machine it runs on.** Treat it like an unlocked terminal session.

- **localhost only (default)** — safe for local development, only accessible from the same machine
- **Tailscale/VPN** (`GODOM_HOST=0.0.0.0`) — safe for remote access over an encrypted, authenticated network
- **Open network** (`GODOM_HOST=0.0.0.0` on shared Wi-Fi) — risky, auth token is sent in cleartext, port is scannable
- **Public internet** — **do not do this**. No TLS, no robust authentication, full shell access

Auth tokens are generated per session and validated on both the godom connection and the terminal WebSocket. But they are transmitted over unencrypted HTTP, so network-level encryption (Tailscale, VPN, or a reverse proxy with TLS) is essential for any non-localhost use.

See the Security section in [implementation.md](implementation.md) for the full threat model.

## Environment variables

All standard godom env vars apply:

| Env variable | Default | Description |
|---|---|---|
| `GODOM_PORT` | random | Port for the godom HTTP server |
| `GODOM_HOST` | `localhost` | Interface to bind to (`0.0.0.0` for network access) |
| `GODOM_NO_AUTH` | `false` | Disable token authentication (not recommended) |
| `GODOM_TOKEN` | random | Use a fixed auth token instead of generating one |
| `GODOM_NO_BROWSER` | `false` | Don't open browser automatically on start |
| `GODOM_QUIET` | `false` | Suppress startup output |

## Dependencies

- [godom](../../) — page serving, auth, plugin system
- [creack/pty](https://github.com/creack/pty) — PTY allocation (pseudo-terminal syscalls)
- [gorilla/websocket](https://github.com/gorilla/websocket) — terminal WebSocket server
- [xterm.js](https://xtermjs.org/) 4.19.0 — terminal emulation in the browser (embedded in binary, MIT license)
