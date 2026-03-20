package vdom

import (
	"reflect"
	"testing"
)

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

	templates, err := ParseTemplate(htmlStr, nil)
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
// Helpers
// ---------------------------------------------------------------------------

func makeDiv(children ...Node) *ElementNode {
	return &ElementNode{Tag: "div", Children: children}
}

func makeReflectValue(v any) reflect.Value {
	return reflect.ValueOf(v)
}
