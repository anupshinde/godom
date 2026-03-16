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

## ~~Static File Serving~~ ✅

Implemented. Non-root HTTP paths are now served from the embedded UI filesystem via `http.FileServer`. CSS, images, fonts, and other assets placed alongside `index.html` in the `ui/` directory are served with correct MIME types.

Example: `examples/stock-ticker/` — uses `<link rel="stylesheet" href="style.css">` with a separate CSS file.

---

## ~~Style Binding~~ ✅

Implemented. `g-style:prop="expr"` binds inline style properties to Go struct fields. Uses `el.style.setProperty()` on the browser side, so CSS property names with hyphens work directly (e.g., `g-style:background-color="BgColor"`).

Example: `examples/progress-bar/` — animated progress bar driven by `g-style:width`.

---

## ~~Disconnect Handling~~ ✅

Implemented. The bridge shows a blurred dark overlay on disconnect:
- **Server stopped/killed**: "Disconnected — Waiting for server…" with auto-reconnect
- **Application crash (panic)**: "Application Crashed — Restart the application to continue" with the panic message in a code block; no auto-reconnect; process exits

---

## Drop gorilla/websocket dependency

Currently using `github.com/gorilla/websocket`. Evaluate alternatives:
- `golang.org/x/net/websocket` — already in dep tree (used for HTML parsing), covers godom's needs (binary WebSocket read/write for protobuf)
- SSE + POST — see [docs/transport.md](transport.md) for detailed analysis
- Stdlib websocket — not available yet, monitor future Go releases

Note: the wire format is already Protocol Buffers (binary WebSocket). Any WebSocket replacement just needs to support binary message read/write.

---

## Review: g-for implementation

The `g-for` implementation — especially nested g-for — needs a manual review pass. Areas to audit:

- **GID replacement logic** in `computeSubLoopCmd` (render.go) — the two-step replacement (outer prefix first, then inner `__IDX__`) is correct for two levels but should be stress-tested for 3+ nesting depths
- **Inner list diffing** — currently absent; inner loops fully re-render when the outer item changes. May need per-inner-loop `prevLists` tracking for performance with large inner lists
- **Edge cases** — empty inner lists, outer items added/removed while inner loops exist, interaction with stateful components inside nested loops
- **Bridge anchor cleanup** — when outer list items are removed, inner anchors in `anchorMap` are not explicitly cleaned up (they become orphaned but harmless)
- **Bridge innerHTML context** — `createTmpContainer()` currently inspects the HTML string to detect `<tr>`/`<td>`/`<th>` and wrap them correctly for parsing. This should be replaced with a parent-tag-based approach using `start.parentNode.tagName` to handle all context-sensitive elements (`<option>`, `<thead>`, `<tbody>`, etc.) in one shot. See known-issues.md for details.
