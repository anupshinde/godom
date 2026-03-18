package vdom

// Patch types — the output of diffing two virtual DOM trees.

// Patch operation constants.
const (
	PatchRedraw     = iota // replace entire node
	PatchText              // update text content
	PatchFacts             // update properties/attributes/styles/events
	PatchAppend            // add children at end
	PatchRemoveLast        // remove N children from end
	PatchRemove            // remove specific child (keyed)
	PatchReorder           // reorder keyed children (inserts/removes/moves)
	PatchPlugin            // plugin data changed
	PatchLazy              // wrapper for patches inside a lazy node
)

// Patch describes a single DOM mutation produced by the diff algorithm.
type Patch struct {
	Type  int // one of the Patch* constants
	Index int // position in depth-first tree traversal
	Data  any // type-specific payload (see below)
}

// ---------------------------------------------------------------------------
// Patch payloads
// ---------------------------------------------------------------------------

// PatchRedrawData carries the new node to render from scratch.
type PatchRedrawData struct {
	Node Node
}

// PatchTextData carries the new text content.
type PatchTextData struct {
	Text string
}

// PatchFactsData carries the diff between old and new facts.
type PatchFactsData struct {
	Diff FactsDiff
}

// PatchAppendData carries new children to append.
type PatchAppendData struct {
	Nodes []Node
}

// PatchRemoveLastData carries the number of children to remove from the end.
type PatchRemoveLastData struct {
	Count int
}

// PatchRemoveData carries info for removing a specific keyed child.
type PatchRemoveData struct {
	Key     string
	Patches []Patch
}

// PatchReorderData carries the keyed reorder operation.
type PatchReorderData struct {
	Inserts []ReorderInsert
	Removes []ReorderRemove
	Patches []Patch
}

// ReorderInsert describes a node to insert at a given position.
type ReorderInsert struct {
	Index int
	Key   string
	Node  Node
}

// ReorderRemove describes a node to remove (or move) during reorder.
type ReorderRemove struct {
	Index int
	Key   string
}

// PatchPluginData carries new data for a plugin node.
type PatchPluginData struct {
	Data any
}

// PatchLazyData wraps patches that apply inside a lazy node's cached subtree.
type PatchLazyData struct {
	Patches []Patch
}

// ---------------------------------------------------------------------------
// FactsDiff
// ---------------------------------------------------------------------------

// FactsDiff represents changes between two Facts.
type FactsDiff struct {
	Props   map[string]any           // changed/added/removed properties
	Attrs   map[string]string        // changed/added/removed attributes ("" = remove)
	AttrsNS map[string]NSAttr        // changed/added/removed namespaced attributes
	Styles  map[string]string        // changed/added/removed styles ("" = remove)
	Events  map[string]*EventHandler // changed/added events (nil = remove)
}

// IsEmpty returns true if there are no changes.
func (d *FactsDiff) IsEmpty() bool {
	return len(d.Props) == 0 &&
		len(d.Attrs) == 0 &&
		len(d.AttrsNS) == 0 &&
		len(d.Styles) == 0 &&
		len(d.Events) == 0
}
