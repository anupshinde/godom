package vdom

import "testing"

func TestMergeTree_TextUpdate(t *testing.T) {
	dst := &TextNode{NodeBase: NodeBase{ID: 10}, Text: "hello"}
	src := &TextNode{NodeBase: NodeBase{ID: 100}, Text: "world"}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID preserved as 10, got %d", dst.ID)
	}
	if dst.Text != "world" {
		t.Errorf("expected text 'world', got %q", dst.Text)
	}
}

func TestMergeTree_ElementFactsUpdate(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "old"}},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "new"}},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID preserved as 10, got %d", dst.ID)
	}
	if dst.Facts.Attrs["class"] != "new" {
		t.Errorf("expected class 'new', got %q", dst.Facts.Attrs["class"])
	}
}

func TestMergeTree_ChildrenPreserveIDs(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "hello"},
			&ElementNode{
				NodeBase: NodeBase{ID: 12},
				Tag:      "span",
				Children: []Node{
					&TextNode{NodeBase: NodeBase{ID: 13}, Text: "nested"},
				},
			},
		},
	}

	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "updated"},
			&ElementNode{
				NodeBase: NodeBase{ID: 102},
				Tag:      "span",
				Children: []Node{
					&TextNode{NodeBase: NodeBase{ID: 103}, Text: "changed"},
				},
			},
		},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("root: expected ID 10, got %d", dst.ID)
	}
	if dst.Children[0].NodeID() != 11 {
		t.Errorf("text child: expected ID 11, got %d", dst.Children[0].NodeID())
	}
	if dst.Children[0].(*TextNode).Text != "updated" {
		t.Errorf("text child: expected 'updated', got %q", dst.Children[0].(*TextNode).Text)
	}
	if dst.Children[1].NodeID() != 12 {
		t.Errorf("span child: expected ID 12, got %d", dst.Children[1].NodeID())
	}
	span := dst.Children[1].(*ElementNode)
	if span.Children[0].NodeID() != 13 {
		t.Errorf("nested text: expected ID 13, got %d", span.Children[0].NodeID())
	}
	if span.Children[0].(*TextNode).Text != "changed" {
		t.Errorf("nested text: expected 'changed', got %q", span.Children[0].(*TextNode).Text)
	}
}

func TestMergeTree_AppendChildren(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "first"},
		},
	}

	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "first"},
			&TextNode{NodeBase: NodeBase{ID: 102}, Text: "second"},
		},
	}

	MergeTree(dst, src)

	if len(dst.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(dst.Children))
	}
	if dst.Children[0].NodeID() != 11 {
		t.Errorf("existing child: expected ID 11, got %d", dst.Children[0].NodeID())
	}
	// New child keeps its new ID (browser learns it from PatchAppend).
	if dst.Children[1].NodeID() != 102 {
		t.Errorf("new child: expected ID 102, got %d", dst.Children[1].NodeID())
	}
}

func TestMergeTree_TruncateChildren(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "first"},
			&TextNode{NodeBase: NodeBase{ID: 12}, Text: "second"},
		},
	}

	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "first"},
		},
	}

	MergeTree(dst, src)

	if len(dst.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(dst.Children))
	}
	if dst.Children[0].NodeID() != 11 {
		t.Errorf("remaining child: expected ID 11, got %d", dst.Children[0].NodeID())
	}
}

func TestMergeTree_TagMismatchReplace(t *testing.T) {
	oldSpan := &ElementNode{NodeBase: NodeBase{ID: 12}, Tag: "span"}
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{oldSpan},
	}

	newP := &ElementNode{NodeBase: NodeBase{ID: 102}, Tag: "p"}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{newP},
	}

	MergeTree(dst, src)

	// Child was replaced because tags don't match.
	if dst.Children[0].NodeID() != 102 {
		t.Errorf("replaced child: expected new ID 102, got %d", dst.Children[0].NodeID())
	}
	if dst.Children[0].(*ElementNode).Tag != "p" {
		t.Errorf("replaced child: expected tag 'p', got %q", dst.Children[0].(*ElementNode).Tag)
	}
}

