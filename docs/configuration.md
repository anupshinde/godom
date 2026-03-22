# Configuration

godom apps can be configured in two ways: **in code** (by setting fields on the `Engine` struct) and **via CLI flags** (passed when running the binary). CLI flags override framework defaults, but values set in code always take priority.

## Settings

### Port

The TCP port to listen on.

| | Value |
|---|---|
| Default | `0` (random available port) |
| Code | `eng.Port = 8081` |
| CLI | `--port=8081` |

### Host

The network interface to bind to.

| | Value |
|---|---|
| Default | `localhost` (loopback only) |
| Code | `eng.Host = "0.0.0.0"` |
| CLI | `--host=0.0.0.0` |

Set to `0.0.0.0` to allow access from other machines on the network. The startup URL and QR code will show your machine's LAN IP instead of `localhost`.

### NoAuth

Disable token-based authentication.

| | Value |
|---|---|
| Default | `false` (auth enabled) |
| Code | `eng.NoAuth = true` |
| CLI | `--no-auth` |

### Token

Use a fixed auth token instead of generating a random one on each startup. Useful for bookmarks, scripts, or services that restart.

| | Value |
|---|---|
| Default | `""` (generate random token) |
| Code | `eng.Token = "my-secret"` |
| CLI | `--token=my-secret` |

Ignored when `NoAuth` is set.

### NoBrowser

Don't open the browser automatically on startup. Useful for headless servers or when running as a background service.

| | Value |
|---|---|
| Default | `false` (open browser) |
| Code | `eng.NoBrowser = true` |
| CLI | `--no-browser` |

### Quiet

Suppress the startup URL and QR code output.

| | Value |
|---|---|
| Default | `false` (print URL and QR code) |
| Code | `eng.Quiet = true` |
| CLI | `--quiet` |

## CLI flags

Every godom app automatically supports these flags — no code changes needed:

```
--port=PORT      Port to listen on (default: random)
--host=HOST      Host to bind to (default: localhost)
--no-auth        Disable token authentication
--token=TOKEN    Use a fixed auth token (default: random)
--no-browser     Don't open browser on start
--quiet          Suppress startup output
```

Examples:

```
./myapp --port=8081
./myapp --host=0.0.0.0 --port=8081
./myapp --no-browser --token=my-secret
./myapp --quiet
```

## Precedence

```
Developer code  >  CLI flags  >  Framework defaults
```

If a developer explicitly sets a value in code, the CLI flag for that setting is ignored. This lets developers lock down settings when needed while still giving end users control over defaults.

For example:

```go
eng := godom.NewEngine()
eng.Port = 9000  // locked to 9000 — `--port` flag is ignored
// Host is not set — `--host` flag applies
// NoAuth is not set — `--no-auth` flag applies
```

## Authentication

godom generates a random auth token on every startup. The full URL (with token) is printed to the terminal along with a QR code, and opened in the browser automatically:

```
godom running at http://localhost:8081?token=a1b2c3d4e5f6...
█▀▀▀▀▀█ ...  █▀▀▀▀▀█
█ ███ █ ...  █ ███ █
...
```

When using `--host=0.0.0.0`, the URL displays your machine's LAN IP (e.g., `http://192.168.1.10:8081?token=...`) so the QR code can be scanned from other devices on the network.

### How it works

1. On startup, a 32-character hex token is generated using `crypto/rand` (unless a fixed token is provided via `Token` or `--token`)
2. The browser is opened with `?token=...` in the URL (unless `NoBrowser` is set)
3. The server validates the token and sets an **HttpOnly** cookie (`godom_token`)
4. The URL is redirected to strip the token from the address bar
5. Subsequent visits use the cookie — no token needed in the URL

### Cookie behavior

- Cookies are scoped per hostname. Accessing via `localhost` and `192.168.1.10` are separate cookie jars — each needs the token URL on first visit
- Cookies persist across browser restarts — close the tab, reopen `localhost:8081`, and you're back in
- The cookie uses `SameSite=Strict` and `HttpOnly` flags for security

### Fixed tokens

By default, a new random token is generated on every startup. This means restarting the app invalidates old bookmarks and cookies.

Use a fixed token for stable access across restarts:

```go
eng.Token = "my-secret"  // in code
```

```
./myapp --token=my-secret  # from command line
```

### Sharing access

To give someone else access to your app over the network:

1. Set `eng.Host = "0.0.0.0"` (or run with `--host=0.0.0.0`)
2. Share the token URL from the terminal output
3. They visit the URL once, get a cookie, and can revisit without the token

Anyone without the token or cookie gets a 401 Unauthorized response.

### Disabling auth

For local-only tools where multi-user security isn't a concern:

```go
eng.NoAuth = true   // in code
```

```
./myapp --no-auth   # from command line
```

When auth is disabled, no token is generated and all requests are allowed.

## Headless / service mode

To run godom as a background service on a headless machine:

```
./myapp --no-browser --host=0.0.0.0 --port=8081 --token=my-secret
```

This binds to all interfaces on a fixed port with a stable token, without trying to open a browser. Access the UI from any browser on the network using the token URL.
