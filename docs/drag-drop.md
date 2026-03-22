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
| `g-draggable:group="value"` | Draggable within a named group | Group isolation via MIME types |
| `g-dropzone="'name'"` | Mark a drop target with a name | Identifies this element for drop resolution |
| `g-drop="Method"` | Handle drops, call a Go method | Like `g-click` — an event handler |
| `g-drop:group="Method"` | Drop handler filtered by group | Only fires for matching `g-draggable:group` |

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

The group name (after the colon in `g-draggable:palette`) is encoded as a MIME type suffix in `dataTransfer`: `application/x-godom-palette`, `application/x-godom-canvas`. This uses the browser's native MIME-type filtering — `dragover` only fires if the MIME type matches, so incompatible drags don't trigger visual feedback.

**Why MIME types, not a JS-side group map?** Because the browser already has a filtering mechanism for drag data types. Using it means zero JS-side state for group tracking. The bridge sets the MIME type on `dragstart` and checks it on `dragover`/`drop` — no group registry, no lookup tables.

### Drop data flow

When a drop occurs:

1. Bridge reads `from` (the draggable's value from `dataTransfer`)
2. Bridge reads `to` (the drop target's `g-dropzone` value, or its own `g-draggable` value for sortable lists)
3. Bridge computes `position` (`"above"` or `"below"` based on cursor Y relative to the element's midpoint)
4. Bridge sends `from`, `to`, and `position` as JSON-encoded `MethodCall` args
5. Go unpacks the arguments and calls the method

The Go method signature is flexible:
- `Reorder(from, to float64)` — position is ignored (extra args are discarded)
- `Reorder(from, to float64, position string)` — position is used
- `AddField(fieldType string)` — from palette, only the drag data matters

**Why `float64` for indices?** JSON numbers are `float64`. The Go method receives them as `float64` and casts internally (`int(from)`). This avoids type coercion logic in the bridge — it sends what JSON gives it, and Go handles the types.

### CSS feedback

The bridge applies CSS classes automatically during drag:

| Class | When applied | Purpose |
|-------|-------------|---------|
| `.g-dragging` | On the source element during drag | Dim or hide the original |
| `.g-drag-over` | On drop zone when compatible item hovers | General "can drop here" feedback |
| `.g-drag-over-above` | On sortable item, cursor in top half | Show insertion line above |
| `.g-drag-over-below` | On sortable item, cursor in bottom half | Show insertion line below |

Classes are cleaned up on `dragend` and `dragleave`. The developer styles these in CSS — the framework just applies and removes them.

**Why CSS classes, not style commands from Go?** Drag feedback needs to be instant — sub-frame latency. A round trip to Go for "add class on hover" would feel sluggish. The bridge applies classes directly because this is visual feedback, not state. Go never needs to know that an element has `.g-drag-over` — it only cares about the final drop.

### What the bridge handles vs. what Go handles

| Concern | Bridge (JS) | Go |
|---------|-------------|-----|
| `dragstart` listener | ✅ Sets `dataTransfer`, adds `.g-dragging` | — |
| `dragover` listener | ✅ `preventDefault()`, computes above/below, applies CSS | — |
| `dragenter`/`dragleave` | ✅ Counter-based tracking, CSS cleanup | — |
| `dragend` | ✅ Removes all drag CSS classes | — |
| `drop` listener | ✅ Reads data, computes position, sends to Go | ✅ Receives `(from, to, position)`, calls method, diffs state |
| Group filtering | ✅ MIME type check on `dragover`/`drop` | ✅ Group name encoded in directive (`.group` suffix) |
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

Each item is both draggable (with its index as data) and a drop target. Dropping item 3 onto item 1 calls `Reorder(3, 1, "above")`. The Go method reorders the slice, and godom's list diffing handles the DOM update.

The `g-draggable` binding is re-evaluated on each render, so indices stay correct after reordering. This is important — after moving item 3 to position 1, the old item 1 is now at position 2, and its `g-draggable` value updates accordingly.

## The form builder as proof

The `basic-form-builder` example exercises all of this:

- **Palette → canvas** (`g-draggable:palette` / `g-drop:palette`): Drag field types from a palette, drop onto the canvas to add them. Different groups prevent palette items from being sortable within the palette.
- **Canvas reordering** (`g-draggable:canvas` / `g-drop:canvas`): Drag canvas fields to reorder them within the form.
- **Canvas → trash** (`g-drop:canvas` on a trash zone): Drag a canvas field to the trash to remove it.
- **CSS feedback**: `.g-dragging` dims the source, `.g-drag-over` highlights the canvas, `.g-drag-over-above`/`.g-drag-over-below` show insertion lines on sortable items.

Three groups (`palette`, `canvas`, and ungrouped), three drop handlers (`AddField`, `Reorder`, `RemoveField`), zero JavaScript.