func TestMergeTree_TypeMismatchReplace(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "hello"},
		},
	}

	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&ElementNode{NodeBase: NodeBase{ID: 101}, Tag: "span"},
		},
	}

	MergeTree(dst, src)

	if dst.Children[0].NodeType() != NodeElement {
		t.Errorf("expected element node, got type %d", dst.Children[0].NodeType())
	}
	if dst.Children[0].NodeID() != 101 {
		t.Errorf("replaced child: expected new ID 101, got %d", dst.Children[0].NodeID())
	}
}

func TestMergeTree_KeyedChildren(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "A"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 12}, Text: "B"}},
		},
	}

	// Reordered: b, a, c (new)
	src := &KeyedElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 101}, Text: "B-updated"}},
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 102}, Text: "A"}},
			{Key: "c", Node: &TextNode{NodeBase: NodeBase{ID: 103}, Text: "C"}},
		},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("root: expected ID 10, got %d", dst.ID)
	}
	if len(dst.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(dst.Children))
	}
	// "b" was ID 12, should be preserved and text updated.
	if dst.Children[0].Node.NodeID() != 12 {
		t.Errorf("key 'b': expected ID 12, got %d", dst.Children[0].Node.NodeID())
	}
	if dst.Children[0].Node.(*TextNode).Text != "B-updated" {
		t.Errorf("key 'b': expected 'B-updated', got %q", dst.Children[0].Node.(*TextNode).Text)
	}
	// "a" was ID 11, should be preserved.
	if dst.Children[1].Node.NodeID() != 11 {
		t.Errorf("key 'a': expected ID 11, got %d", dst.Children[1].Node.NodeID())
	}
	// "c" is new, keeps new ID.
	if dst.Children[2].Node.NodeID() != 103 {
		t.Errorf("key 'c': expected ID 103, got %d", dst.Children[2].Node.NodeID())
	}
}

func TestMergeTree_NilNodes(t *testing.T) {
	// Should not panic.
	MergeTree(nil, nil)
	MergeTree(nil, &TextNode{NodeBase: NodeBase{ID: 1}})
	MergeTree(&TextNode{NodeBase: NodeBase{ID: 1}}, nil)
}

// ---------------------------------------------------------------------------
// Negative / edge cases
// ---------------------------------------------------------------------------

// Root-level type mismatch: dst should be completely untouched.
func TestMergeTree_RootTypeMismatch_NoChange(t *testing.T) {
	dst := &TextNode{NodeBase: NodeBase{ID: 10}, Text: "original"}
	src := &ElementNode{NodeBase: NodeBase{ID: 100}, Tag: "div"}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID unchanged at 10, got %d", dst.ID)
	}
	if dst.Text != "original" {
		t.Errorf("expected text unchanged as 'original', got %q", dst.Text)
	}
}

// Root-level tag mismatch: dst should be completely untouched.
func TestMergeTree_RootTagMismatch_NoChange(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "original"}},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "span",
		Facts:    Facts{Attrs: map[string]string{"class": "new"}},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID unchanged at 10, got %d", dst.ID)
	}
	if dst.Facts.Attrs["class"] != "original" {
		t.Errorf("expected class unchanged as 'original', got %q", dst.Facts.Attrs["class"])
	}
}

// Empty children on both sides: no panic, no changes.
func TestMergeTree_EmptyChildren(t *testing.T) {
	dst := &ElementNode{NodeBase: NodeBase{ID: 10}, Tag: "div"}
	src := &ElementNode{NodeBase: NodeBase{ID: 100}, Tag: "div"}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID 10, got %d", dst.ID)
	}
	if len(dst.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(dst.Children))
	}
}

