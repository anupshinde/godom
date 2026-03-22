# Nested g-for — Design & Implementation

## The problem

godom's `g-for` loops render lists by parsing the loop element into a template at startup and resolving it against each item at render time. This needs to support nesting — a `g-for` inside another `g-for`:

```html
<div g-for="group in Groups">
    <h3 g-text="group.Name"></h3>
    <option g-for="opt in group.Options" g-text="opt"></option>
</div>
```

The inner `g-for` iterates over a field of the outer loop variable.

## Why other frameworks handle this naturally

Frameworks like Vue, Svelte, and Alpine process templates at render time in the browser. When the outer loop renders an item, the browser-side engine encounters the inner `g-for` and processes it. Nesting works because template processing is recursive and happens where the DOM lives.

godom is different: templates are parsed at startup in Go, and the browser-side bridge doesn't evaluate directives. The bridge receives a tree description on init and minimal patches on updates — it doesn't know what `g-for` means.

This is a deliberate design choice (see [architecture.md](architecture.md)). The bridge's simplicity is a feature — all logic is testable in Go, the bridge stays in sync with framework semantics, and debugging stays in one language. But it means nested loops need to be handled entirely on the Go side.

## The solution

The VDOM template system handles nesting naturally through recursive `TemplateNode` structures. The template parser extracts nested `g-for` elements during parsing, and the tree resolver expands them recursively during render.

### Parse time (internal/vdom/tree.go)

The `TemplateNode` struct represents `g-for` loops:

```go
type TemplateNode struct {
    IsFor    bool
    ForItem  string           // loop variable name: "item"
    ForIndex string           // index variable name: "i" (empty if unused)
    ForList  string           // list field: "Items"
    ForKey   string           // key expression: "item.ID" (empty = positional)
    ForBody  []*TemplateNode  // template for each iteration
    // ...
}
```

When `ParseTemplate` encounters a `g-for` attribute, it:

1. Extracts the loop expression (e.g., `opt in group.Options`)
2. Parses the element's children into the `ForBody` template
3. Any inner `g-for` elements become nested `TemplateNode` entries in the body — recursively supporting arbitrary nesting depth

### Resolve time (internal/vdom/tree.go)

When `ResolveTree` encounters a `g-for` template node, it:

1. Resolves the list field from the current context (e.g., `group.Options` looks up the `group` loop variable, then resolves `Options` on it)
2. For each item in the list, creates a child `ResolveContext` with the loop variable and optional index variable added to `Vars`
3. Recursively resolves `ForBody` templates against the child context
4. Each resolved node gets a unique stable ID from the `IDCounter`

For nested loops, the inner `g-for` template is encountered during resolution of the outer loop's body, and the process recurses naturally — the inner context inherits the outer loop variable.

### Example resolution

For this template:

```html
<div g-for="group in Groups">
    <h3 g-text="group.Name"></h3>
    <option g-for="opt in group.Options" g-text="opt"></option>
</div>
```

With state `Groups = [{Name: "Colors", Options: ["Red", "Blue"]}, {Name: "Sizes", Options: ["S", "M"]}]`:

1. Outer loop iterates over `Groups`
2. For group 0: `group = {Name: "Colors", Options: ["Red", "Blue"]}`
   - `<h3>` resolved with text "Colors"
   - Inner loop iterates over `group.Options` → `["Red", "Blue"]`
   - Two `<option>` nodes: "Red", "Blue"
3. For group 1: `group = {Name: "Sizes", Options: ["S", "M"]}`
   - `<h3>` resolved with text "Sizes"
   - Inner loop iterates over `group.Options` → `["S", "M"]`
   - Two `<option>` nodes: "S", "M"

Each node gets a unique stable ID, and the differ handles updates normally.

### Validation (internal/template/validate.go)

`validateForExpr` accepts dotted paths through loop variables. For `g-for="opt in group.Options"`, `group` is a known loop variable from the outer `g-for`, so the expression is valid even though `group.Options` is not a top-level struct field.

## What this enables

Nested `g-for` unlocks patterns that were previously impossible:

- **Select dropdowns** with dynamic options from a list of structs
- **Checkbox groups** where each form field has its own set of choices
- **Grouped lists** — categories containing items
- **Tree structures** — nested to arbitrary depth

The `basic-form-builder` example uses nested `g-for` for both select options and checkbox groups in preview mode:

```html
<!-- Inside g-for="field, i in Fields" -->
<select>
    <option value="">Select an option...</option>
    <option g-for="opt in field.Options" g-text="opt"></option>
</select>
```
