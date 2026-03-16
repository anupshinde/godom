# Nested g-for — Design & Implementation

## The problem

godom's `g-for` loops work by extracting the loop element as a static HTML template at parse time, replacing it with anchor comments in the DOM, and rendering items at runtime by filling in `__IDX__` placeholders and resolving bindings. This works well for flat lists, but breaks for nested loops — a `g-for` inside another `g-for`:

```html
<div g-for="group in Groups">
    <h3 g-text="group.Name"></h3>
    <option g-for="opt in group.Options" g-text="opt"></option>
</div>
```

The inner `g-for` is just raw text inside the outer template. Nobody processes it.

## Why other frameworks handle this naturally

Frameworks like Vue, Svelte, and Alpine process templates at render time in the browser. When the outer loop renders an item, the browser-side engine encounters the inner `g-for` and processes it. Nesting works because template processing is recursive and happens where the DOM lives.

godom is different: templates are extracted at parse time in Go, and the browser-side bridge is a thin command executor that doesn't evaluate directives. The bridge receives concrete commands ("set text of element g3-0 to 'hello'") — it doesn't know what `g-for` means.

This is a deliberate design choice (see [architecture.md](architecture.md)). The bridge's simplicity is a feature — all logic is testable in Go, the bridge stays in sync with framework semantics, and debugging stays in one language. But it means nested loops need to be handled entirely on the Go side.

## The solution

Process inner `g-for` elements during parse time, store them as sub-templates on the parent `forTemplate`, and expand them at render time in Go. The bridge just needs to register anchor comments from dynamically inserted HTML.

### Parse time (parser.go)

The `forTemplate` struct gains a `SubLoops` field:

```go
type forTemplate struct {
    GID          string
    ItemVar      string
    IndexVar     string
    ListField    string            // "Todos" or "field.Options"
    TemplateHTML string
    Bindings     []binding
    Events       []eventBinding
    SubLoops     []*forTemplate    // nested g-for loops
    // ...
}
```

When `walkForSubtree` encounters a `g-for` inside an outer template, it calls `processNestedFor`:

1. Extracts the inner `g-for` expression (e.g., `opt in group.Options`)
2. Assigns the inner loop a GID like `g3-__IDX__-2` — containing the outer `__IDX__` placeholder so each outer item gets unique inner anchors
3. Recursively calls `walkForSubtree` on the inner element (supports arbitrary nesting depth)
4. Renders the inner element to HTML as the inner template
5. Replaces the inner element with anchor comments in the outer template
6. Appends the sub-template to `parentFT.SubLoops`

After parsing, the outer template HTML contains inner anchor comments:

```html
<div data-gid="g3-__IDX__">
    <h3 data-gid="g3-__IDX__-0" g-text="group.Name"></h3>
    <!-- g-for:g3-__IDX__-2 --><!-- /g-for:g3-__IDX__-2 -->
</div>
```

And the inner template HTML is stored separately:

```html
<option data-gid="g3-__IDX__-2-__IDX__" g-text="opt"></option>
```

Note the double `__IDX__` — the first is the outer loop index (from the parent GID prefix), the second is the inner loop index.

### Render time (render.go)

When `resolveItemBindings` processes an outer item, it also expands sub-loops:

```go
for _, sub := range ft.SubLoops {
    subCmds := computeSubLoopCmd(sub, state, ctx, idxStr)
    cmds = append(cmds, subCmds)
}
```

`computeSubLoopCmd` renders the inner loop for a single outer item:

1. Resolves the inner GID: replaces `__IDX__` in `sub.GID` with the outer index → `g3-0-2`
2. Resolves the inner list from the outer item's context (e.g., `group.Options` resolves against the `group` loop variable)
3. For each inner item, builds the inner context (inheriting the outer context) and resolves bindings/events
4. Returns a `list` command targeting the resolved inner GID

**GID replacement order matters.** The inner template has GIDs like `g3-__IDX__-2-__IDX__`. The first `__IDX__` is the outer index, the second is the inner index. Both are the same placeholder string, so we can't do a simple `ReplaceAll`. Instead:

1. First replace the `sub.GID` prefix (`g3-__IDX__-2`) with the resolved prefix (`g3-0-2`) — this fixes the outer index
2. Then replace remaining `__IDX__` with the inner index

This produces correct GIDs like `g3-0-2-0`, `g3-0-2-1`, `g3-1-2-0`, etc.

### Bridge (bridge.js)

The bridge needs one change: when `execList` or `execListAppend` inserts HTML containing inner anchor comments, those comments need to be registered in `anchorMap`. Without this, the inner `list` command (e.g., `{op: "list", id: "g3-0-2"}`) can't find its anchors.

A new `indexAnchors(node)` function scans a DOM element for anchor comments and registers them. It's called after inserting each list item's HTML.

### Validation (validate.go)

`validateForExpr` now accepts dotted paths through loop variables. For `g-for="opt in group.Options"`, `group` is a known loop variable from the outer `g-for`, so the expression is valid even though `group.Options` is not a top-level struct field.

`collectLoopVars` resolves element types through loop variable struct types. If `group` has type `FormField` and `FormField` has `Options []string`, then `opt` gets element type `string`.

## What this enables

Nested `g-for` unlocks patterns that were previously impossible:

- **Select dropdowns** with dynamic options from a list of structs
- **Checkbox groups** where each form field has its own set of choices
- **Grouped lists** — categories containing items
- **Tree structures** — nested to arbitrary depth (recursive `SubLoops`)

The `basic-form-builder` example uses nested `g-for` for both select options and checkbox groups in preview mode:

```html
<!-- Inside g-for="field, i in Fields" -->
<select>
    <option value="">Select an option...</option>
    <option g-for="opt in field.Options" g-text="opt"></option>
</select>
```

## Limitations

- **No list diffing for inner loops.** Inner loops are fully re-rendered when the outer item changes. This is fine for typical use (small inner lists), but could be optimized later if needed.
- **prevLists keying.** The `prevLists` map (used for list diffing) is keyed by `ft.GID`, not `ft.ListField`. This was a bug fix — two `g-for` loops over the same field (e.g., builder view and preview view both iterating `Fields`) previously shared diff state, causing the second loop to see "no changes" because the first already updated the snapshot.