// Old has children, new has none: all children removed.
func TestMergeTree_AllChildrenRemoved(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "a"},
			&TextNode{NodeBase: NodeBase{ID: 12}, Text: "b"},
			&TextNode{NodeBase: NodeBase{ID: 13}, Text: "c"},
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
	}

	MergeTree(dst, src)

	if len(dst.Children) != 0 {
		t.Errorf("expected 0 children after merge, got %d", len(dst.Children))
	}
}

// Old has no children, new has several: all appended.
func TestMergeTree_AllChildrenNew(t *testing.T) {
	dst := &ElementNode{NodeBase: NodeBase{ID: 10}, Tag: "div"}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "x"},
			&TextNode{NodeBase: NodeBase{ID: 102}, Text: "y"},
		},
	}

	MergeTree(dst, src)

	if len(dst.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(dst.Children))
	}
	if dst.Children[0].NodeID() != 101 {
		t.Errorf("first child: expected new ID 101, got %d", dst.Children[0].NodeID())
	}
	if dst.Children[1].NodeID() != 102 {
		t.Errorf("second child: expected new ID 102, got %d", dst.Children[1].NodeID())
	}
}

// Multiple type mismatches at different child positions.
func TestMergeTree_MixedChildMismatches(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "keep"},           // position 0: text→text, merge
			&ElementNode{NodeBase: NodeBase{ID: 12}, Tag: "span"},         // position 1: span→p, replace
			&TextNode{NodeBase: NodeBase{ID: 13}, Text: "also keep"},      // position 2: text→element, replace
			&ElementNode{NodeBase: NodeBase{ID: 14}, Tag: "div"},          // position 3: div→div, merge
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "updated"},
			&ElementNode{NodeBase: NodeBase{ID: 102}, Tag: "p"},
			&ElementNode{NodeBase: NodeBase{ID: 103}, Tag: "a"},
			&ElementNode{NodeBase: NodeBase{ID: 104}, Tag: "div", Facts: Facts{Attrs: map[string]string{"id": "merged"}}},
		},
	}

	MergeTree(dst, src)

	// Position 0: merged (same type), keeps old ID.
	if dst.Children[0].NodeID() != 11 {
		t.Errorf("pos 0: expected ID 11, got %d", dst.Children[0].NodeID())
	}
	if dst.Children[0].(*TextNode).Text != "updated" {
		t.Errorf("pos 0: expected 'updated', got %q", dst.Children[0].(*TextNode).Text)
	}

	// Position 1: replaced (span→p), gets new ID.
	if dst.Children[1].NodeID() != 102 {
		t.Errorf("pos 1: expected new ID 102, got %d", dst.Children[1].NodeID())
	}
	if dst.Children[1].(*ElementNode).Tag != "p" {
		t.Errorf("pos 1: expected tag 'p', got %q", dst.Children[1].(*ElementNode).Tag)
	}

	// Position 2: replaced (text→element), gets new ID.
	if dst.Children[2].NodeID() != 103 {
		t.Errorf("pos 2: expected new ID 103, got %d", dst.Children[2].NodeID())
	}

	// Position 3: merged (div→div), keeps old ID, gets new facts.
	if dst.Children[3].NodeID() != 14 {
		t.Errorf("pos 3: expected ID 14, got %d", dst.Children[3].NodeID())
	}
	if dst.Children[3].(*ElementNode).Facts.Attrs["id"] != "merged" {
		t.Errorf("pos 3: expected attr id='merged'")
	}
}

