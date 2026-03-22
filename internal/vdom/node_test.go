package vdom

import "testing"

// --- ElementNode child manipulation ---

func TestAppendChild(t *testing.T) {
	parent := &ElementNode{NodeBase: NodeBase{ID: 1}, Tag: "div"}
	child1 := &TextNode{NodeBase: NodeBase{ID: 2}, Text: "hello"}
	child2 := &TextNode{NodeBase: NodeBase{ID: 3}, Text: "world"}

	parent.AppendChild(child1)
	if len(parent.Children) != 1 || parent.Children[0].NodeID() != 2 {
		t.Fatalf("expected 1 child with ID=2, got %d children", len(parent.Children))
	}

	parent.AppendChild(child2)
	if len(parent.Children) != 2 || parent.Children[1].NodeID() != 3 {
		t.Fatalf("expected 2 children, second with ID=3")
	}
}

func TestRemoveChild(t *testing.T) {
	parent := &ElementNode{NodeBase: NodeBase{ID: 1}, Tag: "div"}
	parent.Children = []Node{
		&TextNode{NodeBase: NodeBase{ID: 2}, Text: "a"},
		&TextNode{NodeBase: NodeBase{ID: 3}, Text: "b"},
		&TextNode{NodeBase: NodeBase{ID: 4}, Text: "c"},
	}

	// Remove middle child
	ok := parent.RemoveChild(1)
	if !ok {
		t.Fatal("expected RemoveChild(1) to succeed")
	}
	if len(parent.Children) != 2 {
		t.Fatalf("expected 2 children after removal, got %d", len(parent.Children))
	}
	if parent.Children[0].NodeID() != 2 || parent.Children[1].NodeID() != 4 {
		t.Error("expected children [2, 4] after removing index 1")
	}

	// Out of bounds: negative
	if parent.RemoveChild(-1) {
		t.Error("expected RemoveChild(-1) to return false")
	}

	// Out of bounds: too large
	if parent.RemoveChild(5) {
		t.Error("expected RemoveChild(5) to return false")
	}

	// Remove first
	ok = parent.RemoveChild(0)
	if !ok || len(parent.Children) != 1 || parent.Children[0].NodeID() != 4 {
		t.Error("expected child [4] after removing index 0")
	}

	// Remove last remaining
	ok = parent.RemoveChild(0)
	if !ok || len(parent.Children) != 0 {
		t.Error("expected empty children after removing last child")
	}
}

func TestReplaceChild(t *testing.T) {
	parent := &ElementNode{NodeBase: NodeBase{ID: 1}, Tag: "div"}
	parent.Children = []Node{
		&TextNode{NodeBase: NodeBase{ID: 2}, Text: "old"},
		&TextNode{NodeBase: NodeBase{ID: 3}, Text: "keep"},
	}

	replacement := &TextNode{NodeBase: NodeBase{ID: 10}, Text: "new"}

	ok := parent.ReplaceChild(0, replacement)
	if !ok {
		t.Fatal("expected ReplaceChild(0) to succeed")
	}
	if parent.Children[0].NodeID() != 10 {
		t.Errorf("expected replaced child ID=10, got %d", parent.Children[0].NodeID())
	}
	if parent.Children[0].(*TextNode).Text != "new" {
		t.Error("expected replaced child text='new'")
	}
	// Second child unchanged
	if parent.Children[1].NodeID() != 3 {
		t.Error("expected second child unchanged")
	}

	// Out of bounds
	if parent.ReplaceChild(-1, replacement) {
		t.Error("expected ReplaceChild(-1) to return false")
	}
	if parent.ReplaceChild(5, replacement) {
		t.Error("expected ReplaceChild(5) to return false")
	}
}

func TestRemoveChildByID(t *testing.T) {
	parent := &ElementNode{NodeBase: NodeBase{ID: 1}, Tag: "div"}
	parent.Children = []Node{
		&TextNode{NodeBase: NodeBase{ID: 10}, Text: "a"},
		&TextNode{NodeBase: NodeBase{ID: 20}, Text: "b"},
		&TextNode{NodeBase: NodeBase{ID: 30}, Text: "c"},
	}

	// Remove by ID=20 (middle)
	ok := parent.RemoveChildByID(20)
	if !ok {
		t.Fatal("expected RemoveChildByID(20) to succeed")
	}
	if len(parent.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(parent.Children))
	}
	if parent.Children[0].NodeID() != 10 || parent.Children[1].NodeID() != 30 {
		t.Error("expected children [10, 30]")
	}

	// Remove nonexistent ID
	if parent.RemoveChildByID(999) {
		t.Error("expected RemoveChildByID(999) to return false")
	}

	// Remove first by ID
	ok = parent.RemoveChildByID(10)
	if !ok || len(parent.Children) != 1 || parent.Children[0].NodeID() != 30 {
		t.Error("expected child [30] after removing ID=10")
	}

	// Remove last by ID
	ok = parent.RemoveChildByID(30)
	if !ok || len(parent.Children) != 0 {
		t.Error("expected empty children after removing ID=30")
	}
}
