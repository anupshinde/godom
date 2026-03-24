package vdom

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Unreachable code — cannot be covered via tests
//
// ComputeDescendants final return (node.go:249):
//   The Node interface is a closed set in production (TextNode, ElementNode,
//   KeyedElementNode, PluginNode, LazyNode). Covered via
//   fakeNode test type since Node is an exported interface.
//
// lazyArgsEqual / valEqual — each have ~1 statement gap from branches
//   that require internal states not producible through the public API.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Text diffing
// ---------------------------------------------------------------------------

func TestDiff_IdenticalText(t *testing.T) {
	node := &TextNode{Text: "hello"}
	patches := Diff(node, node)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for identical nodes, got %d", len(patches))
	}
}

func TestDiff_TextChange(t *testing.T) {
	old := &TextNode{Text: "hello"}
	new := &TextNode{Text: "world"}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchText {
		t.Errorf("expected PatchText, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchTextData)
	if data.Text != "world" {
		t.Errorf("expected 'world', got %q", data.Text)
	}
}

func TestDiff_SameText(t *testing.T) {
	old := &TextNode{Text: "hello"}
	new := &TextNode{Text: "hello"}
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for same text, got %d", len(patches))
	}
}

// ---------------------------------------------------------------------------
// Different node types → redraw
// ---------------------------------------------------------------------------

func TestDiff_DifferentNodeTypes(t *testing.T) {
	old := &TextNode{Text: "hello"}
	new := &ElementNode{Tag: "div"}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchRedraw {
		t.Errorf("expected PatchRedraw, got %d", patches[0].Type)
	}
}

// ---------------------------------------------------------------------------
// Element diffing
// ---------------------------------------------------------------------------

func TestDiff_ElementTagChange(t *testing.T) {
	old := &ElementNode{Tag: "div"}
	new := &ElementNode{Tag: "span"}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchRedraw {
		t.Errorf("expected PatchRedraw for tag change, got %d", patches[0].Type)
	}
}

func TestDiff_ElementSameTag(t *testing.T) {
	old := &ElementNode{Tag: "div"}
	new := &ElementNode{Tag: "div"}
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for identical elements, got %d", len(patches))
	}
}

func TestDiff_ElementFactsChange(t *testing.T) {
	old := &ElementNode{
		Tag:   "div",
		Facts: Facts{Props: map[string]any{"className": "old"}},
	}
	new := &ElementNode{
		Tag:   "div",
		Facts: Facts{Props: map[string]any{"className": "new"}},
	}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchFacts {
		t.Errorf("expected PatchFacts, got %d", patches[0].Type)
	}
	fd := patches[0].Data.(PatchFactsData)
	if fd.Diff.Props["className"] != "new" {
		t.Errorf("expected className='new' in diff, got %v", fd.Diff.Props["className"])
	}
}

// ---------------------------------------------------------------------------
// Children diffing
// ---------------------------------------------------------------------------

