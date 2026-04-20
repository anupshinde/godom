# crash-test

Demonstrates godom's disconnect overlay, disconnect badge, and panic recovery.

## Usage

```bash
# Root mode — custom overlay (default)
go run .

# Root mode — built-in godom overlay
go run . -default

# Embedded mode — custom badge
go run . -embed

# Embedded mode — built-in godom badge
go run . -embed -default
```

## What to expect

### Root mode (full-page overlay)

Three disconnect scenarios:

- **Exit button** — clean `os.Exit(0)` after 5s countdown. Shows "Disconnected" overlay with no error message.
- **Crash button** — panic via `godom.call` after 5s countdown. The framework catches the panic, sends the error message to the browser, then exits. Overlay shows the panic message.
- **Background crash** — a 30s countdown runs in a background goroutine and panics when it hits zero. This panic is *not* recoverable by the framework (Go can't recover panics across goroutines), so the overlay shows "Disconnected" with no error detail.

### Embedded mode (per-island badge)

A static HTML page with two godom islands (clock and controls). Kill the server to see disconnect badges appear next to each island.

## Customization

By default this example loads custom HTML from `ui/partials/`:
- `disconnect.html` — full-page overlay (root mode)
- `disconnect-badge.html` — per-island badge (embedded mode)

Use `-default` to see godom's built-in overlay/badge instead.

To customize in your own app:

```go
eng.DisconnectHTML = `<div>Your custom overlay</div>`
eng.DisconnectBadgeHTML = `<div>Your custom badge</div>`
```

Include an element with class `godom-disconnect-error` containing a `<pre>` tag to display crash error messages in the overlay.
