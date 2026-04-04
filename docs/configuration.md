# Configuration

godom apps can be configured via **environment variables** (`GODOM_*`). `NewEngine()` reads env vars at creation time. Values set in code after `NewEngine()` override env vars.

## Settings

### Port

The TCP port to listen on (used by `ListenAndServe` and `QuickServe`).

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

Disable token-based authentication on `/ws` and `/godom.js`.

| | Value |
|---|---|
| Default | `false` (auth enabled) |
| Code | `eng.NoAuth = true` |
| Env | `GODOM_NO_AUTH=1` |

### FixedAuthToken

Use a fixed auth token instead of generating a random one on each startup. Useful for bookmarks, scripts, or services that restart.

| | Value |
|---|---|
| Default | `""` (generate random token) |
| Code | `eng.FixedAuthToken = "my-secret"` |
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

### Validate Only

Exit immediately after `Run()` validation succeeds, without starting the server. Useful for CI checks and pre-commit hooks to catch template errors (unknown fields, invalid directives) without running the full app.

| | Value |
|---|---|
| Default | not set (normal startup) |
| Env | `GODOM_VALIDATE_ONLY=1` |

This is env-only — there is no code-level field.

## Browser-side settings

These are JavaScript `window` variables set in the HTML page **before** loading the godom JS bundle. They are not Go engine settings — they configure the bridge running in the browser.

### GODOM_WS_URL

Override the WebSocket URL the bridge connects to. By default, the bridge derives the URL from the current page's host. Set this when the HTML page is served from a different origin than the godom server (e.g. embedded widget pattern).

```html
<script>window.GODOM_WS_URL = "ws://localhost:9091/ws";</script>
<script src="http://localhost:9091/godom.js"></script>
```

When using `MuxOptions` with a custom `WSPath`, the server automatically injects the correct path into the JS bundle via the `__GODOM_WS_PATH__` template — no need to set `GODOM_WS_URL` for same-origin setups.

### GODOM_DISABLE_EXEC

Prevent godom from executing arbitrary JavaScript via `ExecJS`. When set, the bridge refuses to evaluate JS expressions sent by the server, returning an error instead.

```html
<script>window.GODOM_DISABLE_EXEC = true;</script>
<script src="http://localhost:9091/godom.js"></script>
```

This is a **page-owner control** — the person who writes the HTML decides whether the godom server can execute JS in their page. The server cannot override this because the flag is checked before any JS is evaluated, including attempts to change the flag itself.

There is also a server-side equivalent (`eng.DisableExecJS = true`) that prevents the server from sending ExecJS calls at all. Both can be used independently or together:

| | Server sends JSCall | Bridge executes |
|---|---|---|
| Neither set | Yes | Yes |
| `eng.DisableExecJS = true` | No | N/A (nothing arrives) |
| `GODOM_DISABLE_EXEC = true` | Yes | No (returns error) |
| Both set | No | No |

### GODOM_NS

Change the global namespace the bridge registers on. Default is `"godom"` (`window.godom`). Useful when embedding godom in a third-party page to avoid name collisions.

```html
<script>window.GODOM_NS = "myApp";</script>
<script src="http://localhost:9091/godom.js"></script>
```

### GODOM_DEBUG

Automatically injected by the server when `GODOM_DEBUG` is set. Accepts `1`, `true`, `0`, or `false`. Enables debug-level warnings in the bridge console (e.g. missing component targets during init). Not set manually — controlled via the server-side env var.

### DisconnectHTML

Custom HTML to display when the WebSocket connection is lost. Replaces the default disconnect overlay.

| | Value |
|---|---|
| Default | built-in disconnect overlay |
| Code | `eng.DisconnectHTML = "<div>Connection lost</div>"` |

### DisconnectBadgeHTML

Custom HTML for a small disconnect badge indicator, shown instead of the full overlay. Useful for a subtle notification.