// Deeply nested: 4 levels deep, IDs preserved at every level.
func TestMergeTree_DeeplyNested(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div",
		Children: []Node{
			&ElementNode{
				NodeBase: NodeBase{ID: 2}, Tag: "div",
				Children: []Node{
					&ElementNode{
						NodeBase: NodeBase{ID: 3}, Tag: "div",
						Children: []Node{
							&TextNode{NodeBase: NodeBase{ID: 4}, Text: "deep"},
						},
					},
				},
			},
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 101}, Tag: "div",
		Children: []Node{
			&ElementNode{
				NodeBase: NodeBase{ID: 102}, Tag: "div",
				Children: []Node{
					&ElementNode{
						NodeBase: NodeBase{ID: 103}, Tag: "div",
						Children: []Node{
							&TextNode{NodeBase: NodeBase{ID: 104}, Text: "updated-deep"},
						},
					},
				},
			},
		},
	}

	MergeTree(dst, src)

	if dst.ID != 1 {
		t.Errorf("level 0: expected ID 1, got %d", dst.ID)
	}
	l1 := dst.Children[0].(*ElementNode)
	if l1.ID != 2 {
		t.Errorf("level 1: expected ID 2, got %d", l1.ID)
	}
	l2 := l1.Children[0].(*ElementNode)
	if l2.ID != 3 {
		t.Errorf("level 2: expected ID 3, got %d", l2.ID)
	}
	l3 := l2.Children[0].(*TextNode)
	if l3.ID != 4 {
		t.Errorf("level 3: expected ID 4, got %d", l3.ID)
	}
	if l3.Text != "updated-deep" {
		t.Errorf("level 3: expected 'updated-deep', got %q", l3.Text)
	}
}

// Keyed children: keys removed from old.
func TestMergeTree_KeyedChildrenRemoved(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "A"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 12}, Text: "B"}},
			{Key: "c", Node: &TextNode{NodeBase: NodeBase{ID: 13}, Text: "C"}},
		},
	}
	// Only "b" survives.
	src := &KeyedElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 101}, Text: "B"}},
		},
	}

	MergeTree(dst, src)

	if len(dst.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(dst.Children))
	}
	if dst.Children[0].Key != "b" {
		t.Errorf("expected key 'b', got %q", dst.Children[0].Key)
	}
	if dst.Children[0].Node.NodeID() != 12 {
		t.Errorf("key 'b': expected old ID 12, got %d", dst.Children[0].Node.NodeID())
	}
}

// Keyed children: tag mismatch on a matched key → replace.
func TestMergeTree_KeyedChildTagMismatch(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &ElementNode{NodeBase: NodeBase{ID: 11}, Tag: "li"}},
		},
	}
	src := &KeyedElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &ElementNode{NodeBase: NodeBase{ID: 101}, Tag: "div"}},
		},
	}

	MergeTree(dst, src)

	// Same key but different tag → replaced, gets new ID.
	if dst.Children[0].Node.NodeID() != 101 {
		t.Errorf("key 'a': expected new ID 101, got %d", dst.Children[0].Node.NodeID())
	}
	if dst.Children[0].Node.(*ElementNode).Tag != "div" {
		t.Errorf("key 'a': expected tag 'div', got %q", dst.Children[0].Node.(*ElementNode).Tag)
	}
}

// Plugin node merge: data and facts updated, ID preserved.
func TestMergeTree_PluginNode(t *testing.T) {
	dst := &PluginNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "canvas",
		Name:     "chart",
		Facts:    Facts{Attrs: map[string]string{"width": "100"}},
		Data:     map[string]int{"points": 5},
	}
	src := &PluginNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "canvas",
		Name:     "chart",
		Facts:    Facts{Attrs: map[string]string{"width": "200"}},
		Data:     map[string]int{"points": 10},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID 10, got %d", dst.ID)
	}
	if dst.Facts.Attrs["width"] != "200" {
		t.Errorf("expected width '200', got %q", dst.Facts.Attrs["width"])
	}
	if dst.Data.(map[string]int)["points"] != 10 {
		t.Errorf("expected points 10, got %v", dst.Data)
	}
}

