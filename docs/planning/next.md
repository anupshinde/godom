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

## Drop gorilla/websocket dependency

Currently using `github.com/gorilla/websocket`. Evaluate alternatives:
- `golang.org/x/net/websocket` — already in dep tree (used for HTML parsing), covers godom's needs (binary WebSocket read/write for protobuf)
- SSE + POST — see [docs/transport.md](transport.md) for detailed analysis
- Stdlib websocket — not available yet, monitor future Go releases

Note: the wire format is already Protocol Buffers (binary WebSocket). Any WebSocket replacement just needs to support binary message read/write.

---

## Validator hardening

The startup validator (`internal/template/validate.go`) has known limitations — it's useful as a guardrail but not a source of truth:

- **Child component validation is too permissive**: if a directive doesn't validate against the parent, it falls back to any registered child component type, not the actual subtree where it appears. Can allow incorrect templates to pass.
- **g-bind validation is broader than runtime**: validates dotted-path and loop-scoped binds, but runtime `setField` only supports direct top-level fields via `FieldByName`. Valid templates can still fail at runtime.
- **g-props collection is global**: `collectLoopVars` scans all g-props in the entire HTML for every g-for, not scoped to the actual subtree. Can create false positives in complex nested structures.

These are low priority — the runtime parser/render path is more solid than the validator.

---

## Previously completed

These items were on the next list and have been implemented:

- ~~Static File Serving~~ ✅ — Non-root HTTP paths served from embedded UI filesystem via `http.FileServer`
- ~~Style Binding~~ ✅ — `g-style:prop="expr"` binds inline style properties to Go struct fields
- ~~Disconnect Handling~~ ✅ — Bridge shows overlay on disconnect/crash with auto-reconnect or error message
- ~~Publish module path~~ ✅ — Module path changed to `github.com/anupshinde/godom`
- ~~Child-local state lost when scoped event also changes root state~~ ✅ — Fixed with dual-update approach
- ~~bridge.js g-for innerHTML parsing is context-sensitive~~ ✅ — Fixed with `contextWrappers` lookup map
- ~~Keyed identity for g-for~~ ✅ — Implemented via `g-key="item.ID"` with `KeyedElementNode` and `PatchReorder`
- ~~Virtual DOM~~ ✅ — Full VDOM pipeline: tree-based init, diff-based patches, stable node IDs
