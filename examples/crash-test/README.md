# crash-test

Demonstrates godom's disconnect overlay and panic recovery.

## Usage

```bash
# Custom disconnect overlay (default)
go run .

# Built-in godom disconnect overlay
go run . -default
```

## What to expect

Three disconnect scenarios:

- **Exit button** — clean `os.Exit(0)` after 5s countdown. Shows "Disconnected" overlay with no error message. Server won't auto-reconnect.
- **Crash button** — panic via `godom.call` after 5s countdown. The framework catches the panic, sends the error message to the browser, then exits. Overlay shows the panic message.
- **Background crash** — a 30s countdown runs in a background goroutine and panics when it hits zero. This panic is *not* recoverable by the framework (Go can't recover panics across goroutines), so the overlay shows "Disconnected" with no error detail.

## Custom overlay

By default this example loads a custom overlay from `ui/partials/disconnect.html`. Use `-default` to see godom's built-in overlay instead.

To customize the overlay in your own app:

```go
eng.DisconnectHTML = `<div>Your custom HTML here</div>`
```

Include an element with class `godom-disconnect-error` containing a `<pre>` tag to display crash error messages.