// Plugin node: name mismatch → no merge (dst untouched).
func TestMergeTree_PluginNameMismatch(t *testing.T) {
	dst := &PluginNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "canvas",
		Name:     "chart",
		Data:     "old-data",
	}
	src := &PluginNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "canvas",
		Name:     "graph",
		Data:     "new-data",
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID unchanged at 10, got %d", dst.ID)
	}
	if dst.Data != "old-data" {
		t.Errorf("expected data unchanged as 'old-data', got %v", dst.Data)
	}
}

// Keyed element namespace mismatch → no merge.
func TestMergeTree_KeyedElementNamespaceMismatch(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase:  NodeBase{ID: 10},
		Tag:       "g",
		Namespace: "http://www.w3.org/2000/svg",
		Facts:     Facts{Attrs: map[string]string{"fill": "red"}},
	}
	src := &KeyedElementNode{
		NodeBase:  NodeBase{ID: 100},
		Tag:       "g",
		Namespace: "",
		Facts:     Facts{Attrs: map[string]string{"fill": "blue"}},
	}

	MergeTree(dst, src)

	if dst.Facts.Attrs["fill"] != "red" {
		t.Errorf("expected facts unchanged, got fill=%q", dst.Facts.Attrs["fill"])
	}
}

// Verify merge is idempotent: merging identical trees changes nothing.
func TestMergeTree_Idempotent(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "same"}},
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 11}, Text: "unchanged"},
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "same"}},
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "unchanged"},
		},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID 10, got %d", dst.ID)
	}
	if dst.Facts.Attrs["class"] != "same" {
		t.Errorf("expected class 'same', got %q", dst.Facts.Attrs["class"])
	}
	if dst.Children[0].NodeID() != 11 {
		t.Errorf("expected child ID 11, got %d", dst.Children[0].NodeID())
	}
	if dst.Children[0].(*TextNode).Text != "unchanged" {
		t.Errorf("expected text 'unchanged', got %q", dst.Children[0].(*TextNode).Text)
	}
}

// ---------------------------------------------------------------------------
// LazyNode tests
// ---------------------------------------------------------------------------

// LazyNode merge: Func, Args updated, Cached merged when both non-nil.
func TestMergeTree_LazyNode(t *testing.T) {
	oldFunc := func() {}
	newFunc := func() {}
	dst := &LazyNode{
		NodeBase: NodeBase{ID: 10},
		Func:     oldFunc,
		Args:     []any{"old"},
		Cached:   &TextNode{NodeBase: NodeBase{ID: 11}, Text: "cached-old"},
	}
	src := &LazyNode{
		NodeBase: NodeBase{ID: 100},
		Func:     newFunc,
		Args:     []any{"new"},
		Cached:   &TextNode{NodeBase: NodeBase{ID: 101}, Text: "cached-new"},
	}

	MergeTree(dst, src)

	if dst.ID != 10 {
		t.Errorf("expected ID 10, got %d", dst.ID)
	}
	if len(dst.Args) != 1 || dst.Args[0] != "new" {
		t.Errorf("expected Args ['new'], got %v", dst.Args)
	}
	// Cached should be merged: old ID preserved, text updated.
	if dst.Cached.NodeID() != 11 {
		t.Errorf("cached: expected ID 11, got %d", dst.Cached.NodeID())
	}
	if dst.Cached.(*TextNode).Text != "cached-new" {
		t.Errorf("cached: expected 'cached-new', got %q", dst.Cached.(*TextNode).Text)
	}
}

// LazyNode: dst.Cached nil, src.Cached non-nil → src.Cached adopted.
func TestMergeTree_LazyNodeNilToCache(t *testing.T) {
	dst := &LazyNode{
		NodeBase: NodeBase{ID: 10},
		Cached:   nil,
	}
	src := &LazyNode{
		NodeBase: NodeBase{ID: 100},
		Cached:   &TextNode{NodeBase: NodeBase{ID: 101}, Text: "new-cache"},
	}

	MergeTree(dst, src)

	if dst.Cached == nil {
		t.Fatal("expected Cached to be set")
	}
	if dst.Cached.NodeID() != 101 {
		t.Errorf("cached: expected ID 101, got %d", dst.Cached.NodeID())
	}
}

