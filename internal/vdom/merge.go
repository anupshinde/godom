package vdom

// MergeTree updates dst in place with data from src, keeping dst's node IDs.
// Structurally matching nodes (same type, same tag) get their data updated.
// Non-matching nodes at the same position are replaced with src nodes.
//
// Returns a map from src node IDs → dst node IDs for every position where
// dst's ID was kept (canMerge=true and IDs differ). Callers use this to
// remap bindings that reference src IDs to the IDs the merged tree has.
//
// Call this after Diff(dst, src) to bring dst in sync with what the browser
// will have after patches are applied. dst is never replaced — it is the
// long-lived tree that persists across renders.
func MergeTree(dst, src Node) map[int]int {
	remap := make(map[int]int)
	mergeTree(dst, src, remap)
	return remap
}

func mergeTree(dst, src Node, remap map[int]int) {
	if dst == nil || src == nil {
		return
	}
	if !canMerge(dst, src) {
		return
	}

	// Record the ID mapping before mutating dst.
	// MergeTree never changes NodeBase.ID, so this is safe at any point,
	// but we do it up front for clarity.
	if dst.NodeID() != src.NodeID() {
		remap[src.NodeID()] = dst.NodeID()
	}

	switch d := dst.(type) {
	case *TextNode:
		s := src.(*TextNode)
		d.Text = s.Text

	case *ElementNode:
		s := src.(*ElementNode)
		if d.Tag != s.Tag || d.Namespace != s.Namespace {
			return
		}
		d.Facts = s.Facts
		mergeChildren(d, s.Children, remap)

	case *KeyedElementNode:
		s := src.(*KeyedElementNode)
		if d.Tag != s.Tag || d.Namespace != s.Namespace {
			return
		}
		d.Facts = s.Facts
		mergeKeyedChildren(d, s.Children, remap)

	case *PluginNode:
		s := src.(*PluginNode)
		if d.Tag != s.Tag || d.Name != s.Name {
			return
		}
		d.Facts = s.Facts
		d.Data = s.Data

	case *LazyNode:
		s := src.(*LazyNode)
		d.Func = s.Func
		d.Args = s.Args
		if d.Cached != nil && s.Cached != nil {
			mergeTree(d.Cached, s.Cached, remap)
		} else {
			d.Cached = s.Cached
		}
	}
}

func mergeChildren(dst *ElementNode, srcKids []Node, remap map[int]int) {
	minLen := len(dst.Children)
	if len(srcKids) < minLen {
		minLen = len(srcKids)
	}

	for i := 0; i < minLen; i++ {
		if canMerge(dst.Children[i], srcKids[i]) {
			mergeTree(dst.Children[i], srcKids[i], remap)
		} else {
			MarkRemoved(dst.Children[i])
			dst.Children[i] = srcKids[i]
		}
	}

	// Append new children from src.
	if len(srcKids) > len(dst.Children) {
		dst.Children = append(dst.Children, srcKids[len(dst.Children):]...)
	}

	// Truncate removed children.
	if len(dst.Children) > len(srcKids) {
		for i := len(srcKids); i < len(dst.Children); i++ {
			MarkRemoved(dst.Children[i])
		}
		dst.Children = dst.Children[:len(srcKids)]
	}
}

func mergeKeyedChildren(dst *KeyedElementNode, srcKids []KeyedChild, remap map[int]int) {
	// Build old key → index map.
	oldByKey := make(map[string]int, len(dst.Children))
	for i, kc := range dst.Children {
		oldByKey[kc.Key] = i
	}

	// Build new children list, reusing old nodes where keys match.
	merged := make([]KeyedChild, len(srcKids))
	usedOld := make(map[int]bool, len(srcKids))
	for i, sk := range srcKids {
		if oi, ok := oldByKey[sk.Key]; ok {
			usedOld[oi] = true
			// Key exists in old — merge data into old node, keep old ID.
			oldNode := dst.Children[oi].Node
			if canMerge(oldNode, sk.Node) {
				mergeTree(oldNode, sk.Node, remap)
				merged[i] = KeyedChild{Key: sk.Key, Node: oldNode}
			} else {
				MarkRemoved(oldNode)
				merged[i] = sk
			}
		} else {
			// New key — use src node directly.
			merged[i] = sk
		}
	}
	// Mark old keyed children that were not reused as removed.
	for i, kc := range dst.Children {
		if !usedOld[i] {
			MarkRemoved(kc.Node)
		}
	}
	dst.Children = merged
}

// canMerge checks if two nodes are structurally compatible for merging
// (same type and same tag/name).
func canMerge(dst, src Node) bool {
	if dst.NodeType() != src.NodeType() {
		return false
	}
	switch d := dst.(type) {
	case *ElementNode:
		s := src.(*ElementNode)
		return d.Tag == s.Tag && d.Namespace == s.Namespace
	case *KeyedElementNode:
		s := src.(*KeyedElementNode)
		return d.Tag == s.Tag && d.Namespace == s.Namespace
	case *PluginNode:
		s := src.(*PluginNode)
		return d.Tag == s.Tag && d.Name == s.Name
	}
	return true // TextNode, LazyNode — always mergeable
}
