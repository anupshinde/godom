# vdom — Virtual DOM in Go

A pure-Go virtual DOM implementation with tree construction, diffing, and patch generation. Used internally by [godom](https://github.com/anupshinde/godom) but designed as a self-contained package.

## What it does

This package lets you:

1. **Build virtual DOM trees** from Go structs (`TextNode`, `ElementNode`, `KeyedElementNode`, etc.)
2. **Parse HTML templates** with directives (`g-if`, `g-for`, `g-click`, etc.) into reusable template trees
3. **Resolve templates** against Go struct state to produce concrete VNode trees with stable node IDs
4. **Diff two VNode trees** to compute the minimal set of patches needed to transform one into the other
5. **Inspect patches** to apply them to any rendering target (browser DOM, terminal, test assertions, etc.)

---

## Core concepts

### Node interface

Every virtual DOM node implements:

```go
type Node interface {
    NodeType() int           // which node variant (text, element, keyed, etc.)
    NodeID() int             // stable identity assigned during ResolveTree()
    DescendantsCount() int   // total descendant count (cached by ComputeDescendants)
}
```

### NodeBase

All concrete node types embed `NodeBase`, which provides the stable identity and descendant count:

```go
type NodeBase struct {
    ID          int // stable identity, assigned during ResolveTree(), used to address the node
    Descendants int // cached count, set by ComputeDescendants
}
```

IDs are assigned by a monotonic `IDCounter` during `ResolveTree()`. The counter never resets across renders, ensuring existing IDs remain valid in the rendering engine's node map.

### Node types

| Go type | Constant | Description |
|---------|----------|-------------|
| `*TextNode` | `NodeText` | Leaf node containing plain text |
| `*ElementNode` | `NodeElement` | HTML/SVG element with a tag, facts, and ordered children |
| `*KeyedElementNode` | `NodeKeyed` | Like `ElementNode` but children have stable string keys for efficient reordering |
| `*PluginNode` | `NodePlugin` | An opaque node whose rendering is delegated to an external system (e.g. a JS library) |
| `*LazyNode` | `NodeLazy` | Deferred computation — if the function and args haven't changed, the entire subtree is skipped |

### Facts (element metadata)

`Facts` groups everything about an element that isn't its tag or children:

```go
type Facts struct {
    Props   map[string]any          // DOM properties: className, value, checked, id
    Attrs   map[string]string       // HTML attributes: data-*, aria-*, role, etc.
    AttrsNS map[string]NSAttr       // Namespaced attributes (SVG): xlink:href, xml:lang
    Styles  map[string]string       // Inline CSS: background-color, width, etc.
    Events  map[string]EventHandler // Event listeners: click, input, keydown, etc.
}
```

Why group them? Because the diff algorithm can diff all of them in one pass (`DiffFacts()`) and produce a single `FactsDiff` with only the changed/added/removed entries.

### EventHandler

```go
type EventHandler struct {
    Handler string       // method name to call (e.g. "Save", "Toggle")
    Args    []any        // pre-resolved arguments
    Scope   string       // routing info for child components (e.g. "g3:2")
    Options EventOptions // key filter, stopPropagation, preventDefault
}

type EventOptions struct {
    StopPropagation bool
    PreventDefault  bool
    Key             string // key filter for keydown events
}
```

Events are declarative — the vdom tree says "when this element is clicked, call `Save` with these args." The runtime decides how to wire that up.

---

## Building trees by hand

You can construct VNode trees directly without using the template parser:

```go
package main

import "github.com/anupshinde/godom/internal/vdom"

func main() {
    // A simple tree: <div class="app"><p>Hello</p><p>World</p></div>
    tree := &vdom.ElementNode{
        NodeBase: vdom.NodeBase{ID: 1},
        Tag:      "div",
        Facts: vdom.Facts{
            Props: map[string]any{"className": "app"},
        },
        Children: []vdom.Node{
            &vdom.ElementNode{
                NodeBase: vdom.NodeBase{ID: 2},
                Tag:      "p",
                Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 3}, Text: "Hello"}},
            },
            &vdom.ElementNode{
                NodeBase: vdom.NodeBase{ID: 4},
                Tag:      "p",
                Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 5}, Text: "World"}},
            },
        },
    }

    // IMPORTANT: compute descendant counts before diffing
    vdom.ComputeDescendants(tree)
}
```

When using the template system, IDs are assigned automatically by `ResolveTree()` via the `IDCounter` in `ResolveContext`. When building trees by hand, you assign IDs manually.

### Keyed children (for lists)

When children have stable identifiers, use `KeyedElementNode` for efficient reordering:

```go
list := &vdom.KeyedElementNode{
    NodeBase: vdom.NodeBase{ID: 10},
    Tag:      "ul",
    Children: []vdom.KeyedChild{
        {Key: "id-1", Node: &vdom.ElementNode{NodeBase: vdom.NodeBase{ID: 11}, Tag: "li", Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 12}, Text: "Alice"}}}},
        {Key: "id-2", Node: &vdom.ElementNode{NodeBase: vdom.NodeBase{ID: 13}, Tag: "li", Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 14}, Text: "Bob"}}}},
        {Key: "id-3", Node: &vdom.ElementNode{NodeBase: vdom.NodeBase{ID: 15}, Tag: "li", Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 16}, Text: "Carol"}}}},
    },
}
```

When you reorder, insert, or remove keyed children, the differ detects exactly what moved and produces `PatchReorder` instead of redrawing everything.

### Lazy nodes (skip unchanged subtrees)

```go
renderSidebar := func(items []string) vdom.Node {
    // ... expensive tree construction ...
    return &vdom.ElementNode{Tag: "aside", Children: /* ... */}
}

lazy := &vdom.LazyNode{
    NodeBase: vdom.NodeBase{ID: 20},
    Func:     renderSidebar,
    Args:     []any{items}, // compared by reference equality
}
```

If `Func` pointer and all `Args` are reference-equal to the previous render, the entire subtree is skipped — zero computation.

### Plugin nodes (external rendering)

```go
chart := &vdom.PluginNode{
    NodeBase: vdom.NodeBase{ID: 30},
    Tag:      "canvas",             // host element
    Name:     "chartjs",            // plugin identifier
    Facts: vdom.Facts{
        Attrs: map[string]string{"width": "400", "height": "300"},
    },
    Data: map[string]any{       // JSON-serializable data for the plugin
        "type": "bar",
        "data": chartData,
    },
}
```

The differ JSON-compares `Data` — if it changed, it emits a `PatchPlugin` with the new data.

---

## Diffing two trees

```go
// Build the old tree (with IDs — normally assigned by ResolveTree)
oldTree := &vdom.ElementNode{
    NodeBase: vdom.NodeBase{ID: 1},
    Tag:      "div",
    Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 2}, Text: "Hello"}},
}
vdom.ComputeDescendants(oldTree)

// Build the new tree (text changed, fresh IDs)
newTree := &vdom.ElementNode{
    NodeBase: vdom.NodeBase{ID: 3},
    Tag:      "div",
    Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 4}, Text: "Goodbye"}},
}
vdom.ComputeDescendants(newTree)

// Diff
patches := vdom.Diff(oldTree, newTree)

// patches will contain one entry:
//   Patch{Type: PatchText, NodeID: 2, Data: PatchTextData{Text: "Goodbye"}}
//
// NodeID 2 is the old text node's ID — the rendering engine looks up this ID
// in its node map to find the DOM node to update.
```

### The Patch struct

```go
type Patch struct {
    Type   int // one of the Patch* constants
    NodeID int // ID of the target node in the OLD tree
    Data   any // type-specific payload (see below)
}
```

The differ always uses the **old tree's** node IDs for patches, because those are the IDs the rendering engine already has in its node map.

### Patch types

| Constant | Payload type | When emitted |
|----------|-------------|--------------|
| `PatchRedraw` | `PatchRedrawData{Node}` | Node type changed or element tag/namespace changed |
| `PatchText` | `PatchTextData{Text}` | Text node content changed |
| `PatchFacts` | `PatchFactsData{Diff}` | Properties, attributes, styles, or events changed |
| `PatchAppend` | `PatchAppendData{Nodes}` | New children added at the end |
| `PatchRemoveLast` | `PatchRemoveLastData{Count}` | N children removed from the end |
| `PatchReorder` | `PatchReorderData{Inserts, Removes, Patches}` | Keyed children inserted, removed, or moved |
| `PatchPlugin` | `PatchPluginData{Data}` | Plugin data changed (JSON comparison) |
| `PatchLazy` | `PatchLazyData{Patches}` | Wrapper for patches inside a lazy node's subtree |

### FactsDiff

When element metadata changes, the diff contains only what changed:

```go
type FactsDiff struct {
    Props   map[string]any           // changed/added props (nil value = removed)
    Attrs   map[string]string        // changed/added attrs ("" = removed)
    AttrsNS map[string]NSAttr        // changed/added namespaced attrs
    Styles  map[string]string        // changed/added styles ("" = removed)
    Events  map[string]*EventHandler // changed/added events (nil = removed)
}
```

Example: if only `className` changed from `"active"` to `"inactive"`, the `FactsDiff` will be:

```go
FactsDiff{
    Props: map[string]any{"className": "inactive"},
    // Attrs, Styles, Events are all nil — unchanged
}
```

### Stable identity addressing

Patches reference their target node by a **stable node ID** (`NodeID`). Each node gets a unique ID assigned during `ResolveTree()` via a monotonic `IDCounter` that never resets across renders. The differ uses the **old tree's** IDs because those are the IDs the rendering engine (bridge) already knows.

New nodes in the new tree get fresh IDs. When the bridge receives a patch, it looks up the target DOM node using `nodeMap[nodeID]`.

**Important**: You must call `ComputeDescendants()` on both trees before calling `Diff()`.

---

## Template system

The template system parses HTML with `g-*` directives into a reusable template tree, then resolves it against Go struct state on each render cycle.

### Parsing

```go
html := `<div>
    <h1 g-text="Title"></h1>
    <p>Welcome, {{Name}}!</p>
    <ul>
        <li g-for="item in Items" g-key="item.ID">
            <span g-text="item.Text"></span>
            <button g-click="Remove(item.ID)">Delete</button>
        </li>
    </ul>
    <p g-if="ShowFooter">That's all!</p>
</div>`

templates, err := vdom.ParseTemplate(html)
```

`ParseTemplate()` returns `[]*TemplateNode` — a tree of template nodes that can be resolved repeatedly against different state values. This is parsed once and reused.

### Template node structure

```go
type TemplateNode struct {
    // Element nodes
    Tag        string
    Namespace  string           // "http://www.w3.org/2000/svg" for SVG elements
    Attrs      []html.Attribute // static HTML attributes (non-directive)
    Directives []Directive      // extracted g-* directives
    Children   []*TemplateNode

    // Text nodes
    IsText    bool
    TextParts []TextPart // mix of static text and {{expr}} interpolations

    // g-for loop nodes
    IsFor    bool
    ForItem  string           // loop variable name: "item"
    ForIndex string           // index variable name: "i" (empty if unused)
    ForList  string           // list field: "Items"
    ForKey   string           // key expression: "item.ID" (empty = positional)
    ForBody  []*TemplateNode  // template for each iteration

    // Plugin nodes
    IsPlugin   bool
    PluginName string
    PluginExpr string
}
```

### Directives

Directives are extracted from `g-*` attributes during parsing:

```go
type Directive struct {
    Type string // "text", "bind", "value", "checked", "if", "show", "hide", "class", "attr", "style",
               // "click", "keydown", "mousedown", "mousemove", "mouseup", "wheel",
               // "drop", "draggable", "dropzone"
    Name string // modifier: class name, style property, key filter, drag group, etc.
    Expr string // expression: field name, method call, boolean literal, etc.
}
```

| Directive | HTML syntax | Effect on resolved tree |
|-----------|-------------|------------------------|
| `g-text` | `g-text="Name"` | Sets the element's children to a single text node with the resolved value |
| `g-bind` | `g-bind="Input"` | Sets `value` prop + adds an `input` event handler for two-way binding |
| `g-checked` | `g-checked="Done"` | Sets the `checked` prop to true/false |
| `g-if` | `g-if="ShowPanel"` | If falsy, the entire node is excluded from the resolved tree |
| `g-show` | `g-show="Visible"` | If falsy, adds `display: none` style (node stays in tree) |
| `g-class:name` | `g-class:active="IsActive"` | Conditionally appends a CSS class to `className` |
| `g-attr:name` | `g-attr:data-id="ID"` | Sets an HTML attribute to the resolved value |
| `g-style:prop` | `g-style:color="TextColor"` | Sets an inline CSS property |
| `g-click` | `g-click="Save"` | Adds a click event handler |
| `g-keydown` | `g-keydown="Enter:Submit"` | Adds a keydown handler with optional key filter |
| `g-mousedown` | `g-mousedown="Start"` | Mouse button event (passes coordinates) |
| `g-mousemove` | `g-mousemove="OnMove"` | Mouse move event (throttled to rAF in the browser) |
| `g-mouseup` | `g-mouseup="End"` | Mouse button release event |
| `g-wheel` | `g-wheel="OnScroll"` | Wheel/scroll event (passes deltaY) |
| `g-for` | `g-for="todo in Todos"` | Loop: renders the element body once per slice item |
| `g-key` | `g-key="todo.ID"` | Stable key for `g-for` list diffing (avoids positional redraw) |
| `g-draggable` | `g-draggable="ID"` | Makes the element draggable with a payload value |
| `g-draggable:group` | `g-draggable:tasks="ID"` | Draggable within a named group |
| `g-dropzone` | `g-dropzone="OnDrop"` | Registers a drop target |
| `g-drop` | `g-drop="HandleDrop"` | Drop event handler |
| `g-plugin:name` | `g-plugin:chart="Data"` | Delegates rendering to a named plugin |

### Resolving templates against state

```go
type MyApp struct {
    Title string
    Name  string
    Items []Item
    ShowFooter bool
}

type Item struct {
    ID   int
    Text string
}

app := &MyApp{
    Title: "My List",
    Name:  "Alice",
    Items: []Item{{ID: 1, Text: "Buy milk"}, {ID: 2, Text: "Write code"}},
    ShowFooter: true,
}

// IDCounter must persist across renders — never reset it.
ids := &vdom.IDCounter{}

ctx := &vdom.ResolveContext{
    State: reflect.ValueOf(app).Elem(), // must be the struct value (not pointer)
    Vars:  map[string]any{},            // loop variables (empty at top level)
    IDs:   ids,                         // assigns unique IDs to each node
}

nodes := vdom.ResolveTree(templates, ctx)
```

This produces a concrete `[]Node` tree where:
- Every node has a unique `ID` from the `IDCounter`
- `{{Name}}` is replaced with `"Alice"`
- `g-text="Title"` creates a text child node `"My List"`
- `g-for="item in Items"` is unrolled into two `<li>` elements
- `g-if="ShowFooter"` includes the `<p>` node (since `ShowFooter` is true)
- `g-click="Remove(item.ID)"` creates an `EventHandler` with the resolved ID value

### Expression resolution

`ResolveExpr(expr, ctx)` resolves expressions in this order:

1. **Boolean literals**: `"true"` → `true`, `"false"` → `false`
2. **Loop variables**: If `expr` matches a key in `ctx.Vars`, return that value
3. **Dotted paths on loop variables**: `"todo.Text"` → look up `todo` in `Vars`, then resolve `Text` field via reflection
4. **Struct fields**: Fall back to `ctx.State.FieldByName(expr)` — supports dotted paths like `"Address.City"`

### Text interpolation

Text like `"Hello, {{Name}}! You have {{Count}} items."` is parsed into parts:

```go
parts := vdom.ParseTextInterpolations("Hello, {{Name}}! You have {{Count}} items.")
// Result:
// []TextPart{
//     {Static: true,  Value: "Hello, "},
//     {Static: false, Value: "Name"},
//     {Static: true,  Value: "! You have "},
//     {Static: false, Value: "Count"},
//     {Static: true,  Value: " items."},
// }
```

During resolution, non-static parts are evaluated via `ResolveExpr()`.

### Loop expression parsing

```go
item, index, list := vdom.ParseForExpr("todo, i in Todos")
// item="todo", index="i", list="Todos"

item, index, list = vdom.ParseForExpr("item in Items")
// item="item", index="", list="Items"
```

### Method call parsing

```go
method, args := vdom.ParseMethodCall("Remove(i, todo.ID)")
// method="Remove", args=["i", "todo.ID"]

method, args = vdom.ParseMethodCall("Save")
// method="Save", args=nil
```

---

## Full example: parse, resolve, diff

```go
package main

import (
    "fmt"
    "reflect"

    "github.com/anupshinde/godom/internal/vdom"
)

type Counter struct {
    Count int
}

func main() {
    html := `<div><p>Count: {{Count}}</p></div>`

    templates, _ := vdom.ParseTemplate(html)

    // ID counter persists across renders — never reset.
    ids := &vdom.IDCounter{}

    // --- First render ---
    state1 := &Counter{Count: 0}
    ctx1 := &vdom.ResolveContext{
        State: reflect.ValueOf(state1).Elem(),
        Vars:  map[string]any{},
        IDs:   ids,
    }
    tree1 := vdom.ResolveTree(templates, ctx1)
    root1 := &vdom.ElementNode{Tag: "body", Children: tree1}
    vdom.ComputeDescendants(root1)

    // --- Second render (count changed) ---
    state2 := &Counter{Count: 5}
    ctx2 := &vdom.ResolveContext{
        State: reflect.ValueOf(state2).Elem(),
        Vars:  map[string]any{},
        IDs:   ids, // same counter — new nodes get fresh IDs
    }
    tree2 := vdom.ResolveTree(templates, ctx2)
    root2 := &vdom.ElementNode{Tag: "body", Children: tree2}
    vdom.ComputeDescendants(root2)

    // --- Diff ---
    patches := vdom.Diff(root1, root2)

    for _, p := range patches {
        switch p.Type {
        case vdom.PatchText:
            d := p.Data.(vdom.PatchTextData)
            fmt.Printf("Text change at node %d: %q\n", p.NodeID, d.Text)
        case vdom.PatchFacts:
            fmt.Printf("Facts change at node %d\n", p.NodeID)
        case vdom.PatchRedraw:
            fmt.Printf("Redraw at node %d\n", p.NodeID)
        case vdom.PatchAppend:
            d := p.Data.(vdom.PatchAppendData)
            fmt.Printf("Append %d children at node %d\n", len(d.Nodes), p.NodeID)
        case vdom.PatchRemoveLast:
            d := p.Data.(vdom.PatchRemoveLastData)
            fmt.Printf("Remove last %d children at node %d\n", d.Count, p.NodeID)
        }
    }
    // Output: Text change at node 3: "Count: 5"
}
```

---

## Truthiness

`IsTruthy()` determines whether a value is "truthy" for `g-if`, `g-show`, and `g-class`:

| Type | Falsy when |
|------|-----------|
| `nil` | always |
| `bool` | `false` |
| `int`, `int64`, `float64` | `0` |
| `string` | `""` |
| `slice`, `map` | `Len() == 0` |
| everything else | never (always truthy) |

---

## Helper functions

| Function | Purpose |
|----------|---------|
| `ComputeDescendants(node)` | Recursively calculates and caches descendant counts. **Must be called before `Diff()`**. |
| `MergeAdjacentText(nodes)` | Collapses consecutive `TextNode`s into one and drops empty text nodes. Called automatically by `ResolveTree()`. |
| `CopyVars(vars)` | Shallow-copies a variable map (used internally by `g-for` to create child contexts). |
| `DeepCopyJSON(v)` | Deep-copies a value via JSON round-trip (used for plugin data isolation). |

---

## File layout

```
internal/vdom/
├── node.go        Node interface, NodeBase, all node types, Facts, EventHandler, ComputeDescendants
├── tree.go        IDCounter, ResolveContext, ParseTemplate(), ResolveTree(), ResolveExpr(),
│                  text interpolation, for-loop parsing, method call parsing, IsTruthy, helpers
├── diff.go        Diff(), DiffFacts(), keyed diff algorithm, equality helpers
├── patch.go       Patch struct, FactsDiff, all patch payload structs
├── merge.go       MergeTree(), MergeAdjacentText() — tree merging utilities
├── node_test.go   Tests for node types and ComputeDescendants
├── tree_test.go   Tests for parsing, resolution, text interpolation, for expressions
├── diff_test.go   Tests for diffing, keyed diffing, facts diffing, NodeID targeting
└── merge_test.go  Tests for tree merging and adjacent text merging
```

---

## Design decisions

### No virtual DOM reconciliation
godom uses positional diffing for non-keyed children and key-based matching for keyed children. There is no heuristic tree matching (React-style) because the template structure is static — parsed once at startup — so the old and new trees always have the same shape. Only the data changes.

### Stable identity patch addressing
Patches reference nodes by stable node ID rather than positional index. Each node gets a unique ID during `ResolveTree()` from a monotonic counter that persists across renders. The differ uses the old tree's IDs because those are what the rendering engine already has in its node map. This avoids the fragility of DFS-index addressing where tree mutations invalidate subsequent indices.

### Facts as a unified concept
Grouping props, attrs, namespaced attrs, styles, and events into one `Facts` struct means the differ handles all of them in a single `DiffFacts()` call. The result is a `FactsDiff` with only the changes, which maps directly to the browser's DOM API (set property, set attribute, set style, add/remove event listener).

### Lazy nodes use reference equality
`LazyNode` compares the function pointer and args by reference (`reflect.ValueOf().Pointer()`), not by value. If nothing changed by reference, the subtree is skipped entirely with zero work. This is the primary optimization for large trees with mostly-static sections.

### Template tree is immutable after parse
`ParseTemplate()` runs once. Every render cycle resolves the same template tree against new state, which is fast because the structure is known and fixed — only expressions are evaluated.

### Adjacent text nodes are merged
After resolving `g-for` loops (which can produce empty text nodes from whitespace) and `g-if` conditionals (which can remove nodes between text), `MergeAdjacentText()` collapses consecutive text nodes. This prevents the differ from generating spurious text patches for whitespace-only nodes.
