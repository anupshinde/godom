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

## ~~Publish module path~~ ✅

Implemented. Module path changed from `godom` to `github.com/anupshinde/godom`. All internal imports and sub-module go.mod files updated. README Install section now shows `go get github.com/anupshinde/godom`.

---

## Keyed identity for g-for stateful components

Currently, g-for items have positional identity — item 0 is always gid `g3-0`, item 1 is `g3-1`, etc. Child component instances are stored by index. This means inserts, deletes in the middle, or reorders can attach a child's local state to the wrong logical item.

**What's needed:**
- Keyed diff algorithm (detect inserts, deletes, moves — not just append/truncate)
- New wire operations: `list-insert-at`, `list-remove-at`, `list-move`
- Gid generation that encodes key instead of index (or a key→gid mapping)
- Child instance tracking by key instead of flat index slice
- Template HTML can't pre-bake `__IDX__` if items move; gids need rewriting on the fly

**Syntax idea:** `g-for="todo, i in Todos" g-key="todo.ID"`

This is a cross-cutting change touching parser, renderer, protocol, and bridge. Not urgent for append-only or full-replace lists, but required for reorderable lists with stateful children.

---

## Child-local state lost when scoped event also changes root state

For scoped calls and binds, child re-rendering only happens when the root component shows no changed fields. If a child mutates its own local state and also causes parent/root state to change (e.g. via a callback), only the root update path runs, so child-local UI updates are dropped.

**Fix:** The root update path (`computeUpdateMessage`) needs to also include child re-renders for any scoped components that were involved in the current event, not just skip them when root state changed.

---

## Drop gorilla/websocket dependency

Currently using `github.com/gorilla/websocket`. Evaluate alternatives:
- `golang.org/x/net/websocket` — already in dep tree (used for HTML parsing), covers godom's needs (binary WebSocket read/write for protobuf)
- SSE + POST — see [docs/transport.md](transport.md) for detailed analysis
- Stdlib websocket — not available yet, monitor future Go releases

Note: the wire format is already Protocol Buffers (binary WebSocket). Any WebSocket replacement just needs to support binary message read/write.

---

## Validator hardening

The startup validator (`validate.go`) has known limitations — it's useful as a guardrail but not a source of truth:

- **Child component validation is too permissive**: if a directive doesn't validate against the parent, it falls back to any registered child component type, not the actual subtree where it appears. Can allow incorrect templates to pass.
- **g-bind validation is broader than runtime**: validates dotted-path and loop-scoped binds, but runtime `setField` only supports direct top-level fields via `FieldByName`. Valid templates can still fail at runtime.
- **g-props collection is global**: `collectLoopVars` scans all g-props in the entire HTML for every g-for, not scoped to the actual subtree. Can create false positives in complex nested structures.

These are low priority — the runtime parser/render path is more solid than the validator.

---

## Review: g-for implementation

The `g-for` implementation — especially nested g-for — needs a manual review pass. Areas to audit:

- **GID replacement logic** in `computeSubLoopCmd` (render.go) — the two-step replacement (outer prefix first, then inner `__IDX__`) is correct for two levels but should be stress-tested for 3+ nesting depths
- **Inner list diffing** — currently absent; inner loops fully re-render when the outer item changes. May need per-inner-loop `prevLists` tracking for performance with large inner lists
- **Edge cases** — empty inner lists, outer items added/removed while inner loops exist, interaction with stateful components inside nested loops
- **Bridge anchor cleanup** — when outer list items are removed, inner anchors in `anchorMap` are not explicitly cleaned up (they become orphaned but harmless)
- ~~**Bridge innerHTML context**~~ ✅ — Fixed. `createTmpContainer()` now uses `start.parentNode.tagName` with a `contextWrappers` lookup map.

---

## Review: "re-event" op in protocol.proto

`protocol.proto` lists `"re-event"` as a valid `op` value, but no Go code ever sends it and `bridge.js` has no handler for it. The event re-registration behavior exists implicitly — resending an event setup command for the same element just overwrites `eventMap[key]` in the bridge. Decide whether to:

- **Remove** `"re-event"` from the proto comment (it was never a real op)
- **Keep** it if there's a future plan to make re-registration an explicit, distinct operation

Also update `architecture.md`'s op list to match whatever is decided — it's currently missing `draggable`, `dropzone`, and `style` ops.
