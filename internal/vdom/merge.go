package vdom

// MergeTree updates dst in place with data from src, keeping dst's node IDs.
// Structurally matching nodes (same type, same tag) get their data updated.
// Non-matching nodes at the same position are replaced with src nodes.
//
// Call this after Diff(dst, src) to bring dst in sync with what the browser
// will have after patches are applied. dst is never replaced — it is the
// long-lived tree that persists across renders.
func MergeTree(dst, src Node) {
	if dst == nil || src == nil {
		return
	}
	if dst.NodeType() != src.NodeType() {
		return
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
		mergeChildren(d, s.Children)

	case *KeyedElementNode:
		s := src.(*KeyedElementNode)
		if d.Tag != s.Tag || d.Namespace != s.Namespace {
			return
		}
		d.Facts = s.Facts
		mergeKeyedChildren(d, s.Children)

	case *ComponentNode:
		s := src.(*ComponentNode)
		if d.Tag != s.Tag {
			return
		}
		d.Props = s.Props
		if d.SubTree != nil && s.SubTree != nil {
			MergeTree(d.SubTree, s.SubTree)
		} else if s.SubTree != nil {
			d.SubTree = s.SubTree
		}

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
			MergeTree(d.Cached, s.Cached)
		} else {
			d.Cached = s.Cached
		}
	}
}

func mergeChildren(dst *ElementNode, srcKids []Node) {
	minLen := len(dst.Children)
	if len(srcKids) < minLen {
		minLen = len(srcKids)
	}

	for i := 0; i < minLen; i++ {
		if canMerge(dst.Children[i], srcKids[i]) {
			MergeTree(dst.Children[i], srcKids[i])
		} else {
			dst.Children[i] = srcKids[i]
		}
	}

	// Append new children from src.
	if len(srcKids) > len(dst.Children) {
		dst.Children = append(dst.Children, srcKids[len(dst.Children):]...)
	}

	// Truncate removed children.
	if len(dst.Children) > len(srcKids) {
		dst.Children = dst.Children[:len(srcKids)]
	}
}

func mergeKeyedChildren(dst *KeyedElementNode, srcKids []KeyedChild) {
	// Build old key → index map.
	oldByKey := make(map[string]int, len(dst.Children))
	for i, kc := range dst.Children {
		oldByKey[kc.Key] = i
	}

	// Build new children list, reusing old nodes where keys match.
	merged := make([]KeyedChild, len(srcKids))
	for i, sk := range srcKids {
		if oi, ok := oldByKey[sk.Key]; ok {
			// Key exists in old — merge data into old node, keep old ID.
			oldNode := dst.Children[oi].Node
			if canMerge(oldNode, sk.Node) {
				MergeTree(oldNode, sk.Node)
				merged[i] = KeyedChild{Key: sk.Key, Node: oldNode}
			} else {
				merged[i] = sk
			}
		} else {
			// New key — use src node directly.
			merged[i] = sk
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
	case *ComponentNode:
		s := src.(*ComponentNode)
		return d.Tag == s.Tag
	case *PluginNode:
		s := src.(*PluginNode)
		return d.Tag == s.Tag && d.Name == s.Name
	}
	return true // TextNode, LazyNode — always mergeable
}
