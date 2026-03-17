package godom

// Patch types — the output of diffing two virtual DOM trees.
// Each patch targets a specific node by its traversal index and
// describes the minimal DOM mutation needed.

// Patch operation constants.
const (
	patchRedraw     = iota // replace entire node
	patchText              // update text content
	patchFacts             // update properties/attributes/styles/events
	patchAppend            // add children at end
	patchRemoveLast        // remove N children from end
	patchRemove            // remove specific child (keyed)
	patchReorder           // reorder keyed children (inserts/removes/moves)
	patchPlugin            // plugin data changed
	patchLazy              // wrapper for patches inside a lazy node
)

// Patch describes a single DOM mutation produced by the diff algorithm.
type Patch struct {
	Type  int // one of the patch* constants
	Index int // position in depth-first tree traversal
	Data  any // type-specific payload (see below)
}

// ---------------------------------------------------------------------------
// Patch payloads — the Data field holds one of these depending on Type.
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
	Patches []Patch // sub-patches to apply before removal (if node is being moved)
}

// PatchReorderData carries the keyed reorder operation.
type PatchReorderData struct {
	Inserts []ReorderInsert
	Removes []ReorderRemove
	// Sub-patches for children that changed content (applied after reorder).
	Patches []Patch
}

// ReorderInsert describes a node to insert at a given position.
type ReorderInsert struct {
	Index int    // position to insert at
	Key   string // key of the node
	Node  Node   // the node to insert (nil if it's a move from Removes)
}

// ReorderRemove describes a node to remove (or move) during reorder.
type ReorderRemove struct {
	Index int    // current position
	Key   string // key of the node
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
// FactsDiff — the diff between two Facts structs.
// Only non-nil maps contain changes. Within each map:
//   - present key = add or update to that value
//   - value is nil/empty = remove
// ---------------------------------------------------------------------------

// FactsDiff represents changes between two Facts.
type FactsDiff struct {
	Props   map[string]any        // changed/added/removed properties
	Attrs   map[string]string     // changed/added/removed attributes ("" = remove)
	AttrsNS map[string]NSAttr     // changed/added/removed namespaced attributes
	Styles  map[string]string     // changed/added/removed styles ("" = remove)
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
