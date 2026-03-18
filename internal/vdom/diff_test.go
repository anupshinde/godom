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

func TestDiffFacts_NoChange(t *testing.T) {
	f := Facts{
		Props:  map[string]any{"id": "main"},
		Styles: map[string]string{"width": "100px"},
	}
	d := DiffFacts(&f, &f)
	if !d.IsEmpty() {
		t.Error("expected empty diff for identical facts")
	}
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
	data := map[string]int{"a": 1}
	old := &PluginNode{Tag: "canvas", Name: "chart", Data: data}
	new := &PluginNode{Tag: "canvas", Name: "chart", Data: data}
	patches := Diff(old, new)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for same plugin data, got %d", len(patches))
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
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Type != PatchAppend {
		t.Errorf("expected PatchAppend, got %d", patches[0].Type)
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

func TestDiff_PatchIndex(t *testing.T) {
	// Verify that patch indices are correct for tree traversal.
	// Tree structure:
	//   div (0)
	//     text "a" (1)
	//     span (2)
	//       text "b" (3)
	//     text "c" (4)
	old := makeDiv(
		&TextNode{Text: "a"},
		&ElementNode{
			Tag:      "span",
			Children: []Node{&TextNode{Text: "b"}},
		},
		&TextNode{Text: "c"},
	)
	new := makeDiv(
		&TextNode{Text: "a"},
		&ElementNode{
			Tag:      "span",
			Children: []Node{&TextNode{Text: "B"}},
		},
		&TextNode{Text: "C"},
	)
	ComputeDescendants(old)
	ComputeDescendants(new)

	patches := Diff(old, new)
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d: %+v", len(patches), patches)
	}

	// "b" → "B" at index 3 (div=0, "a"=1, span=2, "b"=3)
	if patches[0].Index != 3 {
		t.Errorf("expected index 3 for 'b'→'B', got %d", patches[0].Index)
	}
	// "c" → "C" at index 4
	if patches[1].Index != 4 {
		t.Errorf("expected index 4 for 'c'→'C', got %d", patches[1].Index)
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
