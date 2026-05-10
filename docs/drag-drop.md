# Drag and Drop — Design Decisions

## The problem

godom apps need drag-and-drop for practical tools like form builders, kanban boards, and sortable lists. The challenge: HTML5 drag-and-drop is a browser API, but godom's architecture demands that all state and logic live in Go. The browser is a rendering engine, not the source of truth.

We need drag-and-drop that:
- Works with godom's directive model (`g-*` attributes)
- Keeps all state in Go (what's being dragged, where it lands, what to do about it)
- Supports common patterns: reordering within a list, dragging between containers, palette-to-canvas
- Doesn't require the developer to write any JavaScript

## Alternatives considered

### Option A: Full Go-side drag tracking

Route every `dragstart`, `dragover`, `dragenter`, `dragleave`, `dragend`, and `drop` event through the WebSocket to Go. Go tracks all drag state — which element is being dragged, which element is being hovered, what visual feedback to show.

**Rejected.** `dragover` fires continuously (like `mousemove`) and requires `preventDefault()` to allow dropping. A round trip to Go for every `dragover` would add latency to the visual feedback. The solar system example showed that Go handles `mousemove` well, but drag events also need synchronous `preventDefault()` — by the time Go responds, the browser has already rejected the drop.

### Option B: Pure JS drag handling

Let the bridge handle all drag logic in JavaScript. Only send the final drop result to Go.

**Rejected.** This contradicts godom's core principle that the bridge is a thin command executor. If the bridge manages drag state, tracks groups, and computes drop positions, we've moved logic into JavaScript that should be testable in Go.

### Option C: Split responsibility (chosen)

The bridge handles the HTML5 DnD ceremony (the required event listeners, `preventDefault`, `dataTransfer`, visual feedback CSS classes). Go handles the semantics (what data is being dragged, what happens on drop, which groups interact).

This maps cleanly to how `g-bind` already works: the bridge handles the browser mechanics (listening for `input` events, reading `element.value`) while Go owns the data and logic.

## The design

### Directives

Three directives cover the common patterns:

| Directive | Role | Analogy |
|-----------|------|---------|
| `g-draggable="value"` | Make an element draggable, attach data | Like `g-bind` — declares what data this element carries |
| `g-draggable:group="value"` | Draggable within a named group | Group isolation via a JS-side group variable + `data-drop-group` attribute |
| `g-drop="Method"` | Handle drops, call a Go method | Like `g-click` — an event handler |
| `g-drop:group="Method"` | Drop handler filtered by group | Only fires for matching `g-draggable:group` |
| `g-dropzone="Method"` | Synonym for `g-drop` | Registers a drop handler (kept for readability where the element is conceptually a "zone") |

### Groups

Groups solve a real problem: in a form builder, you don't want palette items to be reorderable as canvas items. You need two separate drag interaction spaces.

```html
<!-- Palette items: only droppable on palette drop handlers -->
<div g-draggable:palette="'text'">Text Input</div>

<!-- Canvas items: only droppable on canvas drop handlers -->
<div g-draggable:canvas="i" g-drop:canvas="Reorder">...</div>

<!-- Canvas accepts palette drops AND canvas drops (separate handlers) -->
<div g-drop:palette="AddField" g-drop:canvas="Reorder">...</div>
```

The group name (after the colon in `g-draggable:palette` / `g-drop:palette`) is rendered into the DOM as `data-drag-group` on the source and `data-drop-group` on the target. The bridge stores the active drag group in a module-level JS variable on `dragstart` (`_currentDragGroup`) and consults it on `dragover` and `drop`: if the target carries a non-empty `data-drop-group` that doesn't match, the event is short-circuited (`dragover` doesn't `preventDefault`, so the drop is rejected at the browser level; `drop` returns early before sending anything to Go). The `dataTransfer` payload itself uses plain `text/plain` and carries only the source's `data-drag-value`.

**Why a JS-side variable, not MIME types?** A direct group string in a closure variable is the simplest thing that works and is easy to read in the bridge. We considered using custom MIME types in `dataTransfer.setData` to lean on the browser's native drag-data filtering, but that adds ceremony for no measurable benefit at the scales godom drag-drop targets — a small number of drag groups per page, all originating in the same document.

### Drop data flow

When a drop occurs:

