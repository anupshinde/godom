# godom — What's Next

Potential features, roughly ordered by value.

---

## Computed Properties

Methods that derive from state, auto-called on render:

```go
func (t *TodoApp) Remaining() int {
    count := 0
    for _, todo := range t.Todos {
        if !todo.Done {
            count++
        }
    }
    return count
}
```

Used in HTML: `<span g-text="Remaining"></span>`

Framework detects it's a method (not a field), calls it, uses the return value.

---

## Component Lifecycle Hooks

- `OnMount()` — called when component first renders
- `OnUnmount()` — called when component is removed
- `OnUpdate()` — called after state change

---

## Application Context & Object Hierarchy

Support a hierarchy where the app holds views, views hold components, each with their own lifecycle and state scope. Enables patterns like "settings panel with temporary state" or "modal form that gets discarded on cancel."

---

## Static File Serving

Serve images, fonts, CSS from a directory alongside the embedded HTML.

---

## Attribute & Style Binding

- `g-attr:key="expr"` — bind any HTML attribute to state
- `g-style:prop="expr"` — bind inline style properties

---

## Concurrency

Goroutines pushing state changes (timers, background tasks). Requires explicit sync with the main component mutex.

---

## Disconnect Handling

When the Go process exits or crashes, the bridge should update the page to show a disconnected state instead of silently freezing.

---

## Drop gorilla/websocket dependency

Currently using `github.com/gorilla/websocket`. Evaluate alternatives:
- `golang.org/x/net/websocket` — already in dep tree (used for HTML parsing), covers godom's needs
- SSE + POST — see [docs/transport.md](transport.md) for detailed analysis
- Stdlib websocket — not available yet, monitor future Go releases
