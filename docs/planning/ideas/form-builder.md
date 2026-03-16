# godom — Form Builder

## The idea

A drag-and-drop form builder — the kind of practical utility tool that people actually use. Drag field types from a palette, drop them onto a canvas, rearrange them, configure labels and validation, preview the result. All state in Go, minimal JS.

This is the most straightforward example app on the list, and that's exactly the point. It shows godom is usable for real, everyday tools — not just tech demos and animations.

## Why this is a good candidate now

The solar system example already proved that godom handles mouse interactions cleanly — mousedown, mousemove, mouseup, drag tracking, all running through the Go process via the binary WebSocket with no perceptible lag. A form builder is the same interaction pattern (drag, drop, reorder) applied to a practical use case instead of orbiting planets.

What the solar system exercises:
- Continuous mouse tracking (mousemove at high frequency)
- Drag state management in Go (which element is being dragged, where)
- Visual feedback during drag (element follows cursor)
- Drop resolution (where did the user release)

What the form builder adds on top:
- Drop zones and insertion points (snap to grid, insert between existing fields)
- Reordering within a list (drag field up/down to change order)
- Selection and configuration (click a field to edit its properties)
- Form state that's meaningful (field types, labels, validation rules, layout)

The mouse interaction layer is proven. The form builder just uses it for something useful.

## What the app would look like

### Left panel — Field palette
A list of draggable field types:
- Text input
- Textarea
- Dropdown / select
- Checkbox / radio group
- Date picker
- Number input
- File upload
- Section header / divider

### Center — Form canvas
The form being built. Fields appear here as you drag them from the palette. You can:
- Drag fields to reorder them
- Click a field to select it (highlights, shows config panel)
- Delete a field (click X or press Delete)
- See a live preview of what the form looks like

### Right panel — Field configuration
When a field is selected, configure it:
- Label text
- Placeholder text
- Required yes/no
- Validation rules (min/max length, regex pattern, number range)
- Options list (for dropdowns, radio groups)
- Help text

### Top bar — Form-level controls
- Form title
- Preview mode toggle (switch between builder and rendered form)
- Export (generate HTML, or a JSON schema describing the form)

## State model

All state lives in a Go struct. No hidden browser state.

```go
type FormBuilder struct {
    Title       string
    Fields      []FormField
    Selected    int  // index of selected field, -1 if none
    Dragging    int  // index of field being dragged, -1 if none
    DragOverIdx int  // insertion point during drag
    Preview     bool // preview mode toggle
}

type FormField struct {
    Type        string // "text", "textarea", "select", "checkbox", etc.
    Label       string
    Placeholder string
    Required    bool
    Options     []string // for select, radio, checkbox groups
    Validation  ValidationRules
    HelpText    string
}

type ValidationRules struct {
    MinLength int
    MaxLength int
    Pattern   string
    Min       float64
    Max       float64
}
```

Dragging a field reorders `Fields` in the Go slice. Selecting a field sets `Selected`. Editing a property updates the `FormField` struct. The browser always reflects the Go state — no JS-side form state to sync.

## What this demonstrates about godom

| Aspect | What it shows |
|--------|---------------|
| **Drag and drop** | Mouse event handling (already proven in solar system) applied to a real UI pattern |
| **Complex nested state** | Struct with slices of structs, partial updates via field diffing |
| **Two-way data binding** | Editing field properties in the config panel updates the form canvas in real time |
| **Conditional rendering** | `g-if` for showing/hiding config panel, validation options, preview mode |
| **List rendering** | `g-for` over fields with per-item updates as fields are reordered |
| **Practical utility** | An app someone would actually use, not a tech demo |

## Export possibilities

Once the form is built, the Go process can generate output:

- **HTML** — render the form as a standalone HTML file with basic CSS
- **JSON schema** — a machine-readable description of the form (field types, validation rules) that other tools can consume
- **Go struct** — generate a Go struct definition matching the form fields, for use in a godom app that consumes the form

The export happens entirely in Go — template rendering, JSON marshaling, code generation. No browser involvement.

## Status — Implemented

Implemented as `examples/basic-form-builder/`. The basic version covers:

- ✅ Palette → canvas drag-and-drop (using `g-draggable.palette` / `g-drop.palette` groups)
- ✅ Canvas reordering (using `g-draggable.canvas` / `g-drop.canvas` groups)
- ✅ Trash zone for field removal
- ✅ Click-to-select with config panel (label, placeholder, required, help text, options)
- ✅ Preview mode toggle with type-specific rendering
- ✅ JSON export
- ✅ Nested `g-for` for select options and checkbox groups in preview
- ✅ Seven field types: text, textarea, select, checkbox, number, date, section header

What was deferred from the original design:
- Validation rules (min/max length, regex, number range) — not needed for the basic version
- File upload field type
- HTML/Go struct export — only JSON export implemented

Building this example drove the implementation of nested `g-for` support in the framework (see [../nested-for.md](../nested-for.md)).

### Lessons learned

1. **godom doesn't support expression comparisons** — can't do `g-show="field.Type == 'text'"`. Workaround: boolean type flags on the struct (`IsText`, `IsSelect`, etc.). This is a deliberate simplicity choice, not a bug.

2. **g-show negation doesn't work as expected** — godom's init render sends explicit `display` commands for all `g-show` bindings, so "default visible, hide when truthy" breaks. Workaround: use two explicit boolean fields that are inverses of each other (`HasFields` / `ShowEmpty`), both with `style="display:none"`.

3. **g-bind only works with top-level struct fields** — can't bind directly to `Fields[Selected].Label`. Workaround: `Cfg*` fields on the root struct, synced lazily to the selected field via `applyConfig()`.

4. **prevLists was keyed by field name** — two `g-for` loops over the same field (builder and preview both iterating `Fields`) shared diff state. Fixed by keying `prevLists` by `ft.GID` instead of `ft.ListField`.