// LazyNode: both Cached nil → no panic.
func TestMergeTree_LazyNodeBothCachedNil(t *testing.T) {
	dst := &LazyNode{NodeBase: NodeBase{ID: 10}, Cached: nil}
	src := &LazyNode{NodeBase: NodeBase{ID: 100}, Cached: nil}

	MergeTree(dst, src) // should not panic

	if dst.Cached != nil {
		t.Errorf("expected Cached to remain nil")
	}
}

// LazyNode: dst.Cached non-nil, src.Cached nil → dst.Cached replaced with nil.
func TestMergeTree_LazyNodeCacheToNil(t *testing.T) {
	dst := &LazyNode{
		NodeBase: NodeBase{ID: 10},
		Cached:   &TextNode{NodeBase: NodeBase{ID: 11}, Text: "old"},
	}
	src := &LazyNode{
		NodeBase: NodeBase{ID: 100},
		Cached:   nil,
	}

	MergeTree(dst, src)

	// The else branch sets d.Cached = s.Cached (nil).
	if dst.Cached != nil {
		t.Errorf("expected Cached to be nil, got %v", dst.Cached)
	}
}

// ---------------------------------------------------------------------------
// canMerge coverage via child replacement
// ---------------------------------------------------------------------------

// canMerge for PluginNode children (same tag+name → merge, mismatch → replace).
func TestMergeTree_CanMerge_PluginChild(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div",
		Children: []Node{
			&PluginNode{NodeBase: NodeBase{ID: 10}, Tag: "canvas", Name: "chart", Data: "old"},
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100}, Tag: "div",
		Children: []Node{
			&PluginNode{NodeBase: NodeBase{ID: 101}, Tag: "canvas", Name: "chart", Data: "new"},
		},
	}

	MergeTree(dst, src)

	// Same tag+name → merged, old ID preserved.
	if dst.Children[0].NodeID() != 10 {
		t.Errorf("expected ID 10, got %d", dst.Children[0].NodeID())
	}
	if dst.Children[0].(*PluginNode).Data != "new" {
		t.Errorf("expected data updated to 'new'")
	}
}

// canMerge for PluginNode children with name mismatch → replace.
func TestMergeTree_CanMerge_PluginChildMismatch(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div",
		Children: []Node{
			&PluginNode{NodeBase: NodeBase{ID: 10}, Tag: "canvas", Name: "chart"},
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100}, Tag: "div",
		Children: []Node{
			&PluginNode{NodeBase: NodeBase{ID: 101}, Tag: "canvas", Name: "graph"},
		},
	}

	MergeTree(dst, src)

	// Different name → replaced.
	if dst.Children[0].NodeID() != 101 {
		t.Errorf("expected new ID 101, got %d", dst.Children[0].NodeID())
	}
}

// ElementNode namespace mismatch at root → no merge.
func TestMergeTree_ElementNamespaceMismatch(t *testing.T) {
	dst := &ElementNode{
		NodeBase:  NodeBase{ID: 10},
		Tag:       "g",
		Namespace: "http://www.w3.org/2000/svg",
		Facts:     Facts{Attrs: map[string]string{"fill": "red"}},
	}
	src := &ElementNode{
		NodeBase:  NodeBase{ID: 100},
		Tag:       "g",
		Namespace: "",
		Facts:     Facts{Attrs: map[string]string{"fill": "blue"}},
	}

	MergeTree(dst, src)

	if dst.Facts.Attrs["fill"] != "red" {
		t.Errorf("expected facts unchanged, got fill=%q", dst.Facts.Attrs["fill"])
	}
}

