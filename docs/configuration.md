# Configuration

godom apps can be configured in two ways: **in code** (by setting fields on the `Engine` struct) and **via environment variables** (`GODOM_*`). Values set in code always take priority over env vars.

## Settings

### Port

The TCP port to listen on.

| | Value |
|---|---|
| Default | `0` (random available port) |
| Code | `eng.Port = 8081` |
| Env | `GODOM_PORT=8081` |

### Host

The network interface to bind to.

| | Value |
|---|---|
| Default | `localhost` (loopback only) |
| Code | `eng.Host = "0.0.0.0"` |
| Env | `GODOM_HOST=0.0.0.0` |

Set to `0.0.0.0` to allow access from other machines on the network. The startup URL and QR code will show your machine's LAN IP instead of `localhost`.

### NoAuth

Disable token-based authentication.

| | Value |
|---|---|
| Default | `false` (auth enabled) |
| Code | `eng.NoAuth = true` |
| Env | `GODOM_NO_AUTH=1` |

### Token

Use a fixed auth token instead of generating a random one on each startup. Useful for bookmarks, scripts, or services that restart.

| | Value |
|---|---|
| Default | `""` (generate random token) |
| Code | `eng.Token = "my-secret"` |
| Env | `GODOM_TOKEN=my-secret` |

Ignored when `NoAuth` is set.

### NoBrowser

Don't open the browser automatically on startup. Useful for headless servers or when running as a background service.

| | Value |
|---|---|
| Default | `false` (open browser) |
| Code | `eng.NoBrowser = true` |
| Env | `GODOM_NO_BROWSER=1` |

### Quiet

Suppress the startup URL and QR code output.

| | Value |
|---|---|
| Default | `false` (print URL and QR code) |
| Code | `eng.Quiet = true` |
| Env | `GODOM_QUIET=1` |

## Environment variables

godom reads `GODOM_*` environment variables for any setting not already set in code:

```
GODOM_PORT=8081
GODOM_HOST=0.0.0.0
GODOM_NO_AUTH=1
GODOM_TOKEN=my-secret
GODOM_NO_BROWSER=1
GODOM_QUIET=1
```

Examples:

```
GODOM_PORT=8081 ./myapp
GODOM_HOST=0.0.0.0 GODOM_PORT=8081 ./myapp
GODOM_NO_BROWSER=1 GODOM_TOKEN=my-secret ./myapp
```

godom does not parse CLI flags. Your binary owns its flags entirely — there are no flag namespace collisions.

### Disabling env var reads

To prevent godom from reading any `GODOM_*` environment variables, set `NoGodomEnv`:

```go
eng := godom.NewEngine()
eng.NoGodomEnv = true  // skip all GODOM_* env var reads
eng.Port = 8081
eng.Start()
```

When `NoGodomEnv` is true, only values set in code apply. This is useful when you want full programmatic control and don't want external env vars influencing godom's configuration.

## Precedence

```
Code  >  Env vars  >  Framework defaults
```

If a field is set in code, the corresponding env var is not read. If `NoGodomEnv` is true, env vars are skipped entirely. This lets developers lock down settings when needed while still giving end users runtime control via env vars.

For example:

```go
eng := godom.NewEngine()
eng.Port = 9000  // locked to 9000 — GODOM_PORT is not read
// Host is not set — GODOM_HOST is checked, then defaults to "localhost"
// NoAuth is not set — GODOM_NO_AUTH is checked, then defaults to false
```

## Authentication

godom generates a random auth token on every startup. The full URL (with token) is printed to the terminal along with a QR code, and opened in the browser automatically:

```
godom running at http://localhost:8081?token=a1b2c3d4e5f6...
█▀▀▀▀▀█ ...  █▀▀▀▀▀█
█ ███ █ ...  █ ███ █
...
```

When using `Host = "0.0.0.0"`, the URL displays your machine's LAN IP (e.g., `http://192.168.1.10:8081?token=...`) so the QR code can be scanned from other devices on the network.

### How it works

1. On startup, a 32-character hex token is generated using `crypto/rand` (unless a fixed token is provided via `Token` or `GODOM_TOKEN`)
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
GODOM_TOKEN=my-secret ./myapp  # via env var
```

### Sharing access

To give someone else access to your app over the network:

1. Set `eng.Host = "0.0.0.0"` (or set `GODOM_HOST=0.0.0.0`)
2. Share the token URL from the terminal output
3. They visit the URL once, get a cookie, and can revisit without the token

Anyone without the token or cookie gets a 401 Unauthorized response.

### Disabling auth

For local-only tools where multi-user security isn't a concern:

```go
eng.NoAuth = true   // in code
```

```
GODOM_NO_AUTH=1 ./myapp  # via env var
```

When auth is disabled, no token is generated and all requests are allowed.

## Headless / service mode

To run godom as a background service on a headless machine:

```
GODOM_NO_BROWSER=1 GODOM_HOST=0.0.0.0 GODOM_PORT=8081 GODOM_TOKEN=my-secret ./myapp
```

This binds to all interfaces on a fixed port with a stable token, without trying to open a browser. Access the UI from any browser on the network using the token URL.