| | Value |
|---|---|
| Default | not set (full overlay used) |
| Code | `eng.DisconnectBadgeHTML = "<span>offline</span>"` |

### Lifecycle hooks

The bridge exposes callbacks for WebSocket lifecycle events:

| Hook | When it fires |
|------|---------------|
| `godom.onconnect` | On WebSocket open — godom.call works. Fires on reconnect too. |
| `godom.ondisconnect(errorMsg)` | When WS closes. `null` = clean close (will reconnect). Non-null = fatal. |
| `godom.onerror(evt)` | On WS error, before close. |

```html
<script>
var godom = window.godom;
godom.onconnect = function() { /* connected */ };
godom.ondisconnect = function(err) { /* disconnected */ };
</script>
```

### Component readiness (`.g-ready` class)

The bridge adds a `.g-ready` CSS class to elements after their component tree is initialized:

- **Root mode**: added to `document.body`
- **Embedded mode**: added to each `[g-component]` element

This lets you hide raw template content (e.g. `{{Count}}`) until the component is live:

```css
body:not(.g-ready) { visibility: hidden; }
[g-component]:not(.g-ready) { visibility: hidden; }
```

The class is removed on cleanup (re-init after reconnect). This is purely a CSS hook — no configuration needed.

## Environment variables

godom reads `GODOM_*` environment variables when `NewEngine()` is called:

```
GODOM_PORT=8081
GODOM_HOST=0.0.0.0
GODOM_NO_AUTH=1
GODOM_TOKEN=my-secret
GODOM_NO_BROWSER=1
GODOM_QUIET=1
GODOM_VALIDATE_ONLY=1
```

Examples:

```
GODOM_PORT=8081 ./myapp
GODOM_HOST=0.0.0.0 GODOM_PORT=8081 ./myapp
GODOM_NO_BROWSER=1 GODOM_TOKEN=my-secret ./myapp
GODOM_VALIDATE_ONLY=1 ./myapp   # validate templates and exit
```

godom does not parse CLI flags. Your binary owns its flags entirely — there are no flag namespace collisions.

## Precedence

```
Code (after NewEngine)  >  Env vars (read by NewEngine)  >  Framework defaults
```

`NewEngine()` reads env vars into Engine fields. Setting a field in code after `NewEngine()` overrides the env value. For example:

```go
eng := godom.NewEngine()
eng.Port = 9000  // overrides GODOM_PORT
// Host keeps whatever GODOM_HOST was, or defaults to "localhost"
```

## Authentication

godom supports pluggable auth via `SetAuth()`. The default is token-based auth.

### Built-in token auth

When `NoAuth` is false (default), `Run()` sets up token-based authentication:

1. A 32-character hex token is generated using `crypto/rand` (unless `FixedAuthToken` is set)
2. `ListenAndServe()` opens the browser with `?token=...` in the URL
3. The auth middleware validates the token and sets an **HttpOnly** cookie (`godom_token`)
4. The URL is redirected to strip the token from the address bar
5. Subsequent requests use the cookie — no token needed in the URL

### Custom auth

Replace token auth with your own auth logic:

```go
eng.SetAuth(func(w http.ResponseWriter, r *http.Request) bool {
    // Check JWT, session cookie, API key, etc.
    return validateSession(r)
})
```

Custom auth is used on `/ws`, `/godom.js`, and by `AuthMiddleware()` when wrapping the mux.

### Auth middleware

When using `ListenAndServe()`, auth wraps the entire mux automatically. When using `http.ListenAndServe` directly, wrap your mux explicitly:

```go
log.Fatal(http.ListenAndServe(":8080", eng.AuthMiddleware(mux)))
```

### Cookie behavior

- Cookies are scoped per hostname. Accessing via `localhost` and `192.168.1.10` are separate cookie jars
- Cookies persist across browser restarts
- The cookie uses `SameSite=Strict` and `HttpOnly` flags

### Disabling auth

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

This binds to all interfaces on a fixed port with a stable token, without trying to open a browser.