// KeyedElementNode tag mismatch → no merge (dst untouched).
func TestMergeTree_KeyedElementTagMismatch(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "ul",
		Facts:    Facts{Attrs: map[string]string{"class": "old"}},
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "A"}},
		},
	}
	src := &KeyedElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "ol",
		Facts:    Facts{Attrs: map[string]string{"class": "new"}},
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 101}, Text: "A-new"}},
		},
	}

	MergeTree(dst, src)

	if dst.Facts.Attrs["class"] != "old" {
		t.Errorf("expected facts unchanged, got class=%q", dst.Facts.Attrs["class"])
	}
	if dst.Children[0].Node.(*TextNode).Text != "A" {
		t.Errorf("expected children unchanged, got text=%q", dst.Children[0].Node.(*TextNode).Text)
	}
}

// PluginNode tag mismatch (same name, different tag) → no merge.
func TestMergeTree_PluginTagMismatch(t *testing.T) {
	dst := &PluginNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "canvas",
		Name:     "chart",
		Data:     "old-data",
	}
	src := &PluginNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Name:     "chart",
		Data:     "new-data",
	}

	MergeTree(dst, src)

	if dst.Data != "old-data" {
		t.Errorf("expected data unchanged as 'old-data', got %v", dst.Data)
	}
}

// Keyed children: type mismatch on a matched key → replace.
func TestMergeTree_KeyedChildTypeMismatch(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "text"}},
		},
	}
	src := &KeyedElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &ElementNode{NodeBase: NodeBase{ID: 101}, Tag: "li"}},
		},
	}

	MergeTree(dst, src)

	// Same key but text→element type mismatch → replaced.
	if dst.Children[0].Node.NodeType() != NodeElement {
		t.Errorf("expected element node, got type %d", dst.Children[0].Node.NodeType())
	}
	if dst.Children[0].Node.NodeID() != 101 {
		t.Errorf("expected new ID 101, got %d", dst.Children[0].Node.NodeID())
	}
}

// Keyed children: all keys removed (empty src children).
func TestMergeTree_KeyedChildrenAllRemoved(t *testing.T) {
	dst := &KeyedElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "A"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 12}, Text: "B"}},
		},
	}
	src := &KeyedElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "ul",
		Children: []KeyedChild{},
	}

	MergeTree(dst, src)

	if len(dst.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(dst.Children))
	}
}

// canMerge for KeyedElementNode children (via keyed child with matching key).
func TestMergeTree_CanMerge_KeyedElementChild(t *testing.T) {
	dst := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div",
		Children: []Node{
			&KeyedElementNode{
				NodeBase: NodeBase{ID: 10}, Tag: "ul",
				Children: []KeyedChild{
					{Key: "x", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "old"}},
				},
			},
		},
	}
	src := &ElementNode{
		NodeBase: NodeBase{ID: 100}, Tag: "div",
		Children: []Node{
			&KeyedElementNode{
				NodeBase: NodeBase{ID: 101}, Tag: "ul",
				Children: []KeyedChild{
					{Key: "x", Node: &TextNode{NodeBase: NodeBase{ID: 111}, Text: "new"}},
				},
			},
		},
	}

	MergeTree(dst, src)

	// KeyedElementNode merged, old ID preserved.
	if dst.Children[0].NodeID() != 10 {
		t.Errorf("expected ID 10, got %d", dst.Children[0].NodeID())
	}
	ke := dst.Children[0].(*KeyedElementNode)
	if ke.Children[0].Node.(*TextNode).Text != "new" {
		t.Errorf("expected text 'new'")
	}
}

// [COVERAGE GAP] merge.go lines 42-43, 50-51, 58-59
// The inner tag/namespace checks inside mergeTree's switch cases for
// ElementNode, KeyedElementNode, and PluginNode are unreachable.
// canMerge (called at line 24) already verifies the exact same conditions
// (tag, namespace, name) before the switch is entered. If canMerge returns
// false, mergeTree returns early at line 25, so the inner guards can never
// trigger. These are defensive redundant checks.