1. Bridge reads `from` (the source's `data-drag-value`, originally the resolved `g-draggable` expression — defaults to `"null"` if the source had no draggable value)
2. Bridge reads `to` (the drop target's `data-drag-value` — also `"null"` if the target has no `g-draggable`; this is common for "container" zones)
3. Bridge sends `from` and `to` as the first two arguments of a `MethodCall`. Any extra args declared in the directive (e.g. `g-drop="Reorder(item)"`) are appended after them.
4. Go unpacks the arguments and calls the method via reflection.

The Go method signature is just whatever the receiving method declares:
- `Reorder(from, to float64)` — typical sortable list using indices
- `AddField(from, to float64)` — palette-to-canvas; if the palette carries a string payload, declare `from string` instead
- Extra trailing args from the directive expression are passed positionally after `from, to`

**Why `float64` for indices?** When the resolved `g-draggable` value is a number, the bridge encodes it as a JSON number, and Go's reflection-based dispatcher delivers it as `float64`. Cast to `int` in the method body if you need an index. String payloads (`g-draggable="'text'"`) arrive as `string`. This keeps the bridge dumb — it forwards whatever JSON encoded — and lets Go handle the types.

### CSS feedback

The bridge applies CSS classes automatically during drag:

| Class | When applied | Purpose |
|-------|-------------|---------|
| `.g-dragging` | On the source element during drag | Dim or hide the original |
| `.g-drag-over` | On drop zone when a compatible draggable hovers | General "can drop here" feedback |

Classes are cleaned up on `dragend` and `dragleave`. The developer styles these in CSS — the framework just applies and removes them. There is intentionally no built-in "above/below" classifier; if a list needs that affordance, the receiving Go method can read the cursor position from a follow-up event or the app can compute insertion using the source/target indices alone.

**Why CSS classes, not style commands from Go?** Drag feedback needs to be instant — sub-frame latency. A round trip to Go for "add class on hover" would feel sluggish. The bridge applies classes directly because this is visual feedback, not state. Go never needs to know that an element has `.g-drag-over` — it only cares about the final drop.

### What the bridge handles vs. what Go handles

| Concern | Bridge (JS) | Go |
|---------|-------------|-----|
| `dragstart` listener | ✅ Sets `dataTransfer` (`text/plain`), records `_currentDragGroup`, adds `.g-dragging` | — |
| `dragover` listener | ✅ Group check; if compatible, `preventDefault()` + adds `.g-drag-over` | — |
| `dragleave` | ✅ Removes `.g-drag-over` | — |
| `dragend` | ✅ Removes `.g-dragging`, clears `_currentDragGroup` | — |
| `drop` listener | ✅ Group check, reads `data-drag-value` from source and target, sends `(from, to, ...args)` | ✅ Receives `(from, to, ...)`, calls method, diffs state |
| Group filtering | ✅ JS variable + `data-drop-group` attribute | ✅ Group name encoded in directive (`:group` suffix) |
| What happens on drop | — | ✅ Reorder slice, add item, remove item, whatever the method does |

This split mirrors the existing architecture: the bridge handles browser mechanics, Go handles semantics. The bridge never decides what a drop *means* — it just reports what happened.

## How it works with g-for

Drag-and-drop combined with `g-for` lists is the primary use case. A sortable list looks like:

```html
<div g-for="item, i in Items"
     g-draggable="i"
     g-drop="Reorder"
     g-text="item.Name">
</div>
```

Each item is both draggable (with its index as data) and a drop target. Dropping item 3 onto item 1 calls `Reorder(3, 1)`. The Go method reorders the slice, and godom's list diffing handles the DOM update.

The `g-draggable` binding is re-evaluated on each render, so indices stay correct after reordering. This is important — after moving item 3 to position 1, the old item 1 is now at position 2, and its `g-draggable` value updates accordingly.

## The form builder as proof

The `basic-form-builder` example exercises all of this:

- **Palette → canvas** (`g-draggable:palette` / `g-drop:palette`): Drag field types from a palette, drop onto the canvas to add them. Different groups prevent palette items from being sortable within the palette.
- **Canvas reordering** (`g-draggable:canvas` / `g-drop:canvas`): Drag canvas fields to reorder them within the form.
- **Canvas → trash** (`g-drop:canvas` on a trash zone): Drag a canvas field to the trash to remove it.
- **CSS feedback**: `.g-dragging` dims the source, `.g-drag-over` highlights the active drop target.

Three groups (`palette`, `canvas`, and ungrouped), three drop handlers (`AddField`, `Reorder`, `RemoveField`), zero JavaScript.