func TestDiff_ChildAppend(t *testing.T) {
	old := makeDiv(
		&TextNode{Text: "a"},
	)
	new := makeDiv(
		&TextNode{Text: "a"},
		&TextNode{Text: "b"},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchAppend {
		t.Errorf("expected PatchAppend, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchAppendData)
	if len(data.Nodes) != 1 {
		t.Errorf("expected 1 appended node, got %d", len(data.Nodes))
	}
}

func TestDiff_ChildRemoveLast(t *testing.T) {
	old := makeDiv(
		&TextNode{Text: "a"},
		&TextNode{Text: "b"},
		&TextNode{Text: "c"},
	)
	new := makeDiv(
		&TextNode{Text: "a"},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchRemoveLast {
		t.Errorf("expected PatchRemoveLast, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchRemoveLastData)
	if data.Count != 2 {
		t.Errorf("expected count=2, got %d", data.Count)
	}
}

func TestDiff_ChildChange(t *testing.T) {
	old := makeDiv(
		&TextNode{Text: "a"},
		&TextNode{Text: "b"},
	)
	new := makeDiv(
		&TextNode{Text: "a"},
		&TextNode{Text: "c"},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchText {
		t.Errorf("expected PatchText for changed child, got %d", patches[0].Type)
	}
}

func TestDiff_NestedChildChange(t *testing.T) {
	old := makeDiv(
		&ElementNode{
			Tag:      "span",
			Children: []Node{&TextNode{Text: "hello"}},
		},
	)
	new := makeDiv(
		&ElementNode{
			Tag:      "span",
			Children: []Node{&TextNode{Text: "world"}},
		},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchText {
		t.Errorf("expected PatchText, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchTextData)
	if data.Text != "world" {
		t.Errorf("expected 'world', got %q", data.Text)
	}
}

func TestDiff_MultipleChildChanges(t *testing.T) {
	old := makeDiv(
		&TextNode{Text: "a"},
		&TextNode{Text: "b"},
		&TextNode{Text: "c"},
	)
	new := makeDiv(
		&TextNode{Text: "x"},
		&TextNode{Text: "b"},
		&TextNode{Text: "z"},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	// Should have 2 patches: change "a"→"x" and "c"→"z"
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d: %+v", len(patches), patches)
	}
	for _, p := range patches {
		if p.Type != PatchText {
			t.Errorf("expected PatchText, got %d", p.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Facts diffing
// ---------------------------------------------------------------------------

func TestDiffFacts_PropAdded(t *testing.T) {
	old := Facts{}
	new := Facts{Props: map[string]any{"id": "main"}}
	d := DiffFacts(&old, &new)
	if d.Props["id"] != "main" {
		t.Errorf("expected id='main' in diff, got %v", d.Props)
	}
}

func TestDiffFacts_PropRemoved(t *testing.T) {
	old := Facts{Props: map[string]any{"id": "main"}}
	new := Facts{}
	d := DiffFacts(&old, &new)
	if _, ok := d.Props["id"]; !ok {
		t.Error("expected id removal in diff")
	}
	if d.Props["id"] != nil {
		t.Errorf("expected nil for removed prop, got %v", d.Props["id"])
	}
}

func TestDiffFacts_StyleChange(t *testing.T) {
	old := Facts{Styles: map[string]string{"width": "100px"}}
	new := Facts{Styles: map[string]string{"width": "200px"}}
	d := DiffFacts(&old, &new)
	if d.Styles["width"] != "200px" {
		t.Errorf("expected width='200px', got %q", d.Styles["width"])
	}
}

func TestDiffFacts_StyleRemoved(t *testing.T) {
	old := Facts{Styles: map[string]string{"width": "100px"}}
	new := Facts{}
	d := DiffFacts(&old, &new)
	if d.Styles["width"] != "" {
		t.Errorf("expected empty string for removed style, got %q", d.Styles["width"])
	}
}

func TestDiffFacts_EventAdded(t *testing.T) {
	old := Facts{}
	new := Facts{Events: map[string]EventHandler{
		"click": {Handler: "Save"},
	}}
	d := DiffFacts(&old, &new)
	if d.Events["click"] == nil {
		t.Error("expected click event in diff")
	}
	if d.Events["click"].Handler != "Save" {
		t.Errorf("expected handler 'Save', got %q", d.Events["click"].Handler)
	}
}

func TestDiffFacts_EventRemoved(t *testing.T) {
	old := Facts{Events: map[string]EventHandler{
		"click": {Handler: "Save"},
	}}
	new := Facts{}
	d := DiffFacts(&old, &new)
	if _, ok := d.Events["click"]; !ok {
		t.Error("expected click removal in diff")
	}
	if d.Events["click"] != nil {
		t.Error("expected nil for removed event")
	}
}

func TestDiffFacts_EventChanged(t *testing.T) {
	old := Facts{Events: map[string]EventHandler{
		"click": {Handler: "Save"},
	}}
	new := Facts{Events: map[string]EventHandler{
		"click": {Handler: "Update"},
	}}
	d := DiffFacts(&old, &new)
	if d.Events["click"] == nil {
		t.Fatal("expected click change in diff")
	}
	if d.Events["click"].Handler != "Update" {
		t.Errorf("expected handler 'Update', got %q", d.Events["click"].Handler)
	}
}

func TestDiffFacts_Identity(t *testing.T) {
	f := Facts{
		Props:  map[string]any{"id": "main"},
		Styles: map[string]string{"width": "100px"},
	}
	d := DiffFacts(&f, &f)
	if !d.IsEmpty() {
		t.Error("expected empty diff for same Facts pointer")
	}
}

func TestDiffFacts_NoChange(t *testing.T) {
	old := Facts{
		Props:   map[string]any{"id": "main", "className": "container", "hidden": false},
		Attrs:   map[string]string{"data-id": "42", "role": "banner"},
		Styles:  map[string]string{"width": "100px", "color": "red"},
		Events:  map[string]EventHandler{"click": {Handler: "Save", Args: []any{1, "two"}, Options: EventOptions{PreventDefault: true}}},
		AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#icon"}},
	}
	new := Facts{
		Props:   map[string]any{"id": "main", "className": "container", "hidden": false},
		Attrs:   map[string]string{"data-id": "42", "role": "banner"},
		Styles:  map[string]string{"width": "100px", "color": "red"},
		Events:  map[string]EventHandler{"click": {Handler: "Save", Args: []any{1, "two"}, Options: EventOptions{PreventDefault: true}}},
		AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#icon"}},
	}
	d := DiffFacts(&old, &new)
	if !d.IsEmpty() {
		t.Errorf("expected empty diff for equal Facts, got props=%v attrs=%v styles=%v events=%v attrsNS=%v",
			d.Props, d.Attrs, d.Styles, d.Events, d.AttrsNS)
	}
}

func TestDiffFacts_NoChange_Negative(t *testing.T) {
	// Verify that even a single field difference is detected.
	base := func() Facts {
		return Facts{
			Props:  map[string]any{"id": "main", "className": "box"},
			Attrs:  map[string]string{"data-id": "1"},
			Styles: map[string]string{"width": "10px"},
			Events: map[string]EventHandler{"click": {Handler: "Save"}},
		}
	}

	t.Run("prop value differs", func(t *testing.T) {
		old := base()
		new := base()
		new.Props["id"] = "other"
		d := DiffFacts(&old, &new)
		if d.IsEmpty() {
			t.Error("expected non-empty diff when prop value differs")
		}
		if d.Props["id"] != "other" {
			t.Errorf("expected id='other' in diff, got %v", d.Props["id"])
		}
	})

	t.Run("prop added", func(t *testing.T) {
		old := base()
		new := base()
		new.Props["tabIndex"] = 0
		d := DiffFacts(&old, &new)
		if d.Props["tabIndex"] != 0 {
			t.Errorf("expected tabIndex=0 in diff, got %v", d.Props)
		}
	})

	t.Run("prop removed", func(t *testing.T) {
		old := base()
		new := base()
		delete(new.Props, "className")
		d := DiffFacts(&old, &new)
		if _, ok := d.Props["className"]; !ok {
			t.Error("expected className removal in diff")
		}
		if d.Props["className"] != nil {
			t.Errorf("expected nil for removed prop, got %v", d.Props["className"])
		}
	})

	t.Run("attr value differs", func(t *testing.T) {
		old := base()
		new := base()
		new.Attrs["data-id"] = "2"
		d := DiffFacts(&old, &new)
		if d.Attrs["data-id"] != "2" {
			t.Errorf("expected data-id='2', got %q", d.Attrs["data-id"])
		}
	})

	t.Run("style value differs", func(t *testing.T) {
		old := base()
		new := base()
		new.Styles["width"] = "20px"
		d := DiffFacts(&old, &new)
		if d.Styles["width"] != "20px" {
			t.Errorf("expected width='20px', got %q", d.Styles["width"])
		}
	})

	t.Run("event handler differs", func(t *testing.T) {
		old := base()
		new := base()
		new.Events["click"] = EventHandler{Handler: "Delete"}
		d := DiffFacts(&old, &new)
		if d.Events["click"] == nil || d.Events["click"].Handler != "Delete" {
			t.Errorf("expected click handler 'Delete' in diff, got %v", d.Events["click"])
		}
	})

	t.Run("prop type differs", func(t *testing.T) {
		old := base()
		new := base()
		old.Props["id"] = "main"
		new.Props["id"] = 42 // string → int
		d := DiffFacts(&old, &new)
		if d.IsEmpty() {
			t.Error("expected non-empty diff when prop type changes")
		}
		if d.Props["id"] != 42 {
			t.Errorf("expected id=42 in diff, got %v", d.Props["id"])
		}
	})
}

func TestDiffFacts_AttrChange(t *testing.T) {
	old := Facts{Attrs: map[string]string{"data-id": "1"}}
	new := Facts{Attrs: map[string]string{"data-id": "2"}}
	d := DiffFacts(&old, &new)
	if d.Attrs["data-id"] != "2" {
		t.Errorf("expected data-id='2', got %q", d.Attrs["data-id"])
	}
}

// ---------------------------------------------------------------------------
// Plugin diffing
// ---------------------------------------------------------------------------

func TestDiff_PluginDataChange(t *testing.T) {
	old := &PluginNode{Tag: "canvas", Name: "chart", Data: map[string]int{"a": 1}}
	new := &PluginNode{Tag: "canvas", Name: "chart", Data: map[string]int{"a": 2}}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchPlugin {
		t.Errorf("expected PatchPlugin, got %d", patches[0].Type)
	}
}

func TestDiff_PluginNameChange(t *testing.T) {
	old := &PluginNode{Tag: "canvas", Name: "chart", Data: nil}
	new := &PluginNode{Tag: "canvas", Name: "map", Data: nil}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchRedraw {
		t.Errorf("expected PatchRedraw for plugin name change, got %d", patches[0].Type)
	}
}

func TestDiff_PluginSameData(t *testing.T) {
	// Two independently constructed maps with equal content.
	old := &PluginNode{Tag: "canvas", Name: "chart", Data: map[string]int{"a": 1, "b": 2}}
	new := &PluginNode{Tag: "canvas", Name: "chart", Data: map[string]int{"a": 1, "b": 2}}
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for equal plugin data, got %d", len(patches))
	}
}

func TestDiff_PluginSameDataPointer(t *testing.T) {
	// Same pointer — should also produce no patches.
	data := map[string]int{"a": 1}
	old := &PluginNode{Tag: "canvas", Name: "chart", Data: data}
	new := &PluginNode{Tag: "canvas", Name: "chart", Data: data}
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for same pointer data, got %d", len(patches))
	}
}

func TestDiff_PluginDataChange_Negative(t *testing.T) {
	// One value differs — must produce a PatchPlugin.
	old := &PluginNode{Tag: "canvas", Name: "chart", Data: map[string]int{"a": 1, "b": 2}}
	new := &PluginNode{Tag: "canvas", Name: "chart", Data: map[string]int{"a": 1, "b": 99}}
	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchPlugin {
		t.Errorf("expected PatchPlugin, got %d", patches[0].Type)
	}
}

// ---------------------------------------------------------------------------
// Lazy diffing
// ---------------------------------------------------------------------------

func TestDiff_LazySameArgs(t *testing.T) {
	fn := func(n int) Node { return &TextNode{Text: "result"} }
	cached := &TextNode{Text: "result"}

	old := &LazyNode{Func: fn, Args: []any{42}, Cached: cached}
	new := &LazyNode{Func: fn, Args: []any{42}, Cached: nil}

	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for lazy with same args, got %d", len(patches))
	}
	// new.Cached should be set to old.Cached
	if new.Cached != cached {
		t.Error("expected new.Cached to be set to old.Cached")
	}
}

func TestDiff_LazyDifferentArgs(t *testing.T) {
	fn := func(n int) Node { return &TextNode{Text: "new"} }
	oldCached := &TextNode{Text: "old"}

	old := &LazyNode{Func: fn, Args: []any{1}, Cached: oldCached}
	new := &LazyNode{Func: fn, Args: []any{2}, Cached: nil}

	patches := Diff(old, new)
	// Should have a lazy patch wrapping a text change
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchLazy {
		t.Errorf("expected PatchLazy, got %d", patches[0].Type)
	}
}

// ---------------------------------------------------------------------------
// Keyed children (simple/placeholder)
// ---------------------------------------------------------------------------

func TestDiff_KeyedIdenticalChildren(t *testing.T) {
	// Same keys, same order, same content → no removes, no inserts, no subPatches → early return
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 10}, Text: "A"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "B"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 10}, Text: "A"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 11}, Text: "B"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for identical keyed children, got %d: %+v", len(patches), patches)
	}
}

func TestDiff_KeyedSameKeys(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B-updated"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchText {
		t.Errorf("expected PatchText, got %d", patches[0].Type)
	}
}

func TestDiff_KeyedAppend(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchReorder {
		t.Errorf("expected PatchReorder, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchReorderData)
	if len(data.Inserts) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(data.Inserts))
	}
	if data.Inserts[0].Key != "b" {
		t.Errorf("expected insert key 'b', got %q", data.Inserts[0].Key)
	}
	if len(data.Removes) != 0 {
		t.Errorf("expected 0 removes, got %d", len(data.Removes))
	}
}

func TestDiff_KeyedPrepend(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	// Should insert "a" at index 0, no removes.
	if len(data.Inserts) != 1 || data.Inserts[0].Key != "a" || data.Inserts[0].Index != 0 {
		t.Errorf("expected insert of 'a' at 0, got %+v", data.Inserts)
	}
	if data.Inserts[0].Node == nil {
		t.Errorf("new insert should have a Node for HTML rendering")
	}
	if len(data.Removes) != 0 {
		t.Errorf("expected 0 removes, got %+v", data.Removes)
	}
}

func TestDiff_KeyedRemoveMiddle(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	if len(data.Removes) != 1 || data.Removes[0].Key != "b" {
		t.Errorf("expected remove of 'b', got %+v", data.Removes)
	}
	if data.Removes[0].Index != 1 {
		t.Errorf("expected remove index 1, got %d", data.Removes[0].Index)
	}
	if len(data.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %+v", data.Inserts)
	}
}

func TestDiff_KeyedRemoveFirst(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	if len(data.Removes) != 1 || data.Removes[0].Key != "a" {
		t.Errorf("expected remove of 'a', got %+v", data.Removes)
	}
	if data.Removes[0].Index != 0 {
		t.Errorf("expected remove index 0, got %d", data.Removes[0].Index)
	}
	if len(data.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %+v", data.Inserts)
	}
}

func TestDiff_KeyedRemoveFirst_Negative(t *testing.T) {
	// Removing "a" from [a, b, c] should NOT touch "b" or "c".
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)

	// Surviving keys must NOT appear in removes.
	for _, r := range data.Removes {
		if r.Key == "b" || r.Key == "c" {
			t.Errorf("surviving key %q should not be in removes", r.Key)
		}
	}
	// No content patches — "b" and "c" are unchanged.
	if len(data.Patches) != 0 {
		t.Errorf("expected 0 sub-patches for unchanged content, got %d", len(data.Patches))
	}
}

func TestDiff_KeyedRemoveMiddle_Negative(t *testing.T) {
	// Removing "b" from [a, b, c] should NOT touch "a" or "c".
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)

	// Only "b" should be removed.
	if len(data.Removes) != 1 {
		t.Fatalf("expected 1 remove, got %d: %+v", len(data.Removes), data.Removes)
	}
	for _, r := range data.Removes {
		if r.Key == "a" || r.Key == "c" {
			t.Errorf("surviving key %q should not be in removes", r.Key)
		}
	}
	// No inserts for a pure remove.
	if len(data.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %d", len(data.Inserts))
	}
	// No content patches.
	if len(data.Patches) != 0 {
		t.Errorf("expected 0 sub-patches, got %d", len(data.Patches))
	}
}

func TestDiff_KeyedSwap(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "a", Node: &TextNode{Text: "A"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	// Should be move operations, not redraws.
	// Moves use inserts without Node (nil) + removes with the same key.
	for _, ins := range data.Inserts {
		if ins.Node != nil {
			t.Errorf("move insert should have nil Node, got non-nil for key %q", ins.Key)
		}
	}
}

func TestDiff_KeyedReverse(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "c", Node: &TextNode{Text: "C"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "a", Node: &TextNode{Text: "A"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchReorder {
		t.Fatalf("expected PatchReorder, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchReorderData)
	// All inserts should be moves (nil Node), no new HTML.
	for _, ins := range data.Inserts {
		if ins.Node != nil {
			t.Errorf("reverse move should have nil Node for key %q", ins.Key)
		}
	}
	// No PatchRedraw should appear — content hasn't changed, just order.
	if len(data.Patches) != 0 {
		t.Errorf("expected 0 sub-patches (content unchanged), got %d: %+v", len(data.Patches), data.Patches)
	}
}

func TestDiff_KeyedReplaceAll(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "x", Node: &TextNode{Text: "X"}},
			{Key: "y", Node: &TextNode{Text: "Y"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	// All old removed, all new inserted.
	if len(data.Removes) != 2 {
		t.Errorf("expected 2 removes, got %d", len(data.Removes))
	}
	if len(data.Inserts) != 2 {
		t.Errorf("expected 2 inserts, got %d", len(data.Inserts))
	}
	// New inserts should have Node set (they need HTML).
	for _, ins := range data.Inserts {
		if ins.Node == nil {
			t.Errorf("new insert for key %q should have Node", ins.Key)
		}
	}
}

func TestDiff_KeyedMoveWithContentChange(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "b", Node: &TextNode{Text: "B-updated"}},
			{Key: "a", Node: &TextNode{Text: "A"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	// "b" moved to front AND its content changed.
	// Should have a sub-patch for the text change.
	if len(data.Patches) != 1 {
		t.Fatalf("expected 1 sub-patch for content change, got %d: %+v", len(data.Patches), data.Patches)
	}
	if data.Patches[0].Type != PatchText {
		t.Errorf("expected PatchText sub-patch, got %d", data.Patches[0].Type)
	}
	textData := data.Patches[0].Data.(PatchTextData)
	if textData.Text != "B-updated" {
		t.Errorf("expected text 'B-updated', got %q", textData.Text)
	}
}

func TestDiff_KeyedInsertMiddle(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
			{Key: "c", Node: &TextNode{Text: "C"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	if len(data.Inserts) != 1 || data.Inserts[0].Key != "b" {
		t.Errorf("expected insert of 'b', got %+v", data.Inserts)
	}
	if data.Inserts[0].Index != 1 {
		t.Errorf("expected insert at index 1, got %d", data.Inserts[0].Index)
	}
	if data.Inserts[0].Node == nil {
		t.Errorf("new insert should have Node for HTML")
	}
	if len(data.Removes) != 0 {
		t.Errorf("expected 0 removes, got %+v", data.Removes)
	}
}

func TestDiff_KeyedEmpty(t *testing.T) {
	old := &KeyedElementNode{
		Tag:      "ul",
		Children: []KeyedChild{},
	}
	new := &KeyedElementNode{
		Tag:      "ul",
		Children: []KeyedChild{},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for identical empty lists, got %d", len(patches))
	}
}

func TestDiff_KeyedEmptyToFull(t *testing.T) {
	old := &KeyedElementNode{
		Tag:      "ul",
		Children: []KeyedChild{},
	}
	new := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	if len(data.Inserts) != 2 {
		t.Errorf("expected 2 inserts, got %d", len(data.Inserts))
	}
}

func TestDiff_KeyedFullToEmpty(t *testing.T) {
	old := &KeyedElementNode{
		Tag: "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{Text: "A"}},
			{Key: "b", Node: &TextNode{Text: "B"}},
		},
	}
	new := &KeyedElementNode{
		Tag:      "ul",
		Children: []KeyedChild{},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	data := patches[0].Data.(PatchReorderData)
	if len(data.Removes) != 2 {
		t.Errorf("expected 2 removes, got %d", len(data.Removes))
	}
	if len(data.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %d", len(data.Inserts))
	}
}

// ---------------------------------------------------------------------------
// Complex tree diff
// ---------------------------------------------------------------------------

func TestDiff_ComplexTree(t *testing.T) {
	// Simulates a counter app: count changes from 5 to 10
	old := makeDiv(
		&ElementNode{
			Tag: "h1",
			Children: []Node{
				&ElementNode{
					Tag:   "span",
					Facts: Facts{Props: map[string]any{"data-gid": "g1"}},
					Children: []Node{
						&TextNode{Text: "5"},
					},
				},
			},
		},
		&ElementNode{
			Tag: "button",
			Facts: Facts{
				Events: map[string]EventHandler{
					"click": {Handler: "Increment"},
				},
			},
			Children: []Node{&TextNode{Text: "+"}},
		},
	)
	new := makeDiv(
		&ElementNode{
			Tag: "h1",
			Children: []Node{
				&ElementNode{
					Tag:   "span",
					Facts: Facts{Props: map[string]any{"data-gid": "g1"}},
					Children: []Node{
						&TextNode{Text: "10"},
					},
				},
			},
		},
		&ElementNode{
			Tag: "button",
			Facts: Facts{
				Events: map[string]EventHandler{
					"click": {Handler: "Increment"},
				},
			},
			Children: []Node{&TextNode{Text: "+"}},
		},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	// Only the text "5" → "10" should change
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d: %+v", len(patches), patches)
	}
	if patches[0].Type != PatchText {
		t.Errorf("expected PatchText, got %d", patches[0].Type)
	}
	data := patches[0].Data.(PatchTextData)
	if data.Text != "10" {
		t.Errorf("expected '10', got %q", data.Text)
	}
}

func TestDiff_PatchNodeID(t *testing.T) {
	// Verify that patches target the correct node IDs from the old tree.
	textB := &TextNode{NodeBase: NodeBase{ID: 30}, Text: "b"}
	textC := &TextNode{NodeBase: NodeBase{ID: 40}, Text: "c"}
	old := &ElementNode{
		NodeBase: NodeBase{ID: 10},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 20}, Text: "a"},
			&ElementNode{
				NodeBase: NodeBase{ID: 25},
				Tag:      "span",
				Children: []Node{textB},
			},
			textC,
		},
	}
	new := &ElementNode{
		NodeBase: NodeBase{ID: 100},
		Tag:      "div",
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 101}, Text: "a"},
			&ElementNode{
				NodeBase: NodeBase{ID: 102},
				Tag:      "span",
				Children: []Node{&TextNode{NodeBase: NodeBase{ID: 103}, Text: "B"}},
			},
			&TextNode{NodeBase: NodeBase{ID: 104}, Text: "C"},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d: %+v", len(patches), patches)
	}

	// "b" → "B" should target old text node ID 30
	if patches[0].NodeID != 30 {
		t.Errorf("expected NodeID 30 for 'b'→'B', got %d", patches[0].NodeID)
	}
	// "c" → "C" should target old text node ID 40
	if patches[1].NodeID != 40 {
		t.Errorf("expected NodeID 40 for 'c'→'C', got %d", patches[1].NodeID)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: parse → resolve → diff
// ---------------------------------------------------------------------------

func TestDiff_EndToEnd(t *testing.T) {
	htmlStr := `<!DOCTYPE html><html><head></head><body>
		<h1><span g-text="Count">0</span></h1>
		<div g-if="ShowPanel">panel</div>
	</body></html>`

	templates, err := ParseTemplate(htmlStr)
	if err != nil {
		t.Fatal(err)
	}

	type appState struct {
		Count     int
		ShowPanel bool
	}

	// First render
	state1 := &appState{Count: 5, ShowPanel: false}
	ctx1 := &ResolveContext{
		State: makeReflectValue(state1),
		Vars:  make(map[string]any),
	}
	tree1 := ResolveTree(templates, ctx1)
	root1 := &ElementNode{Tag: "body", Children: tree1}
	ComputeDescendants(root1)

	// Second render: count changed, panel now visible
	state2 := &appState{Count: 10, ShowPanel: true}
	ctx2 := &ResolveContext{
		State: makeReflectValue(state2),
		Vars:  make(map[string]any),
	}
	tree2 := ResolveTree(templates, ctx2)
	root2 := &ElementNode{Tag: "body", Children: tree2}
	ComputeDescendants(root2)

	patches := Diff(root1, root2)
	if len(patches) == 0 {
		t.Fatal("expected patches for count change + panel visibility")
	}

	// Should have at least a text change for count
	hasTextPatch := false
	for _, p := range patches {
		if p.Type == PatchText {
			data := p.Data.(PatchTextData)
			if data.Text == "10" {
				hasTextPatch = true
			}
		}
	}
	if !hasTextPatch {
		t.Error("expected text patch for count 5→10")
	}
}

// ---------------------------------------------------------------------------
// Section 4: Node type tests
// ---------------------------------------------------------------------------

func TestNodeType_Constants(t *testing.T) {
	tests := []struct {
		name     string
		node     Node
		wantType int
	}{
		{"TextNode", &TextNode{}, NodeText},
		{"ElementNode", &ElementNode{}, NodeElement},
		{"KeyedElementNode", &KeyedElementNode{}, NodeKeyed},
		{"PluginNode", &PluginNode{}, NodePlugin},
		{"LazyNode", &LazyNode{}, NodeLazy},
	}
	for _, tt := range tests {
		if tt.node.NodeType() != tt.wantType {
			t.Errorf("%s.NodeType() = %d, want %d", tt.name, tt.node.NodeType(), tt.wantType)
		}
	}
}

func TestFindNodeByID(t *testing.T) {
	t.Run("nil root", func(t *testing.T) {
		if FindNodeByID(nil, 1) != nil {
			t.Error("expected nil for nil root")
		}
	})

	t.Run("root matches", func(t *testing.T) {
		n := &TextNode{NodeBase: NodeBase{ID: 5}}
		if FindNodeByID(n, 5) != n {
			t.Error("expected root to match")
		}
	})

	t.Run("ElementNode children", func(t *testing.T) {
		child := &TextNode{NodeBase: NodeBase{ID: 10}, Text: "found"}
		root := &ElementNode{
			NodeBase: NodeBase{ID: 1},
			Tag:      "div",
			Children: []Node{
				&TextNode{NodeBase: NodeBase{ID: 2}},
				&ElementNode{
					NodeBase: NodeBase{ID: 3},
					Tag:      "span",
					Children: []Node{child},
				},
			},
		}
		if found := FindNodeByID(root, 10); found != child {
			t.Errorf("expected to find child, got %v", found)
		}
	})

	t.Run("KeyedElementNode children", func(t *testing.T) {
		child := &TextNode{NodeBase: NodeBase{ID: 20}, Text: "keyed"}
		root := &KeyedElementNode{
			NodeBase: NodeBase{ID: 1},
			Tag:      "ul",
			Children: []KeyedChild{
				{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 10}}},
				{Key: "b", Node: child},
			},
		}
		if found := FindNodeByID(root, 20); found != child {
			t.Errorf("expected to find keyed child, got %v", found)
		}
	})

	t.Run("LazyNode cached", func(t *testing.T) {
		inner := &TextNode{NodeBase: NodeBase{ID: 40}, Text: "lazy"}
		root := &LazyNode{
			NodeBase: NodeBase{ID: 1},
			Cached:   inner,
		}
		if found := FindNodeByID(root, 40); found != inner {
			t.Errorf("expected to find in lazy cached, got %v", found)
		}
	})

	t.Run("LazyNode nil cached", func(t *testing.T) {
		root := &LazyNode{NodeBase: NodeBase{ID: 1}, Cached: nil}
		if found := FindNodeByID(root, 99); found != nil {
			t.Error("expected nil for nil cached")
		}
	})

	t.Run("not found", func(t *testing.T) {
		root := &ElementNode{
			NodeBase: NodeBase{ID: 1},
			Tag:      "div",
			Children: []Node{&TextNode{NodeBase: NodeBase{ID: 2}}},
		}
		if found := FindNodeByID(root, 999); found != nil {
			t.Error("expected nil when ID not found")
		}
	})
}

func TestComputeDescendants_AllNodeTypes(t *testing.T) {
	t.Run("TextNode", func(t *testing.T) {
		n := &TextNode{Text: "hello"}
		count := ComputeDescendants(n)
		if count != 0 || n.Descendants != 0 {
			t.Errorf("expected 0, got count=%d cached=%d", count, n.Descendants)
		}
	})

	t.Run("KeyedElementNode", func(t *testing.T) {
		n := &KeyedElementNode{
			Tag: "ul",
			Children: []KeyedChild{
				{Key: "a", Node: &TextNode{Text: "A"}},
				{Key: "b", Node: &ElementNode{Tag: "li", Children: []Node{&TextNode{Text: "B"}}}},
			},
		}
		count := ComputeDescendants(n)
		// 2 direct children + 1 grandchild = 3
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("PluginNode", func(t *testing.T) {
		n := &PluginNode{Tag: "canvas", Name: "chart"}
		count := ComputeDescendants(n)
		if count != 0 || n.Descendants != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("LazyNode with cached", func(t *testing.T) {
		n := &LazyNode{
			Cached: &TextNode{Text: "cached"},
		}
		count := ComputeDescendants(n)
		// 1 (cached text node) + 0 descendants of text = 1
		if count != 1 {
			t.Errorf("expected 1, got %d", count)
		}
	})

	t.Run("LazyNode nil cached", func(t *testing.T) {
		n := &LazyNode{Cached: nil}
		count := ComputeDescendants(n)
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("unknown Node type", func(t *testing.T) {
		n := &fakeNode{}
		count := ComputeDescendants(n)
		if count != 0 {
			t.Errorf("expected 0 for unknown node type, got %d", count)
		}
	})
}

// fakeNode is a test-only Node implementation to exercise the default
// branch in ComputeDescendants (unreachable with real node types).
type fakeNode struct{}

func (f *fakeNode) NodeType() int         { return -1 }
func (f *fakeNode) NodeID() int           { return 0 }
func (f *fakeNode) DescendantsCount() int { return 0 }

func TestLazyNode_DescendantsCount(t *testing.T) {
	t.Run("with cached", func(t *testing.T) {
		cached := &TextNode{Text: "x"}
		ComputeDescendants(cached)
		n := &LazyNode{Cached: cached}
		if n.DescendantsCount() != 1+cached.DescendantsCount() {
			t.Errorf("expected %d, got %d", 1+cached.DescendantsCount(), n.DescendantsCount())
		}
	})

	t.Run("nil cached", func(t *testing.T) {
		n := &LazyNode{Cached: nil}
		if n.DescendantsCount() != 0 {
			t.Errorf("expected 0, got %d", n.DescendantsCount())
		}
	})
}

// ---------------------------------------------------------------------------
// Section 5: Diff engine tests
// ---------------------------------------------------------------------------

func TestDiffFacts_NSAttr(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		old := Facts{}
		new := Facts{AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#icon"}}}
		d := DiffFacts(&old, &new)
		if d.AttrsNS["xlink:href"].Value != "#icon" {
			t.Errorf("expected add of xlink:href, got %v", d.AttrsNS)
		}
	})

	t.Run("remove", func(t *testing.T) {
		old := Facts{AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#icon"}}}
		new := Facts{}
		d := DiffFacts(&old, &new)
		removed, ok := d.AttrsNS["xlink:href"]
		if !ok {
			t.Fatal("expected xlink:href in diff")
		}
		if removed != (NSAttr{}) {
			t.Errorf("expected empty NSAttr for removal, got %+v", removed)
		}
	})

	t.Run("change value", func(t *testing.T) {
		old := Facts{AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#a"}}}
		new := Facts{AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#b"}}}
		d := DiffFacts(&old, &new)
		if d.AttrsNS["xlink:href"].Value != "#b" {
			t.Errorf("expected value '#b', got %q", d.AttrsNS["xlink:href"].Value)
		}
	})

	t.Run("identical returns nil", func(t *testing.T) {
		ns := map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#a"}}
		old := Facts{AttrsNS: ns}
		new := Facts{AttrsNS: map[string]NSAttr{"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#a"}}}
		d := DiffFacts(&old, &new)
		if d.AttrsNS != nil {
			t.Errorf("expected nil AttrsNS diff for identical, got %v", d.AttrsNS)
		}
	})

	t.Run("both empty returns nil", func(t *testing.T) {
		d := DiffFacts(&Facts{}, &Facts{})
		if d.AttrsNS != nil {
			t.Error("expected nil for both empty")
		}
	})
}

func TestValEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b any
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil vs non-nil", nil, "x", false},
		{"non-nil vs nil", "x", nil, false},
		{"same string", "a", "a", true},
		{"diff string", "a", "b", false},
		{"same bool", true, true, true},
		{"diff bool", true, false, false},
		{"same int", 1, 1, true},
		{"diff int", 1, 2, false},
		{"same float64", 1.5, 1.5, true},
		{"diff float64", 1.5, 2.5, false},
		{"string vs int", "1", 1, false},
		{"int vs float64", 1, 1.0, false},
		// DeepEqual fallback: slices of structs
		{"same slice of structs",
			[]struct{ Name string }{{"alice"}, {"bob"}},
			[]struct{ Name string }{{"alice"}, {"bob"}},
			true},
		{"diff slice of structs",
			[]struct{ Name string }{{"alice"}, {"bob"}},
			[]struct{ Name string }{{"alice"}, {"carol"}},
			false},
		// DeepEqual fallback: map with non-primitive values
		{"same map of slices",
			map[string][]int{"a": {1, 2}},
			map[string][]int{"a": {1, 2}},
			true},
		{"diff map of slices",
			map[string][]int{"a": {1, 2}},
			map[string][]int{"a": {1, 3}},
			false},
	}
	for _, tt := range tests {
		got := valEqual(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("valEqual(%s): got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRefEqual(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		if !refEqual(nil, nil) {
			t.Error("expected true for both nil")
		}
	})

	t.Run("nil vs non-nil", func(t *testing.T) {
		if refEqual(nil, 42) {
			t.Error("expected false for nil vs non-nil")
		}
	})

	t.Run("same pointer", func(t *testing.T) {
		x := &struct{}{}
		if !refEqual(x, x) {
			t.Error("expected true for same pointer")
		}
	})

	t.Run("different pointers", func(t *testing.T) {
		x := &struct{ v int }{1}
		y := &struct{ v int }{1}
		if refEqual(x, y) {
			t.Error("expected false for different pointers")
		}
	})

	t.Run("same slice", func(t *testing.T) {
		s := []int{1, 2, 3}
		if !refEqual(s, s) {
			t.Error("expected true for same slice")
		}
	})

	t.Run("same map", func(t *testing.T) {
		m := map[string]int{"a": 1}
		if !refEqual(m, m) {
			t.Error("expected true for same map")
		}
	})

	t.Run("same value type", func(t *testing.T) {
		if !refEqual(42, 42) {
			t.Error("expected true for same value")
		}
	})

	t.Run("different value type", func(t *testing.T) {
		if refEqual(42, 43) {
			t.Error("expected false for different values")
		}
	})

	t.Run("different types", func(t *testing.T) {
		if refEqual(42, "42") {
			t.Error("expected false for different types")
		}
	})
}

func TestSortRemovesDesc(t *testing.T) {
	t.Run("unsorted", func(t *testing.T) {
		removes := []ReorderRemove{{Index: 1}, {Index: 3}, {Index: 2}}
		sortRemovesDesc(removes)
		if removes[0].Index != 3 || removes[1].Index != 2 || removes[2].Index != 1 {
			t.Errorf("expected [3,2,1], got [%d,%d,%d]", removes[0].Index, removes[1].Index, removes[2].Index)
		}
	})

	t.Run("already descending", func(t *testing.T) {
		removes := []ReorderRemove{{Index: 3}, {Index: 2}, {Index: 1}}
		sortRemovesDesc(removes)
		if removes[0].Index != 3 || removes[1].Index != 2 || removes[2].Index != 1 {
			t.Errorf("expected [3,2,1], got [%d,%d,%d]", removes[0].Index, removes[1].Index, removes[2].Index)
		}
	})

	t.Run("single element", func(t *testing.T) {
		removes := []ReorderRemove{{Index: 5}}
		sortRemovesDesc(removes)
		if removes[0].Index != 5 {
			t.Errorf("expected [5], got [%d]", removes[0].Index)
		}
	})

	t.Run("empty", func(t *testing.T) {
		var removes []ReorderRemove
		sortRemovesDesc(removes) // should not panic
	})
}

func TestSliceHelpers(t *testing.T) {
	t.Run("sliceInsert at start", func(t *testing.T) {
		s := sliceInsert([]string{"b", "c"}, 0, "a")
		if len(s) != 3 || s[0] != "a" || s[1] != "b" || s[2] != "c" {
			t.Errorf("expected [a,b,c], got %v", s)
		}
	})

	t.Run("sliceInsert at middle", func(t *testing.T) {
		s := sliceInsert([]string{"a", "c"}, 1, "b")
		if len(s) != 3 || s[1] != "b" {
			t.Errorf("expected [a,b,c], got %v", s)
		}
	})

	t.Run("sliceInsert beyond length appends", func(t *testing.T) {
		s := sliceInsert([]string{"a"}, 5, "z")
		if len(s) != 2 || s[1] != "z" {
			t.Errorf("expected [a,z], got %v", s)
		}
	})

	t.Run("sliceRemove first", func(t *testing.T) {
		s := sliceRemove([]string{"a", "b", "c"}, 0)
		if len(s) != 2 || s[0] != "b" {
			t.Errorf("expected [b,c], got %v", s)
		}
	})

	t.Run("sliceRemove last", func(t *testing.T) {
		s := sliceRemove([]string{"a", "b", "c"}, 2)
		if len(s) != 2 || s[1] != "b" {
			t.Errorf("expected [a,b], got %v", s)
		}
	})

	t.Run("sliceIndexOf found", func(t *testing.T) {
		if sliceIndexOf([]string{"a", "b", "c"}, "b") != 1 {
			t.Error("expected index 1")
		}
	})

	t.Run("sliceIndexOf not found", func(t *testing.T) {
		if sliceIndexOf([]string{"a", "b"}, "z") != -1 {
			t.Error("expected -1")
		}
	})
}

func TestDiff_KeyedElementTagChange(t *testing.T) {
	old := &KeyedElementNode{Tag: "ul", Children: []KeyedChild{{Key: "a", Node: &TextNode{Text: "A"}}}}
	new := &KeyedElementNode{Tag: "ol", Children: []KeyedChild{{Key: "a", Node: &TextNode{Text: "A"}}}}
	ComputeDescendants(old)
	ComputeDescendants(new)
	patches := Diff(old, new)
	if len(patches) != 1 || patches[0].Type != PatchRedraw {
		t.Errorf("expected PatchRedraw for keyed tag change, got %+v", patches)
	}
}

func TestDiff_KeyedElementNamespaceChange(t *testing.T) {
	old := &KeyedElementNode{Tag: "g", Namespace: ""}
	new := &KeyedElementNode{Tag: "g", Namespace: "http://www.w3.org/2000/svg"}
	patches := Diff(old, new)
	if len(patches) != 1 || patches[0].Type != PatchRedraw {
		t.Errorf("expected PatchRedraw for keyed namespace change, got %+v", patches)
	}
}

func TestDiff_KeyedElementFactsChange(t *testing.T) {
	old := &KeyedElementNode{
		Tag:      "ul",
		Facts:    Facts{Props: map[string]any{"className": "old"}},
		Children: []KeyedChild{},
	}
	new := &KeyedElementNode{
		Tag:      "ul",
		Facts:    Facts{Props: map[string]any{"className": "new"}},
		Children: []KeyedChild{},
	}
	patches := Diff(old, new)
	if len(patches) != 1 || patches[0].Type != PatchFacts {
		t.Errorf("expected PatchFacts for keyed facts change, got %+v", patches)
	}
}

func TestDiff_LazyNilFunc(t *testing.T) {
	// BUG: lazyArgsEqual panics on nil Func because
	// reflect.ValueOf(nil).Pointer() panics. lazyArgsEqual should check
	// for nil Func before comparing pointers.
	old := &LazyNode{NodeBase: NodeBase{ID: 1}, Func: nil}
	new := &LazyNode{NodeBase: NodeBase{ID: 1}, Func: nil}
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected no patches for identical nil-func lazy nodes, got %+v", patches)
	}
}

func TestDiff_LazyOneNilFunc(t *testing.T) {
	// One nil Func, one non-nil → lazyArgsEqual returns false (line 372-373)
	fn := func() Node { return &TextNode{Text: "a"} }
	old := &LazyNode{NodeBase: NodeBase{ID: 1}, Func: nil, Cached: &TextNode{NodeBase: NodeBase{ID: 2}, Text: "a"}}
	new := &LazyNode{NodeBase: NodeBase{ID: 1}, Func: fn}
	patches := Diff(old, new)
	// Different funcs → evaluateLazy called → produces patches
	_ = patches // verify no panic; funcs differ so lazy args not equal
}

func TestDiff_LazyDifferentFuncs(t *testing.T) {
	fn1 := func(n int) Node { return &TextNode{Text: "a"} }
	fn2 := func(n int) Node { return &TextNode{Text: "b"} }
	old := &LazyNode{Func: fn1, Args: []any{1}, Cached: &TextNode{Text: "a"}}
	new := &LazyNode{Func: fn2, Args: []any{1}, Cached: nil}
	patches := Diff(old, new)
	if len(patches) != 1 || patches[0].Type != PatchLazy {
		t.Errorf("expected PatchLazy for different funcs, got %+v", patches)
	}
}

func TestDiff_LazyNonFuncEvaluate(t *testing.T) {
	// Func is not a function — evaluateLazy returns nil
	old := &LazyNode{Func: func(n int) Node { return &TextNode{Text: "old"} }, Args: []any{1}, Cached: &TextNode{Text: "old"}}
	new := &LazyNode{Func: "not a function", Args: []any{2}, Cached: nil}
	patches := Diff(old, new)
	// new.Cached will be nil after evaluateLazy (non-func), old.Cached is non-nil
	// With old cached and new nil cached: no patches (both cached already compared)
	_ = patches // just verify no panic
}

func TestDiff_LazyNoReturnValue(t *testing.T) {
	// Function returns nothing — evaluateLazy returns nil
	fn1 := func(n int) Node { return &TextNode{Text: "x"} }
	fn2 := func() {} // no return value
	old := &LazyNode{Func: fn1, Args: []any{1}, Cached: &TextNode{Text: "x"}}
	new := &LazyNode{Func: fn2, Args: nil, Cached: nil}
	patches := Diff(old, new)
	_ = patches // verify no panic
}

func TestDiff_LazyOldNilCachedNewHasCached(t *testing.T) {
	fn := func(n int) Node { return &TextNode{Text: "result"} }
	old := &LazyNode{Func: nil, Cached: nil}
	new := &LazyNode{Func: fn, Args: []any{1}, Cached: &TextNode{Text: "result"}}
	patches := Diff(old, new)
	// old.Cached nil, new.Cached non-nil → redraw
	if len(patches) != 1 || patches[0].Type != PatchRedraw {
		t.Errorf("expected PatchRedraw, got %+v", patches)
	}
}

func TestDiff_PluginFactsChange(t *testing.T) {
	old := &PluginNode{Tag: "canvas", Name: "chart", Facts: Facts{Props: map[string]any{"id": "a"}}, Data: nil}
	new := &PluginNode{Tag: "canvas", Name: "chart", Facts: Facts{Props: map[string]any{"id": "b"}}, Data: nil}
	patches := Diff(old, new)
	if len(patches) != 1 || patches[0].Type != PatchFacts {
		t.Errorf("expected PatchFacts for plugin facts change, got %+v", patches)
	}
}

func TestArgsEqual(t *testing.T) {
	t.Run("different lengths", func(t *testing.T) {
		if argsEqual([]any{1}, []any{1, 2}) {
			t.Error("expected false for different lengths")
		}
	})
	t.Run("same", func(t *testing.T) {
		if !argsEqual([]any{1, "a"}, []any{1, "a"}) {
			t.Error("expected true for same args")
		}
	})
	t.Run("both nil", func(t *testing.T) {
		if !argsEqual(nil, nil) {
			t.Error("expected true for both nil")
		}
	})
}

func TestJsonEqual(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		if !jsonEqual(nil, nil) {
			t.Error("expected true")
		}
	})
	t.Run("one nil", func(t *testing.T) {
		if jsonEqual(nil, 1) {
			t.Error("expected false")
		}
	})
	t.Run("equal maps", func(t *testing.T) {
		if !jsonEqual(map[string]int{"a": 1}, map[string]int{"a": 1}) {
			t.Error("expected true")
		}
	})
	t.Run("different maps", func(t *testing.T) {
		if jsonEqual(map[string]int{"a": 1}, map[string]int{"a": 2}) {
			t.Error("expected false")
		}
	})
}

func TestDiff_ElementNamespaceChange(t *testing.T) {
	t.Run("namespace change redraws", func(t *testing.T) {
		old := &ElementNode{Tag: "rect", Namespace: ""}
		new := &ElementNode{Tag: "rect", Namespace: "http://www.w3.org/2000/svg"}
		patches := Diff(old, new)
		if len(patches) != 1 || patches[0].Type != PatchRedraw {
			t.Errorf("expected PatchRedraw for namespace change, got %+v", patches)
		}
	})

	t.Run("same namespace no redraw", func(t *testing.T) {
		old := &ElementNode{Tag: "rect", Namespace: "http://www.w3.org/2000/svg"}
		new := &ElementNode{Tag: "rect", Namespace: "http://www.w3.org/2000/svg"}
		patches := Diff(old, new)
		if len(patches) != 0 {
			t.Errorf("expected 0 patches for same namespace, got %+v", patches)
		}
	})
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestDiffFacts_StringAttrsAddRemoveChange(t *testing.T) {
	// Exercises diffMapString: add, remove, and change paths
	old := &Facts{
		Attrs: map[string]string{"keep": "same", "remove": "old", "change": "v1"},
	}
	new := &Facts{
		Attrs: map[string]string{"keep": "same", "add": "new", "change": "v2"},
	}
	d := DiffFacts(old, new)
	if d.Attrs == nil {
		t.Fatal("expected attrs diff")
	}
	if d.Attrs["remove"] != "" {
		t.Errorf("removed attr should be empty string, got %q", d.Attrs["remove"])
	}
	if d.Attrs["add"] != "new" {
		t.Errorf("added attr should be 'new', got %q", d.Attrs["add"])
	}
	if d.Attrs["change"] != "v2" {
		t.Errorf("changed attr should be 'v2', got %q", d.Attrs["change"])
	}
	if _, ok := d.Attrs["keep"]; ok {
		t.Error("unchanged attr should not be in diff")
	}
}

func TestDiffFacts_StylesAddRemoveChange(t *testing.T) {
	// Same pattern for Styles (also uses diffMapString)
	old := &Facts{
		Styles: map[string]string{"color": "red", "width": "100px"},
	}
	new := &Facts{
		Styles: map[string]string{"color": "blue", "height": "50px"},
	}
	d := DiffFacts(old, new)
	if d.Styles == nil {
		t.Fatal("expected styles diff")
	}
	if d.Styles["width"] != "" {
		t.Errorf("removed style should be empty, got %q", d.Styles["width"])
	}
	if d.Styles["color"] != "blue" {
		t.Errorf("changed style should be 'blue', got %q", d.Styles["color"])
	}
	if d.Styles["height"] != "50px" {
		t.Errorf("added style should be '50px', got %q", d.Styles["height"])
	}
}

func TestArgsEqual_DifferentLengths(t *testing.T) {
	// Exercises argsEqual early return for different lengths
	old := &Facts{
		Events: map[string]EventHandler{
			"click": {Handler: "Save", Args: []any{"a", "b"}},
		},
	}
	new := &Facts{
		Events: map[string]EventHandler{
			"click": {Handler: "Save", Args: []any{"a"}},
		},
	}
	d := DiffFacts(old, new)
	if d.Events == nil {
		t.Fatal("expected events diff due to different arg lengths")
	}
}

func TestValEqual_Float64(t *testing.T) {
	// Exercises valEqual float64 path via Props diff
	old := &Facts{Props: map[string]any{"opacity": float64(0.5)}}
	new := &Facts{Props: map[string]any{"opacity": float64(0.5)}}
	d := DiffFacts(old, new)
	if d.Props != nil {
		t.Error("identical float64 props should produce no diff")
	}

	new2 := &Facts{Props: map[string]any{"opacity": float64(0.8)}}
	d2 := DiffFacts(old, new2)
	if d2.Props == nil {
		t.Error("different float64 props should produce diff")
	}
}

func TestDiff_KeyedChildrenSubPatchesOnly(t *testing.T) {
	// Exercises the path where keyed children have same keys in same order
	// but child content differs → sub-patches only, no reorder patch
	old := &KeyedElementNode{
		NodeBase: NodeBase{ID: 1},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 2}, Text: "old-a"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 3}, Text: "old-b"}},
		},
	}
	new := &KeyedElementNode{
		NodeBase: NodeBase{ID: 1},
		Tag:      "ul",
		Children: []KeyedChild{
			{Key: "a", Node: &TextNode{NodeBase: NodeBase{ID: 2}, Text: "new-a"}},
			{Key: "b", Node: &TextNode{NodeBase: NodeBase{ID: 3}, Text: "new-b"}},
		},
	}
	ComputeDescendants(old)
	ComputeDescendants(new)
	patches := Diff(old, new)
	// Should have text patches but no reorder patch
	for _, p := range patches {
		if p.Type == PatchReorder {
			t.Error("expected no reorder patch when keys are unchanged")
		}
	}
	if len(patches) == 0 {
		t.Error("expected text change patches")
	}
}

func TestDiff_LazyReturnNonNode(t *testing.T) {
	// Exercises evaluateLazy where func returns non-Node value
	fn1 := func() string { return "not a node" }
	fn2 := func() string { return "also not a node" }
	old := &LazyNode{
		NodeBase: NodeBase{ID: 1},
		Func:     fn1,
	}
	new := &LazyNode{
		NodeBase: NodeBase{ID: 1},
		Func:     fn2,
	}
	patches := Diff(old, new)
	// Different funcs → evaluateLazy called; fn returns string not Node
	// → evaluateLazy returns nil → diffHelp gets (nil, nil) → no patches
	_ = patches // just verify no panic
}

func TestJsonEqual_MarshalError(t *testing.T) {
	// Exercises jsonEqual fallback when Marshal fails (e.g. channel)
	// jsonEqual is called from diffPlugin, not DiffFacts
	ch := make(chan int)
	old := &PluginNode{
		NodeBase: NodeBase{ID: 1},
		Tag:      "div",
		Name:     "chart",
		Data:     ch,
	}
	new := &PluginNode{
		NodeBase: NodeBase{ID: 1},
		Tag:      "div",
		Name:     "chart",
		Data:     ch,
	}
	// Same channel → fmt.Sprint produces same string → no patch
	patches := Diff(old, new)
	_ = patches // just verify no panic
}

func TestDiffFacts_EventArgsDiffer(t *testing.T) {
	// Exercises argsEqual loop body where valEqual returns false
	old := &Facts{
		Events: map[string]EventHandler{
			"click": {Handler: "Save", Args: []any{"a", "b"}},
		},
	}
	new := &Facts{
		Events: map[string]EventHandler{
			"click": {Handler: "Save", Args: []any{"a", "c"}},
		},
	}
	d := DiffFacts(old, new)
	if d.Events == nil {
		t.Fatal("expected events diff when args differ at same index")
	}
}

func TestDiffFacts_StringAttrsAddOnly(t *testing.T) {
	// Exercises diffMapString add-only path (old is empty)
	old := &Facts{Attrs: map[string]string{}}
	new := &Facts{Attrs: map[string]string{"data-x": "1"}}
	d := DiffFacts(old, new)
	if d.Attrs == nil {
		t.Fatal("expected attrs diff when adding new key to empty map")
	}
	if d.Attrs["data-x"] != "1" {
		t.Error("expected added attr")
	}
}

// ---------------------------------------------------------------------------
// Regression: g-if removal + g-hide sibling style update
//
// Simulates the drag-demo scenario:
//   Old: div.canvas has 2 children — [g-if wrapper div, canvas-empty div(display:none)]
//   New: div.canvas has 1 child — [canvas-empty div (no display:none)]
//
// The index-based diff compares old[0] (wrapper) with new[0] (canvas-empty)
// and removes the last child. This test verifies the patches are correct.
// ---------------------------------------------------------------------------

func TestDiff_GIfRemovalWithSiblingStyleChange(t *testing.T) {
	// Old tree: div.canvas with 2 children
	gifWrapper := &ElementNode{
		NodeBase: NodeBase{ID: 2},
		Tag:      "div",
		Children: []Node{
			&ElementNode{
				NodeBase: NodeBase{ID: 3},
				Tag:      "div",
				Facts: Facts{
					Attrs:  map[string]string{"class": "canvas-card"},
					Styles: map[string]string{"background-color": "#2980b9"},
				},
				Children: []Node{
					&ElementNode{
						NodeBase: NodeBase{ID: 4},
						Tag:      "span",
						Children: []Node{&TextNode{NodeBase: NodeBase{ID: 5}, Text: "Blue"}},
					},
					&ElementNode{
						NodeBase: NodeBase{ID: 6},
						Tag:      "span",
						Facts:    Facts{Attrs: map[string]string{"class": "remove"}},
					},
				},
			},
		},
	}
	canvasEmptyOld := &ElementNode{
		NodeBase: NodeBase{ID: 7},
		Tag:      "div",
		Facts: Facts{
			Attrs:  map[string]string{"class": "canvas-empty"},
			Styles: map[string]string{"display": "none"},
		},
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 8}, Text: "Drag colors from the palette"},
		},
	}
	oldRoot := &ElementNode{
		NodeBase: NodeBase{ID: 1},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "canvas"}},
		Children: []Node{gifWrapper, canvasEmptyOld},
	}

	// New tree: div.canvas with 1 child (canvas-empty, no display:none)
	canvasEmptyNew := &ElementNode{
		NodeBase: NodeBase{ID: 9},
		Tag:      "div",
		Facts: Facts{
			Attrs: map[string]string{"class": "canvas-empty"},
			// No display:none — g-hide is falsy
		},
		Children: []Node{
			&TextNode{NodeBase: NodeBase{ID: 10}, Text: "Drag colors from the palette"},
		},
	}
	newRoot := &ElementNode{
		NodeBase: NodeBase{ID: 11},
		Tag:      "div",
		Facts:    Facts{Attrs: map[string]string{"class": "canvas"}},
		Children: []Node{canvasEmptyNew},
	}

	ComputeDescendants(oldRoot)
	ComputeDescendants(newRoot)

	patches := Diff(oldRoot, newRoot)

	// Log all patches for diagnosis
	for i, p := range patches {
		t.Logf("patch[%d]: Type=%d NodeID=%d Data=%+v", i, p.Type, p.NodeID, p.Data)
	}

	// Simulate bridge-side patch application to verify correctness.
	// Track what the DOM would look like after applying patches.
	//
	// After patching, div.canvas should have 1 child that:
	//   - has class="canvas-empty"
	//   - does NOT have display:none
	//   - contains text "Drag colors from the palette"
	//
	// Verify the patches make this possible:
	// 1. PatchFacts on ID:2 should add class="canvas-empty"
	// 2. Something should replace/update the children to show the text
	// 3. PatchRemoveLast should remove the old canvas-empty (with display:none)

	hasFacts2 := false
	hasRedraw3 := false
	hasRemoveLast1 := false

	for _, p := range patches {
		switch {
		case p.Type == PatchFacts && p.NodeID == 2:
			hasFacts2 = true
			fd := p.Data.(PatchFactsData)
			if fd.Diff.Attrs["class"] != "canvas-empty" {
				t.Errorf("PatchFacts(2): expected class='canvas-empty', got attrs=%v", fd.Diff.Attrs)
			}
		case p.Type == PatchRedraw && p.NodeID == 3:
			hasRedraw3 = true
			rd := p.Data.(PatchRedrawData)
			tn, ok := rd.Node.(*TextNode)
			if !ok {
				t.Errorf("PatchRedraw(3): expected TextNode replacement, got %T", rd.Node)
			} else if tn.Text != "Drag colors from the palette" {
				t.Errorf("PatchRedraw(3): expected text 'Drag colors from the palette', got %q", tn.Text)
			}
		case p.Type == PatchRemoveLast && p.NodeID == 1:
			hasRemoveLast1 = true
			rld := p.Data.(PatchRemoveLastData)
			if rld.Count != 1 {
				t.Errorf("PatchRemoveLast(1): expected count=1, got %d", rld.Count)
			}
		}
	}

	if !hasFacts2 {
		t.Error("missing PatchFacts for node 2 (g-if wrapper → canvas-empty class)")
	}
	if !hasRedraw3 {
		t.Error("missing PatchRedraw for node 3 (canvas-card → text node)")
	}
	if !hasRemoveLast1 {
		t.Error("missing PatchRemoveLast for node 1 (remove old canvas-empty)")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Slot boundary tests
// ---------------------------------------------------------------------------

func TestDiffElement_SlotBoundary_SkipsChildDiff(t *testing.T) {
	// Two slot nodes with different children — diff should NOT produce
	// child patches because slots are opaque boundaries.
	old := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div", IsSlot: true, SlotName: "counter",
		Children: []Node{&TextNode{NodeBase: NodeBase{ID: 2}, Text: "old"}},
	}
	new := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div", IsSlot: true, SlotName: "counter",
		Children: []Node{&TextNode{NodeBase: NodeBase{ID: 3}, Text: "new"}},
	}

	var patches []Patch
	diffElement(old, new, &patches)

	// Should produce no patches — slot children are managed by child components
	for _, p := range patches {
		if p.Type == PatchRedraw || p.Type == PatchText {
			t.Errorf("unexpected child patch for slot node: %+v", p)
		}
	}
}

func TestDiffElement_NonSlot_DiffsChildren(t *testing.T) {
	// Non-slot nodes with different children should produce patches normally.
	old := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div",
		Children: []Node{&TextNode{NodeBase: NodeBase{ID: 2}, Text: "old"}},
	}
	new := &ElementNode{
		NodeBase: NodeBase{ID: 1}, Tag: "div",
		Children: []Node{&TextNode{NodeBase: NodeBase{ID: 2}, Text: "new"}},
	}

	var patches []Patch
	diffElement(old, new, &patches)

	found := false
	for _, p := range patches {
		if p.Type == PatchText {
			found = true
		}
	}
	if !found {
		t.Error("expected PatchText for non-slot node with changed text")
	}
}

func TestIsSlotNode(t *testing.T) {
	slot := &ElementNode{IsSlot: true}
	if !IsSlotNode(slot) {
		t.Error("expected IsSlotNode to return true for slot node")
	}
	normal := &ElementNode{IsSlot: false}
	if IsSlotNode(normal) {
		t.Error("expected IsSlotNode to return false for normal node")
	}
}

func makeDiv(children ...Node) *ElementNode {
	return &ElementNode{Tag: "div", Children: children}
}

func makeReflectValue(v any) reflect.Value {
	return reflect.ValueOf(v)
}
