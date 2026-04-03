package render

// [COVERAGE GAP] json.Marshal error in EncodeInitTreeMessage (tree_encode.go) — json.Marshal
// on a WireNode struct cannot fail because all fields are JSON-serializable primitives.
// This error branch is unreachable in practice.

import (
	"encoding/json"
	"reflect"
	"testing"

	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/anupshinde/godom/internal/vdom"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// Patch encoder tests
// ---------------------------------------------------------------------------

func TestEncodePatchMessage_Text(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchText, NodeID: 3, Data: vdom.PatchTextData{Text: "hello"}},
	}
	msg := EncodePatchMessage(patches)
	if msg.Kind != "patch" {
		t.Errorf("expected type 'patch', got %q", msg.Kind)
	}
	if len(msg.Patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(msg.Patches))
	}
	dp := msg.Patches[0]
	if dp.Op != OpText {
		t.Errorf("expected op 'text', got %q", dp.Op)
	}
	if dp.NodeId != 3 {
		t.Errorf("expected node_id 3, got %d", dp.NodeId)
	}
	if dp.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", dp.Text)
	}
}

func TestEncodePatchMessage_Facts(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchFacts, NodeID: 1, Data: vdom.PatchFactsData{
			Diff: vdom.FactsDiff{
				Props:  map[string]any{"className": "active"},
				Styles: map[string]string{"display": "none"},
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpFacts {
		t.Errorf("expected op 'facts', got %q", dp.Op)
	}
	var fd WireFactsDiff
	if err := json.Unmarshal(dp.Facts, &fd); err != nil {
		t.Fatal(err)
	}
	if fd.Props["className"] != "active" {
		t.Errorf("expected className='active', got %v", fd.Props["className"])
	}
	if fd.Styles["display"] != "none" {
		t.Errorf("expected display='none', got %q", fd.Styles["display"])
	}
}

func TestEncodePatchMessage_Append(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchAppend, NodeID: 5, Data: vdom.PatchAppendData{
			Nodes: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 10}, Text: "new child"}},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpAppend {
		t.Errorf("expected op 'append', got %q", dp.Op)
	}
	if len(dp.TreeContent) == 0 {
		t.Error("expected tree content for append")
	}
	// Decode and verify it's a JSON array of tree nodes
	var trees []*WireNode
	if err := json.Unmarshal(dp.TreeContent, &trees); err != nil {
		t.Fatal(err)
	}
	if len(trees) != 1 {
		t.Fatalf("expected 1 tree node, got %d", len(trees))
	}
	if trees[0].Type != "text" {
		t.Errorf("expected text node, got %q", trees[0].Type)
	}
	if trees[0].Text != "new child" {
		t.Errorf("expected 'new child', got %q", trees[0].Text)
	}
}

func TestEncodePatchMessage_RemoveLast(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRemoveLast, NodeID: 2, Data: vdom.PatchRemoveLastData{Count: 3}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpRemoveLast {
		t.Errorf("expected op 'remove-last', got %q", dp.Op)
	}
	if dp.Count != 3 {
		t.Errorf("expected count 3, got %d", dp.Count)
	}
}

func TestEncodePatchMessage_Redraw(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRedraw, NodeID: 2, Data: vdom.PatchRedrawData{
			Node: &vdom.ElementNode{
				NodeBase: vdom.NodeBase{ID: 20},
				Tag:      "span",
				Children: []vdom.Node{&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 21}, Text: "replaced"}},
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpRedraw {
		t.Errorf("expected op 'redraw', got %q", dp.Op)
	}
	if len(dp.TreeContent) == 0 {
		t.Error("expected tree content for redraw")
	}
	// Decode and verify
	var tree WireNode
	if err := json.Unmarshal(dp.TreeContent, &tree); err != nil {
		t.Fatal(err)
	}
	if tree.Tag != "span" {
		t.Errorf("expected tag 'span', got %q", tree.Tag)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Children))
	}
}

func TestEncodePatchMessage_Plugin(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchPlugin, NodeID: 5, Data: vdom.PatchPluginData{
			Data: map[string]int{"value": 42},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpPlugin {
		t.Errorf("expected op 'plugin', got %q", dp.Op)
	}
	var data map[string]int
	if err := json.Unmarshal(dp.PluginData, &data); err != nil {
		t.Fatal(err)
	}
	if data["value"] != 42 {
		t.Errorf("expected value=42, got %d", data["value"])
	}
}

func TestEncodePatchMessage_Lazy(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchLazy, NodeID: 7, Data: vdom.PatchLazyData{
			Patches: []vdom.Patch{
				{Type: vdom.PatchText, NodeID: 8, Data: vdom.PatchTextData{Text: "inner"}},
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpLazy {
		t.Errorf("expected op 'lazy', got %q", dp.Op)
	}
	if len(dp.SubPatches) != 1 {
		t.Fatalf("expected 1 sub-patch, got %d", len(dp.SubPatches))
	}
	if dp.SubPatches[0].Op != OpText {
		t.Errorf("expected sub-patch op 'text', got %q", dp.SubPatches[0].Op)
	}
}

func TestEncodePatchMessage_Remove(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRemove, NodeID: 4, Data: nil},
	}
	msg := EncodePatchMessage(patches)
	if len(msg.Patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(msg.Patches))
	}
	dp := msg.Patches[0]
	if dp.Op != OpRemove {
		t.Errorf("expected op 'remove', got %q", dp.Op)
	}
	if dp.NodeId != 4 {
		t.Errorf("expected node_id 4, got %d", dp.NodeId)
	}
}

func TestEncodePatchMessage_Reorder(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchReorder, NodeID: 10, Data: vdom.PatchReorderData{
			Inserts: []vdom.ReorderInsert{
				{Index: 0, Key: "a", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 100}, Text: "alpha"}},
				{Index: 2, Key: "c", Node: nil}, // move existing, no new node
			},
			Removes: []vdom.ReorderRemove{
				{Index: 1, Key: "b"},
			},
			Patches: []vdom.Patch{
				{Type: vdom.PatchText, NodeID: 100, Data: vdom.PatchTextData{Text: "updated"}},
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpReorder {
		t.Errorf("expected op 'reorder', got %q", dp.Op)
	}

	// Verify reorder JSON
	var reorder wireReorder
	if err := json.Unmarshal(dp.Reorder, &reorder); err != nil {
		t.Fatal(err)
	}
	if len(reorder.Inserts) != 2 {
		t.Fatalf("expected 2 inserts, got %d", len(reorder.Inserts))
	}
	if reorder.Inserts[0].Key != "a" {
		t.Errorf("expected insert key 'a', got %q", reorder.Inserts[0].Key)
	}
	if reorder.Inserts[0].Index != 0 {
		t.Errorf("expected insert index 0, got %d", reorder.Inserts[0].Index)
	}
	if reorder.Inserts[0].Tree == nil {
		t.Error("expected tree for insert with node")
	}
	if reorder.Inserts[0].Tree.Text != "alpha" {
		t.Errorf("expected tree text 'alpha', got %q", reorder.Inserts[0].Tree.Text)
	}
	if reorder.Inserts[1].Tree != nil {
		t.Error("expected nil tree for insert without node")
	}
	if len(reorder.Removes) != 1 {
		t.Fatalf("expected 1 remove, got %d", len(reorder.Removes))
	}
	if reorder.Removes[0].Key != "b" {
		t.Errorf("expected remove key 'b', got %q", reorder.Removes[0].Key)
	}

	// Verify sub-patches
	if len(dp.SubPatches) != 1 {
		t.Fatalf("expected 1 sub-patch, got %d", len(dp.SubPatches))
	}
	if dp.SubPatches[0].Op != OpText {
		t.Errorf("expected sub-patch op 'text', got %q", dp.SubPatches[0].Op)
	}
}

func TestEncodePatchMessage_UnknownType(t *testing.T) {
	// Unknown patch type should be skipped (returns nil from encodePatch)
	patches := []vdom.Patch{
		{Type: 999, NodeID: 1, Data: nil},
	}
	msg := EncodePatchMessage(patches)
	if len(msg.Patches) != 0 {
		t.Errorf("expected 0 patches for unknown type, got %d", len(msg.Patches))
	}
}

func TestEncodePatchMessage_Empty(t *testing.T) {
	msg := EncodePatchMessage(nil)
	if msg.Kind != "patch" {
		t.Errorf("expected type 'patch', got %q", msg.Kind)
	}
	if len(msg.Patches) != 0 {
		t.Errorf("expected 0 patches, got %d", len(msg.Patches))
	}
}

func TestEncodePatchMessage_Multiple(t *testing.T) {
	// Multiple patches of different types in one message
	patches := []vdom.Patch{
		{Type: vdom.PatchText, NodeID: 1, Data: vdom.PatchTextData{Text: "a"}},
		{Type: vdom.PatchRemove, NodeID: 2, Data: nil},
		{Type: vdom.PatchRemoveLast, NodeID: 3, Data: vdom.PatchRemoveLastData{Count: 1}},
	}
	msg := EncodePatchMessage(patches)
	if len(msg.Patches) != 3 {
		t.Fatalf("expected 3 patches, got %d", len(msg.Patches))
	}
	if msg.Patches[0].Op != OpText {
		t.Errorf("expected first op 'text', got %q", msg.Patches[0].Op)
	}
	if msg.Patches[1].Op != OpRemove {
		t.Errorf("expected second op 'remove', got %q", msg.Patches[1].Op)
	}
	if msg.Patches[2].Op != OpRemoveLast {
		t.Errorf("expected third op 'remove-last', got %q", msg.Patches[2].Op)
	}
}

func TestEncodePatchMessage_Serializable(t *testing.T) {
	// Verify the full message survives protobuf round-trip
	patches := []vdom.Patch{
		{Type: vdom.PatchText, NodeID: 1, Data: vdom.PatchTextData{Text: "hello"}},
		{Type: vdom.PatchFacts, NodeID: 2, Data: vdom.PatchFactsData{
			Diff: vdom.FactsDiff{Styles: map[string]string{"color": "red"}},
		}},
		{Type: vdom.PatchRemoveLast, NodeID: 3, Data: vdom.PatchRemoveLastData{Count: 1}},
	}
	msg := EncodePatchMessage(patches)

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	msg2 := &gproto.ServerMessage{}
	if err := proto.Unmarshal(data, msg2); err != nil {
		t.Fatal(err)
	}
	if len(msg2.Patches) != 3 {
		t.Fatalf("expected 3 patches after round-trip, got %d", len(msg2.Patches))
	}
	if msg2.Patches[0].Text != "hello" {
		t.Errorf("expected text 'hello' after round-trip, got %q", msg2.Patches[0].Text)
	}
}

func TestEncodePatchMessage_ReorderSerializable(t *testing.T) {
	// Verify reorder patch survives protobuf round-trip
	patches := []vdom.Patch{
		{Type: vdom.PatchReorder, NodeID: 1, Data: vdom.PatchReorderData{
			Inserts: []vdom.ReorderInsert{
				{Index: 0, Key: "x", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 50}, Text: "new"}},
			},
			Removes: []vdom.ReorderRemove{
				{Index: 3, Key: "y"},
			},
			Patches: []vdom.Patch{
				{Type: vdom.PatchText, NodeID: 50, Data: vdom.PatchTextData{Text: "changed"}},
			},
		}},
	}
	msg := EncodePatchMessage(patches)

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	msg2 := &gproto.ServerMessage{}
	if err := proto.Unmarshal(data, msg2); err != nil {
		t.Fatal(err)
	}
	if len(msg2.Patches) != 1 {
		t.Fatalf("expected 1 patch after round-trip, got %d", len(msg2.Patches))
	}
	dp := msg2.Patches[0]
	if dp.Op != OpReorder {
		t.Errorf("expected op 'reorder' after round-trip, got %q", dp.Op)
	}
	if len(dp.Reorder) == 0 {
		t.Error("expected reorder data after round-trip")
	}
	if len(dp.SubPatches) != 1 {
		t.Fatalf("expected 1 sub-patch after round-trip, got %d", len(dp.SubPatches))
	}
}

// ---------------------------------------------------------------------------
// encodeFactsDiff tests
// ---------------------------------------------------------------------------

func TestEncodeFactsDiff_Empty(t *testing.T) {
	d := &vdom.FactsDiff{}
	w := encodeFactsDiff(d)
	if w.Props != nil {
		t.Errorf("expected nil props, got %v", w.Props)
	}
	if w.Attrs != nil {
		t.Errorf("expected nil attrs, got %v", w.Attrs)
	}
	if w.AttrsNS != nil {
		t.Errorf("expected nil attrsNS, got %v", w.AttrsNS)
	}
	if w.Styles != nil {
		t.Errorf("expected nil styles, got %v", w.Styles)
	}
	if w.Events != nil {
		t.Errorf("expected nil events, got %v", w.Events)
	}
}

func TestEncodeFactsDiff_Props(t *testing.T) {
	d := &vdom.FactsDiff{
		Props: map[string]any{"className": "active", "disabled": true},
	}
	w := encodeFactsDiff(d)
	if w.Props["className"] != "active" {
		t.Errorf("expected className='active', got %v", w.Props["className"])
	}
	if w.Props["disabled"] != true {
		t.Errorf("expected disabled=true, got %v", w.Props["disabled"])
	}
}

func TestEncodeFactsDiff_Attrs(t *testing.T) {
	d := &vdom.FactsDiff{
		Attrs: map[string]string{"data-id": "42", "role": "button"},
	}
	w := encodeFactsDiff(d)
	if w.Attrs["data-id"] != "42" {
		t.Errorf("expected data-id='42', got %q", w.Attrs["data-id"])
	}
	if w.Attrs["role"] != "button" {
		t.Errorf("expected role='button', got %q", w.Attrs["role"])
	}
}

func TestEncodeFactsDiff_AttrsNS(t *testing.T) {
	d := &vdom.FactsDiff{
		AttrsNS: map[string]vdom.NSAttr{
			"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#icon"},
		},
	}
	w := encodeFactsDiff(d)
	if len(w.AttrsNS) != 1 {
		t.Fatalf("expected 1 namespaced attr, got %d", len(w.AttrsNS))
	}
	attr := w.AttrsNS["xlink:href"]
	if attr.NS != "http://www.w3.org/1999/xlink" {
		t.Errorf("expected xlink namespace, got %q", attr.NS)
	}
	if attr.Val != "#icon" {
		t.Errorf("expected value '#icon', got %q", attr.Val)
	}
}

func TestEncodeFactsDiff_Styles(t *testing.T) {
	d := &vdom.FactsDiff{
		Styles: map[string]string{"color": "red", "font-size": "14px"},
	}
	w := encodeFactsDiff(d)
	if w.Styles["color"] != "red" {
		t.Errorf("expected color='red', got %q", w.Styles["color"])
	}
	if w.Styles["font-size"] != "14px" {
		t.Errorf("expected font-size='14px', got %q", w.Styles["font-size"])
	}
}

func TestEncodeFactsDiff_Events_Simple(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"click": {
				Handler: "HandleClick",
				Options: vdom.EventOptions{StopPropagation: true},
			},
		},
	}
	w := encodeFactsDiff(d)
	if len(w.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(w.Events))
	}
	ev := w.Events["click"]
	if ev == nil {
		t.Fatal("expected click event")
	}
	if ev.On != "click" {
		t.Errorf("expected on='click', got %q", ev.On)
	}
	if ev.Method != "HandleClick" {
		t.Errorf("expected method='HandleClick', got %q", ev.Method)
	}
	if !ev.SP {
		t.Error("expected stopPropagation=true")
	}
	if ev.PD {
		t.Error("expected preventDefault=false")
	}
}

func TestEncodeFactsDiff_Events_WithKeyFilter(t *testing.T) {
	// Event key format: "keydown:Enter" → eventType="keydown", keyFilter="Enter"
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"keydown:Enter": {
				Handler: "Submit",
				Options: vdom.EventOptions{PreventDefault: true},
			},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["keydown:Enter"]
	if ev == nil {
		t.Fatal("expected keydown:Enter event")
	}
	if ev.On != "keydown" {
		t.Errorf("expected on='keydown', got %q", ev.On)
	}
	if ev.Key != "Enter" {
		t.Errorf("expected key='Enter', got %q", ev.Key)
	}
	if !ev.PD {
		t.Error("expected preventDefault=true")
	}
}

func TestEncodeFactsDiff_Events_WithKeyFromOptions(t *testing.T) {
	// When Options.Key is set and there's a colon key in the map key,
	// Options.Key takes precedence
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"keydown:Escape": {
				Handler: "Cancel",
				Options: vdom.EventOptions{Key: "Escape"},
			},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["keydown:Escape"]
	if ev == nil {
		t.Fatal("expected keydown:Escape event")
	}
	if ev.Key != "Escape" {
		t.Errorf("expected key='Escape', got %q", ev.Key)
	}
}

func TestEncodeFactsDiff_Events_WithOptionsKeyNoColon(t *testing.T) {
	// Options.Key is set but map key has no colon
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"keydown": {
				Handler: "HandleKey",
				Options: vdom.EventOptions{Key: "Tab"},
			},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["keydown"]
	if ev == nil {
		t.Fatal("expected keydown event")
	}
	if ev.On != "keydown" {
		t.Errorf("expected on='keydown', got %q", ev.On)
	}
	if ev.Key != "Tab" {
		t.Errorf("expected key='Tab', got %q", ev.Key)
	}
}

func TestEncodeFactsDiff_Events_NilRemoval(t *testing.T) {
	// nil event handler means removal
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"click": nil,
		},
	}
	w := encodeFactsDiff(d)
	if len(w.Events) != 1 {
		t.Fatalf("expected 1 event entry, got %d", len(w.Events))
	}
	if w.Events["click"] != nil {
		t.Error("expected nil event for removal")
	}
}

func TestEncodeFactsDiff_Events_WithArgs(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"click": {
				Handler: "SetItem",
				Args:    []any{"hello", 42, true},
			},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["click"]
	if ev == nil {
		t.Fatal("expected click event")
	}
	if len(ev.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(ev.Args))
	}
	// Verify JSON encoding of args
	if string(ev.Args[0]) != `"hello"` {
		t.Errorf("expected arg[0]='\"hello\"', got %q", string(ev.Args[0]))
	}
	if string(ev.Args[1]) != `42` {
		t.Errorf("expected arg[1]='42', got %q", string(ev.Args[1]))
	}
	if string(ev.Args[2]) != `true` {
		t.Errorf("expected arg[2]='true', got %q", string(ev.Args[2]))
	}
}

func TestEncodeFactsDiff_AllFields(t *testing.T) {
	d := &vdom.FactsDiff{
		Props:   map[string]any{"value": "x"},
		Attrs:   map[string]string{"id": "main"},
		AttrsNS: map[string]vdom.NSAttr{"xlink:href": {Namespace: "ns", Value: "v"}},
		Styles:  map[string]string{"color": "blue"},
		Events: map[string]*vdom.EventHandler{
			"input": {Handler: "OnInput"},
		},
	}
	w := encodeFactsDiff(d)

	// Verify all fields populated
	if w.Props["value"] != "x" {
		t.Errorf("expected props value='x', got %v", w.Props["value"])
	}
	if w.Attrs["id"] != "main" {
		t.Errorf("expected attrs id='main', got %q", w.Attrs["id"])
	}
	if w.AttrsNS["xlink:href"].NS != "ns" {
		t.Errorf("expected attrsNS ns='ns', got %q", w.AttrsNS["xlink:href"].NS)
	}
	if w.Styles["color"] != "blue" {
		t.Errorf("expected styles color='blue', got %q", w.Styles["color"])
	}
	if w.Events["input"].Method != "OnInput" {
		t.Errorf("expected event method='OnInput', got %q", w.Events["input"].Method)
	}

	// Verify JSON round-trip
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var w2 WireFactsDiff
	if err := json.Unmarshal(data, &w2); err != nil {
		t.Fatal(err)
	}
	if w2.Props["value"] != "x" {
		t.Errorf("after round-trip: expected props value='x', got %v", w2.Props["value"])
	}
}

// ---------------------------------------------------------------------------
// encodeReorderData tests
// ---------------------------------------------------------------------------

func TestEncodeReorderData_Empty(t *testing.T) {
	d := &vdom.PatchReorderData{}
	w := encodeReorderData(d)
	if len(w.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %d", len(w.Inserts))
	}
	if len(w.Removes) != 0 {
		t.Errorf("expected 0 removes, got %d", len(w.Removes))
	}
}

func TestEncodeReorderData_InsertsWithNode(t *testing.T) {
	d := &vdom.PatchReorderData{
		Inserts: []vdom.ReorderInsert{
			{Index: 0, Key: "a", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 1}, Text: "new"}},
		},
	}
	w := encodeReorderData(d)
	if len(w.Inserts) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(w.Inserts))
	}
	ins := w.Inserts[0]
	if ins.Index != 0 {
		t.Errorf("expected index 0, got %d", ins.Index)
	}
	if ins.Key != "a" {
		t.Errorf("expected key 'a', got %q", ins.Key)
	}
	if ins.Tree == nil {
		t.Fatal("expected tree for insert")
	}
	if ins.Tree.Text != "new" {
		t.Errorf("expected tree text 'new', got %q", ins.Tree.Text)
	}
}

func TestEncodeReorderData_InsertsWithoutNode(t *testing.T) {
	d := &vdom.PatchReorderData{
		Inserts: []vdom.ReorderInsert{
			{Index: 2, Key: "b", Node: nil},
		},
	}
	w := encodeReorderData(d)
	if w.Inserts[0].Tree != nil {
		t.Error("expected nil tree for insert without node")
	}
}

func TestEncodeReorderData_Removes(t *testing.T) {
	d := &vdom.PatchReorderData{
		Removes: []vdom.ReorderRemove{
			{Index: 1, Key: "x"},
			{Index: 3, Key: "y"},
		},
	}
	w := encodeReorderData(d)
	if len(w.Removes) != 2 {
		t.Fatalf("expected 2 removes, got %d", len(w.Removes))
	}
	if w.Removes[0].Key != "x" || w.Removes[0].Index != 1 {
		t.Errorf("expected remove {1, x}, got {%d, %q}", w.Removes[0].Index, w.Removes[0].Key)
	}
	if w.Removes[1].Key != "y" || w.Removes[1].Index != 3 {
		t.Errorf("expected remove {3, y}, got {%d, %q}", w.Removes[1].Index, w.Removes[1].Key)
	}
}

func TestEncodeReorderData_JSON(t *testing.T) {
	d := &vdom.PatchReorderData{
		Inserts: []vdom.ReorderInsert{
			{Index: 0, Key: "a", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 1}, Text: "item"}},
		},
		Removes: []vdom.ReorderRemove{
			{Index: 2, Key: "c"},
		},
	}
	w := encodeReorderData(d)
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	// Verify JSON structure
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["ins"]; !ok {
		t.Error("expected 'ins' key in JSON")
	}
	if _, ok := parsed["rem"]; !ok {
		t.Error("expected 'rem' key in JSON")
	}
}

// ---------------------------------------------------------------------------
// Tree encoder tests
// ---------------------------------------------------------------------------

func TestEncodeTree_Nil(t *testing.T) {
	wn := EncodeTree(nil)
	if wn != nil {
		t.Errorf("expected nil for nil input, got %+v", wn)
	}
}

func TestEncodeTree_TextNode(t *testing.T) {
	n := &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 1}, Text: "hello"}
	wn := EncodeTree(n)
	if wn.ID != 1 {
		t.Errorf("expected ID 1, got %d", wn.ID)
	}
	if wn.Type != "text" {
		t.Errorf("expected type 'text', got %q", wn.Type)
	}
	if wn.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", wn.Text)
	}
}

func TestEncodeTree_TextNode_Empty(t *testing.T) {
	n := &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 2}, Text: ""}
	wn := EncodeTree(n)
	if wn.Type != "text" {
		t.Errorf("expected type 'text', got %q", wn.Type)
	}
	if wn.Text != "" {
		t.Errorf("expected empty text, got %q", wn.Text)
	}
}

func TestEncodeTree_ElementNode(t *testing.T) {
	n := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 2},
		Tag:      "div",
		Facts: vdom.Facts{
			Props:  map[string]any{"className": "app"},
			Styles: map[string]string{"color": "red"},
		},
		Children: []vdom.Node{
			&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 3}, Text: "child"},
		},
	}
	wn := EncodeTree(n)
	if wn.Tag != "div" {
		t.Errorf("expected tag 'div', got %q", wn.Tag)
	}
	if wn.Props["className"] != "app" {
		t.Errorf("expected className='app', got %v", wn.Props["className"])
	}
	if wn.Styles["color"] != "red" {
		t.Errorf("expected color='red', got %q", wn.Styles["color"])
	}
	if len(wn.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(wn.Children))
	}
	if wn.Children[0].Text != "child" {
		t.Errorf("expected child text 'child', got %q", wn.Children[0].Text)
	}
}

func TestEncodeTree_ElementNode_WithNamespace(t *testing.T) {
	n := &vdom.ElementNode{
		NodeBase:  vdom.NodeBase{ID: 5},
		Tag:       "svg",
		Namespace: "http://www.w3.org/2000/svg",
	}
	wn := EncodeTree(n)
	if wn.NS != "http://www.w3.org/2000/svg" {
		t.Errorf("expected SVG namespace, got %q", wn.NS)
	}
	if wn.Type != "el" {
		t.Errorf("expected type 'el', got %q", wn.Type)
	}
}

func TestEncodeTree_ElementNode_NoChildren(t *testing.T) {
	n := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 6},
		Tag:      "br",
	}
	wn := EncodeTree(n)
	if len(wn.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(wn.Children))
	}
}

func TestEncodeTree_ElementNode_DeepNesting(t *testing.T) {
	n := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1},
		Tag:      "div",
		Children: []vdom.Node{
			&vdom.ElementNode{
				NodeBase: vdom.NodeBase{ID: 2},
				Tag:      "ul",
				Children: []vdom.Node{
					&vdom.ElementNode{
						NodeBase: vdom.NodeBase{ID: 3},
						Tag:      "li",
						Children: []vdom.Node{
							&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 4}, Text: "deep"},
						},
					},
				},
			},
		},
	}
	wn := EncodeTree(n)
	if wn.Children[0].Children[0].Children[0].Text != "deep" {
		t.Error("expected deeply nested text 'deep'")
	}
}

func TestEncodeTree_KeyedElementNode(t *testing.T) {
	n := &vdom.KeyedElementNode{
		NodeBase: vdom.NodeBase{ID: 4},
		Tag:      "ul",
		Children: []vdom.KeyedChild{
			{Key: "a", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 5}, Text: "alpha"}},
			{Key: "b", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 6}, Text: "beta"}},
		},
	}
	wn := EncodeTree(n)
	if wn.Type != "keyed" {
		t.Errorf("expected type 'keyed', got %q", wn.Type)
	}
	if len(wn.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(wn.Keys))
	}
	if wn.Keys[0] != "a" || wn.Keys[1] != "b" {
		t.Errorf("expected keys [a,b], got %v", wn.Keys)
	}
}

func TestEncodeTree_KeyedElementNode_WithFacts(t *testing.T) {
	n := &vdom.KeyedElementNode{
		NodeBase: vdom.NodeBase{ID: 10},
		Tag:      "ol",
		Facts: vdom.Facts{
			Attrs: map[string]string{"class": "list"},
		},
		Children: []vdom.KeyedChild{
			{Key: "1", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 11}, Text: "one"}},
		},
	}
	wn := EncodeTree(n)
	if wn.Attrs["class"] != "list" {
		t.Errorf("expected class='list', got %q", wn.Attrs["class"])
	}
}

func TestEncodeTree_KeyedElementNode_WithNamespace(t *testing.T) {
	n := &vdom.KeyedElementNode{
		NodeBase:  vdom.NodeBase{ID: 12},
		Tag:       "g",
		Namespace: "http://www.w3.org/2000/svg",
		Children:  []vdom.KeyedChild{},
	}
	wn := EncodeTree(n)
	if wn.NS != "http://www.w3.org/2000/svg" {
		t.Errorf("expected SVG namespace, got %q", wn.NS)
	}
}

func TestEncodeTree_PluginNode(t *testing.T) {
	n := &vdom.PluginNode{
		NodeBase: vdom.NodeBase{ID: 9},
		Tag:      "canvas",
		Name:     "chart",
		Data:     map[string]int{"width": 800},
	}
	wn := EncodeTree(n)
	if wn.Plugin != "chart" {
		t.Errorf("expected plugin 'chart', got %q", wn.Plugin)
	}
	if wn.Tag != "canvas" {
		t.Errorf("expected tag 'canvas', got %q", wn.Tag)
	}
	if wn.Type != "el" {
		t.Errorf("expected type 'el', got %q", wn.Type)
	}
}

func TestEncodeTree_PluginNode_NilData(t *testing.T) {
	n := &vdom.PluginNode{
		NodeBase: vdom.NodeBase{ID: 15},
		Tag:      "div",
		Name:     "empty-plugin",
		Data:     nil,
	}
	wn := EncodeTree(n)
	if wn.PlugData != nil {
		t.Errorf("expected nil plugin data, got %v", wn.PlugData)
	}
	if wn.Plugin != "empty-plugin" {
		t.Errorf("expected plugin 'empty-plugin', got %q", wn.Plugin)
	}
}

func TestEncodeTree_PluginNode_WithFacts(t *testing.T) {
	n := &vdom.PluginNode{
		NodeBase: vdom.NodeBase{ID: 16},
		Tag:      "canvas",
		Name:     "drawing",
		Facts: vdom.Facts{
			Styles: map[string]string{"width": "100%"},
			Attrs:  map[string]string{"id": "canvas1"},
		},
		Data: "some-data",
	}
	wn := EncodeTree(n)
	if wn.Styles["width"] != "100%" {
		t.Errorf("expected width='100%%', got %q", wn.Styles["width"])
	}
	if wn.Attrs["id"] != "canvas1" {
		t.Errorf("expected id='canvas1', got %q", wn.Attrs["id"])
	}
}

func TestEncodeTree_LazyNode_WithCached(t *testing.T) {
	n := &vdom.LazyNode{
		NodeBase: vdom.NodeBase{ID: 30},
		Cached:   &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 31}, Text: "cached"},
	}
	wn := EncodeTree(n)
	if wn == nil {
		t.Fatal("expected non-nil for lazy node with cached subtree")
	}
	if wn.Text != "cached" {
		t.Errorf("expected text 'cached', got %q", wn.Text)
	}
	if wn.ID != 31 {
		t.Errorf("expected ID 31, got %d", wn.ID)
	}
}

func TestEncodeTree_LazyNode_NilCached(t *testing.T) {
	n := &vdom.LazyNode{
		NodeBase: vdom.NodeBase{ID: 32},
		Cached:   nil,
	}
	wn := EncodeTree(n)
	if wn != nil {
		t.Errorf("expected nil for lazy node without cached, got %+v", wn)
	}
}

// ---------------------------------------------------------------------------
// encodeFacts tests (tree_encode.go)
// ---------------------------------------------------------------------------

func TestEncodeFacts_Empty(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{}
	encodeFacts(f, wn)
	if wn.Props != nil {
		t.Error("expected nil props")
	}
	if wn.Attrs != nil {
		t.Error("expected nil attrs")
	}
	if wn.AttrsNS != nil {
		t.Error("expected nil attrsNS")
	}
	if wn.Styles != nil {
		t.Error("expected nil styles")
	}
	if wn.Events != nil {
		t.Error("expected nil events")
	}
}

func TestEncodeFacts_Props(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Props: map[string]any{"className": "active", "tabIndex": 0},
	}
	encodeFacts(f, wn)
	if wn.Props["className"] != "active" {
		t.Errorf("expected className='active', got %v", wn.Props["className"])
	}
}

func TestEncodeFacts_Attrs(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Attrs: map[string]string{"data-id": "42", "aria-label": "Close"},
	}
	encodeFacts(f, wn)
	if wn.Attrs["data-id"] != "42" {
		t.Errorf("expected data-id='42', got %q", wn.Attrs["data-id"])
	}
	if wn.Attrs["aria-label"] != "Close" {
		t.Errorf("expected aria-label='Close', got %q", wn.Attrs["aria-label"])
	}
}

func TestEncodeFacts_AttrsNS(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		AttrsNS: map[string]vdom.NSAttr{
			"xlink:href": {Namespace: "http://www.w3.org/1999/xlink", Value: "#icon"},
		},
	}
	encodeFacts(f, wn)
	if len(wn.AttrsNS) != 1 {
		t.Fatalf("expected 1 namespaced attr, got %d", len(wn.AttrsNS))
	}
	attr := wn.AttrsNS["xlink:href"]
	if attr.NS != "http://www.w3.org/1999/xlink" {
		t.Errorf("expected xlink namespace, got %q", attr.NS)
	}
	if attr.Val != "#icon" {
		t.Errorf("expected '#icon', got %q", attr.Val)
	}
}

func TestEncodeFacts_Styles(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Styles: map[string]string{"display": "flex", "gap": "8px"},
	}
	encodeFacts(f, wn)
	if wn.Styles["display"] != "flex" {
		t.Errorf("expected display='flex', got %q", wn.Styles["display"])
	}
}

func TestEncodeFacts_Events_Simple(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Events: map[string]vdom.EventHandler{
			"click": {
				Handler: "OnClick",
				Options: vdom.EventOptions{StopPropagation: true, PreventDefault: true},
			},
		},
	}
	encodeFacts(f, wn)
	if len(wn.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(wn.Events))
	}
	ev := wn.Events[0]
	if ev.On != "click" {
		t.Errorf("expected on='click', got %q", ev.On)
	}
	if ev.Method != "OnClick" {
		t.Errorf("expected method='OnClick', got %q", ev.Method)
	}
	if !ev.SP {
		t.Error("expected stopPropagation=true")
	}
	if !ev.PD {
		t.Error("expected preventDefault=true")
	}
}

func TestEncodeFacts_Events_WithKeyFilter(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Events: map[string]vdom.EventHandler{
			"keydown:Enter": {Handler: "Submit"},
		},
	}
	encodeFacts(f, wn)
	if len(wn.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(wn.Events))
	}
	ev := wn.Events[0]
	if ev.On != "keydown" {
		t.Errorf("expected on='keydown', got %q", ev.On)
	}
	if ev.Key != "Enter" {
		t.Errorf("expected key='Enter', got %q", ev.Key)
	}
}

func TestEncodeFacts_Events_WithArgs(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Events: map[string]vdom.EventHandler{
			"click": {
				Handler: "SetPage",
				Args:    []any{3, "next"},
			},
		},
	}
	encodeFacts(f, wn)
	ev := wn.Events[0]
	if len(ev.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(ev.Args))
	}
	if string(ev.Args[0]) != "3" {
		t.Errorf("expected arg[0]='3', got %q", string(ev.Args[0]))
	}
	if string(ev.Args[1]) != `"next"` {
		t.Errorf("expected arg[1]='\"next\"', got %q", string(ev.Args[1]))
	}
}

func TestEncodeFacts_AllFields(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Props:   map[string]any{"value": "x"},
		Attrs:   map[string]string{"id": "main"},
		AttrsNS: map[string]vdom.NSAttr{"xlink:href": {Namespace: "ns", Value: "v"}},
		Styles:  map[string]string{"color": "blue"},
		Events: map[string]vdom.EventHandler{
			"input": {Handler: "OnInput"},
		},
	}
	encodeFacts(f, wn)
	if wn.Props["value"] != "x" {
		t.Error("expected props populated")
	}
	if wn.Attrs["id"] != "main" {
		t.Error("expected attrs populated")
	}
	if wn.AttrsNS["xlink:href"].NS != "ns" {
		t.Error("expected attrsNS populated")
	}
	if wn.Styles["color"] != "blue" {
		t.Error("expected styles populated")
	}
	if len(wn.Events) != 1 {
		t.Error("expected events populated")
	}
}

// ---------------------------------------------------------------------------
// EncodeTreeJSON tests
// ---------------------------------------------------------------------------

func TestEncodeTreeJSON_Nil(t *testing.T) {
	data, err := EncodeTreeJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "null" {
		t.Errorf("expected 'null', got %q", string(data))
	}
}

func TestEncodeTreeJSON_RoundTrip(t *testing.T) {
	n := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1},
		Tag:      "div",
		Children: []vdom.Node{
			&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 2}, Text: "hello"},
		},
	}
	data, err := EncodeTreeJSON(n)
	if err != nil {
		t.Fatal(err)
	}
	var wn WireNode
	if err := json.Unmarshal(data, &wn); err != nil {
		t.Fatal(err)
	}
	if wn.Tag != "div" {
		t.Errorf("expected tag 'div', got %q", wn.Tag)
	}
}

// ---------------------------------------------------------------------------
// EncodeInitTreeMessage tests
// ---------------------------------------------------------------------------

func TestEncodeInitTreeMessage(t *testing.T) {
	root := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1},
		Tag:      "body",
		Children: []vdom.Node{
			&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 2}, Text: "hi"},
		},
	}
	msg, err := EncodeInitTreeMessage(root)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Kind != "init" {
		t.Errorf("expected type 'init', got %q", msg.Kind)
	}
	if len(msg.Tree) == 0 {
		t.Error("expected non-empty tree JSON")
	}

	// Verify it's serializable via protobuf
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	msg2 := &gproto.ServerMessage{}
	if err := proto.Unmarshal(data, msg2); err != nil {
		t.Fatal(err)
	}
	if msg2.Kind != "init" {
		t.Errorf("expected 'init' after round-trip, got %q", msg2.Kind)
	}
}

func TestEncodeInitTreeMessage_NilRoot(t *testing.T) {
	msg, err := EncodeInitTreeMessage(nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Kind != "init" {
		t.Errorf("expected type 'init', got %q", msg.Kind)
	}
	// Tree should be "null" JSON
	if string(msg.Tree) != "null" {
		t.Errorf("expected 'null' tree, got %q", string(msg.Tree))
	}
}

func TestEncodeInitTreeMessage_ComplexTree(t *testing.T) {
	root := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1},
		Tag:      "body",
		Facts: vdom.Facts{
			Attrs:  map[string]string{"class": "app"},
			Styles: map[string]string{"margin": "0"},
			Events: map[string]vdom.EventHandler{
				"click": {Handler: "OnClick"},
			},
		},
		Children: []vdom.Node{
			&vdom.ElementNode{
				NodeBase: vdom.NodeBase{ID: 2},
				Tag:      "h1",
				Children: []vdom.Node{
					&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 3}, Text: "Title"},
				},
			},
			&vdom.KeyedElementNode{
				NodeBase: vdom.NodeBase{ID: 4},
				Tag:      "ul",
				Children: []vdom.KeyedChild{
					{Key: "1", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 5}, Text: "item1"}},
				},
			},
		},
	}
	msg, err := EncodeInitTreeMessage(root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify JSON content
	var wn WireNode
	if err := json.Unmarshal(msg.Tree, &wn); err != nil {
		t.Fatal(err)
	}
	if wn.Tag != "body" {
		t.Errorf("expected tag 'body', got %q", wn.Tag)
	}
	if len(wn.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(wn.Children))
	}
	if wn.Children[1].Type != "keyed" {
		t.Errorf("expected keyed child, got %q", wn.Children[1].Type)
	}

	// Should survive protobuf round-trip
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	msg2 := &gproto.ServerMessage{}
	if err := proto.Unmarshal(data, msg2); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// JSON wire format correctness
// ---------------------------------------------------------------------------

func TestWireNode_JSONFieldNames(t *testing.T) {
	// Verify compact JSON field names
	wn := &WireNode{
		ID:   1,
		Type: "el",
		Tag:  "div",
		Text: "x",
		NS:   "ns",
	}
	data, err := json.Marshal(wn)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// Verify expected JSON keys
	expectedKeys := map[string]bool{"id": true, "t": true, "tag": true, "x": true, "ns": true}
	for k := range raw {
		if !expectedKeys[k] {
			t.Errorf("unexpected JSON key %q", k)
		}
	}
	if _, ok := raw["t"]; !ok {
		t.Error("expected 't' key for Type field")
	}
	if _, ok := raw["x"]; !ok {
		t.Error("expected 'x' key for Text field")
	}
}

func TestWireFactsDiff_JSONFieldNames(t *testing.T) {
	w := &WireFactsDiff{
		Props:  map[string]any{"a": 1},
		Attrs:  map[string]string{"b": "2"},
		Styles: map[string]string{"c": "3"},
	}
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["p"]; !ok {
		t.Error("expected 'p' key for Props")
	}
	if _, ok := raw["a"]; !ok {
		t.Error("expected 'a' key for Attrs")
	}
	if _, ok := raw["s"]; !ok {
		t.Error("expected 's' key for Styles")
	}
}

func TestWireFactsDiff_OmitsEmpty(t *testing.T) {
	w := &WireFactsDiff{}
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Errorf("expected empty JSON '{}', got %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// End-to-end: parse → resolve → diff → encode
// ---------------------------------------------------------------------------

func TestEndToEnd_ParseResolveDiffEncode(t *testing.T) {
	htmlStr := `<!DOCTYPE html><html><head></head><body>
		<h1><span g-text="Count">0</span></h1>
		<button g-click="Increment">+</button>
	</body></html>`

	templates, err := vdom.ParseTemplate(htmlStr)
	if err != nil {
		t.Fatal(err)
	}

	type counter struct {
		Count int
	}

	// Render 1
	s1 := &counter{Count: 5}
	ctx1 := &vdom.ResolveContext{State: makeReflectValue(s1), Vars: make(map[string]any)}
	tree1 := vdom.ResolveTree(templates, ctx1)
	root1 := &vdom.ElementNode{Tag: "body", Children: tree1}
	vdom.ComputeDescendants(root1)

	// Render 2
	s2 := &counter{Count: 10}
	ctx2 := &vdom.ResolveContext{State: makeReflectValue(s2), Vars: make(map[string]any)}
	tree2 := vdom.ResolveTree(templates, ctx2)
	root2 := &vdom.ElementNode{Tag: "body", Children: tree2}
	vdom.ComputeDescendants(root2)

	// Diff
	patches := vdom.Diff(root1, root2)
	if len(patches) == 0 {
		t.Fatal("expected patches")
	}

	// Encode
	msg := EncodePatchMessage(patches)
	if msg.Kind != "patch" {
		t.Errorf("expected 'patch', got %q", msg.Kind)
	}

	// Should be serializable
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty protobuf output")
	}

	// Verify content
	hasTextPatch := false
	for _, dp := range msg.Patches {
		if dp.Op == OpText && dp.Text == "10" {
			hasTextPatch = true
		}
	}
	if !hasTextPatch {
		t.Error("expected text patch '5' → '10'")
	}
}

func makeReflectValue(v any) reflect.Value {
	return reflect.ValueOf(v)
}

// ===========================================================================
// Negative / edge-case tests
// ===========================================================================

// --- Element with nil child in children list ---
// EncodeTree appends EncodeTree(child) which returns nil for a nil child.
// The resulting JSON array will contain a null entry. Verify this is what happens.
func TestEncodeTree_ElementNode_NilChild(t *testing.T) {
	n := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1},
		Tag:      "div",
		Children: []vdom.Node{
			&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 2}, Text: "before"},
			nil, // nil child
			&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 3}, Text: "after"},
		},
	}
	wn := EncodeTree(n)
	if len(wn.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(wn.Children))
	}
	// The nil child should produce a nil *WireNode
	if wn.Children[1] != nil {
		t.Errorf("expected nil for nil child, got %+v", wn.Children[1])
	}
	// Verify JSON produces [node, null, node] — the bridge must handle this
	data, err := json.Marshal(wn)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var children []json.RawMessage
	if err := json.Unmarshal(raw["c"], &children); err != nil {
		t.Fatal(err)
	}
	if string(children[1]) != "null" {
		t.Errorf("expected null in JSON for nil child, got %s", children[1])
	}
}

// --- KeyedElementNode with nil node in keyed child ---
func TestEncodeTree_KeyedElementNode_NilChildNode(t *testing.T) {
	n := &vdom.KeyedElementNode{
		NodeBase: vdom.NodeBase{ID: 1},
		Tag:      "ul",
		Children: []vdom.KeyedChild{
			{Key: "a", Node: &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 2}, Text: "ok"}},
			{Key: "b", Node: nil}, // nil node in keyed child
		},
	}
	wn := EncodeTree(n)
	if len(wn.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(wn.Children))
	}
	if wn.Children[1] != nil {
		t.Errorf("expected nil for keyed child with nil node, got %+v", wn.Children[1])
	}
	// Keys should still be populated
	if len(wn.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(wn.Keys))
	}
	if wn.Keys[1] != "b" {
		t.Errorf("expected key 'b', got %q", wn.Keys[1])
	}
}

// --- PatchAppend with empty nodes list ---
// Sends an empty JSON array to the bridge — should it be a no-op instead?
func TestEncodePatchMessage_Append_EmptyNodes(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchAppend, NodeID: 1, Data: vdom.PatchAppendData{
			Nodes: []vdom.Node{},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpAppend {
		t.Errorf("expected op 'append', got %q", dp.Op)
	}
	// Produces "[]" — an empty array, not omitted
	var trees []*WireNode
	if err := json.Unmarshal(dp.TreeContent, &trees); err != nil {
		t.Fatal(err)
	}
	if len(trees) != 0 {
		t.Errorf("expected empty tree array, got %d", len(trees))
	}
}

// --- PatchRedraw with nil node ---
// Sends "null" as tree content — does the bridge handle this?
func TestEncodePatchMessage_Redraw_NilNode(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRedraw, NodeID: 5, Data: vdom.PatchRedrawData{
			Node: nil,
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpRedraw {
		t.Errorf("expected op 'redraw', got %q", dp.Op)
	}
	// EncodeTree(nil) returns nil, json.Marshal(nil) = "null"
	if string(dp.TreeContent) != "null" {
		t.Errorf("expected 'null' tree content for nil node, got %q", string(dp.TreeContent))
	}
}

// --- PatchRemoveLast with count 0 ---
func TestEncodePatchMessage_RemoveLast_Zero(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRemoveLast, NodeID: 1, Data: vdom.PatchRemoveLastData{Count: 0}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Count != 0 {
		t.Errorf("expected count 0, got %d", dp.Count)
	}
}

// --- PatchLazy with empty sub-patches ---
func TestEncodePatchMessage_Lazy_EmptySubPatches(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchLazy, NodeID: 1, Data: vdom.PatchLazyData{
			Patches: []vdom.Patch{},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpLazy {
		t.Errorf("expected op 'lazy', got %q", dp.Op)
	}
	if len(dp.SubPatches) != 0 {
		t.Errorf("expected 0 sub-patches, got %d", len(dp.SubPatches))
	}
}

// --- Lazy sub-patch contains unknown type — should be filtered out ---
func TestEncodePatchMessage_Lazy_UnknownSubPatch(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchLazy, NodeID: 1, Data: vdom.PatchLazyData{
			Patches: []vdom.Patch{
				{Type: 999, NodeID: 2, Data: nil}, // unknown
				{Type: vdom.PatchText, NodeID: 3, Data: vdom.PatchTextData{Text: "ok"}},
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	// Unknown sub-patch returns nil from encodePatch, should be skipped
	if len(dp.SubPatches) != 1 {
		t.Fatalf("expected 1 sub-patch (unknown filtered), got %d", len(dp.SubPatches))
	}
	if dp.SubPatches[0].Op != OpText {
		t.Errorf("expected remaining sub-patch op 'text', got %q", dp.SubPatches[0].Op)
	}
}

// --- Reorder sub-patch contains unknown type — should be filtered ---
func TestEncodePatchMessage_Reorder_UnknownSubPatch(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchReorder, NodeID: 1, Data: vdom.PatchReorderData{
			Patches: []vdom.Patch{
				{Type: 999, NodeID: 2, Data: nil}, // unknown
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if len(dp.SubPatches) != 0 {
		t.Errorf("expected 0 sub-patches (unknown filtered), got %d", len(dp.SubPatches))
	}
}

// --- encodeFacts ignores Options.Key, only uses colon parsing ---
// This is different from encodeFactsDiff which gives Options.Key precedence.
// Test documents this behavioral difference.
func TestEncodeFacts_Events_OptionsKeyIgnored(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Events: map[string]vdom.EventHandler{
			"keydown": {
				Handler: "HandleKey",
				Options: vdom.EventOptions{Key: "Tab"},
			},
		},
	}
	encodeFacts(f, wn)
	ev := wn.Events[0]
	// encodeFacts does NOT check Options.Key — it only uses colon parsing.
	// Since there's no colon in "keydown", keyFilter stays "".
	// This means Options.Key="Tab" is lost.
	// Compare with encodeFactsDiff which would produce Key="Tab".
	if ev.Key != "Tab" {
		t.Errorf("encodeFacts should respect Options.Key='Tab', got %q "+
			"(known inconsistency: encodeFacts ignores Options.Key, "+
			"encodeFactsDiff respects it)", ev.Key)
	}
}

// --- encodeFactsDiff: Options.Key takes precedence over colon-parsed key ---
func TestEncodeFactsDiff_Events_OptionsKeyPrecedence(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"keydown:Enter": {
				Handler: "Submit",
				Options: vdom.EventOptions{Key: "Space"},
			},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["keydown:Enter"]
	// Options.Key="Space" should take precedence over colon-parsed "Enter"
	if ev.Key != "Space" {
		t.Errorf("expected Options.Key 'Space' to take precedence, got %q", ev.Key)
	}
}

// --- Event key: multiple colons ---
// "keydown:Enter:extra" — strings.Index finds first colon
func TestEncodeFactsDiff_Events_MultipleColons(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"keydown:Enter:extra": {Handler: "Handle"},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["keydown:Enter:extra"]
	if ev.On != "keydown" {
		t.Errorf("expected on='keydown', got %q", ev.On)
	}
	// After first colon: "Enter:extra"
	if ev.Key != "Enter:extra" {
		t.Errorf("expected key='Enter:extra', got %q", ev.Key)
	}
}

// --- Event key: colon only ---
func TestEncodeFactsDiff_Events_ColonOnly(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			":": {Handler: "Handle"},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events[":"]
	// eventType = "" (before colon), keyFilter = "" (after colon)
	if ev.On != "" {
		t.Errorf("expected empty event type for ':', got %q", ev.On)
	}
	if ev.Key != "" {
		t.Errorf("expected empty key for ':', got %q", ev.Key)
	}
}

// --- Event key: leading colon ---
func TestEncodeFactsDiff_Events_LeadingColon(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			":Enter": {Handler: "Handle"},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events[":Enter"]
	if ev.On != "" {
		t.Errorf("expected empty event type for ':Enter', got %q", ev.On)
	}
	if ev.Key != "Enter" {
		t.Errorf("expected key='Enter', got %q", ev.Key)
	}
}

// --- encodeFacts: same colon edge cases ---
func TestEncodeFacts_Events_MultipleColons(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Events: map[string]vdom.EventHandler{
			"keydown:Enter:extra": {Handler: "Handle"},
		},
	}
	encodeFacts(f, wn)
	ev := wn.Events[0]
	if ev.On != "keydown" {
		t.Errorf("expected on='keydown', got %q", ev.On)
	}
	if ev.Key != "Enter:extra" {
		t.Errorf("expected key='Enter:extra', got %q", ev.Key)
	}
}

// --- Empty handler method name ---
func TestEncodeFactsDiff_Events_EmptyMethod(t *testing.T) {
	d := &vdom.FactsDiff{
		Events: map[string]*vdom.EventHandler{
			"click": {Handler: ""},
		},
	}
	w := encodeFactsDiff(d)
	ev := w.Events["click"]
	if ev.Method != "" {
		t.Errorf("expected empty method, got %q", ev.Method)
	}
}

// --- Empty maps (non-nil but zero length) treated as absent ---
// If the diff produces an empty map to signal "clear all", it gets dropped.
func TestEncodeFactsDiff_EmptyMaps(t *testing.T) {
	d := &vdom.FactsDiff{
		Props:   map[string]any{},
		Attrs:   map[string]string{},
		AttrsNS: map[string]vdom.NSAttr{},
		Styles:  map[string]string{},
		Events:  map[string]*vdom.EventHandler{},
	}
	w := encodeFactsDiff(d)
	// All empty maps should be treated as nil (omitted from JSON)
	if w.Props != nil {
		t.Errorf("expected nil props for empty map, got %v", w.Props)
	}
	if w.Attrs != nil {
		t.Errorf("expected nil attrs for empty map, got %v", w.Attrs)
	}
	if w.AttrsNS != nil {
		t.Errorf("expected nil attrsNS for empty map, got %v", w.AttrsNS)
	}
	if w.Styles != nil {
		t.Errorf("expected nil styles for empty map, got %v", w.Styles)
	}
	if w.Events != nil {
		t.Errorf("expected nil events for empty map, got %v", w.Events)
	}
}

// --- encodeFacts: empty maps treated as absent ---
func TestEncodeFacts_EmptyMaps(t *testing.T) {
	wn := &WireNode{}
	f := &vdom.Facts{
		Props:   map[string]any{},
		Attrs:   map[string]string{},
		AttrsNS: map[string]vdom.NSAttr{},
		Styles:  map[string]string{},
		Events:  map[string]vdom.EventHandler{},
	}
	encodeFacts(f, wn)
	if wn.Props != nil {
		t.Errorf("expected nil props for empty map, got %v", wn.Props)
	}
	if wn.Attrs != nil {
		t.Errorf("expected nil attrs for empty map, got %v", wn.Attrs)
	}
	if wn.AttrsNS != nil {
		t.Errorf("expected nil attrsNS for empty map, got %v", wn.AttrsNS)
	}
	if wn.Styles != nil {
		t.Errorf("expected nil styles for empty map, got %v", wn.Styles)
	}
	if wn.Events != nil {
		t.Errorf("expected nil events for empty map, got %v", wn.Events)
	}
}

// --- NodeID 0 (zero value) ---
func TestEncodePatchMessage_ZeroNodeID(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchText, NodeID: 0, Data: vdom.PatchTextData{Text: "root"}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.NodeId != 0 {
		t.Errorf("expected node_id 0, got %d", dp.NodeId)
	}
}

// --- PatchAppend with nil node in list ---
func TestEncodePatchMessage_Append_NilNode(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchAppend, NodeID: 1, Data: vdom.PatchAppendData{
			Nodes: []vdom.Node{
				&vdom.TextNode{NodeBase: vdom.NodeBase{ID: 10}, Text: "ok"},
				nil, // nil node
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	var trees []*WireNode
	if err := json.Unmarshal(dp.TreeContent, &trees); err != nil {
		t.Fatal(err)
	}
	if len(trees) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(trees))
	}
	if trees[1] != nil {
		t.Errorf("expected nil for nil node, got %+v", trees[1])
	}
}

// --- Plugin patch with nil data ---
func TestEncodePatchMessage_Plugin_NilData(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchPlugin, NodeID: 1, Data: vdom.PatchPluginData{
			Data: nil,
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpPlugin {
		t.Errorf("expected op 'plugin', got %q", dp.Op)
	}
	// json.Marshal(nil) = "null"
	if string(dp.PluginData) != "null" {
		t.Errorf("expected 'null' plugin data, got %q", string(dp.PluginData))
	}
}

// --- Deeply nested lazy patches ---
func TestEncodePatchMessage_Lazy_Nested(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchLazy, NodeID: 1, Data: vdom.PatchLazyData{
			Patches: []vdom.Patch{
				{Type: vdom.PatchLazy, NodeID: 2, Data: vdom.PatchLazyData{
					Patches: []vdom.Patch{
						{Type: vdom.PatchText, NodeID: 3, Data: vdom.PatchTextData{Text: "deep"}},
					},
				}},
			},
		}},
	}
	msg := EncodePatchMessage(patches)
	dp := msg.Patches[0]
	if dp.Op != OpLazy {
		t.Errorf("expected op 'lazy', got %q", dp.Op)
	}
	if len(dp.SubPatches) != 1 {
		t.Fatalf("expected 1 sub-patch, got %d", len(dp.SubPatches))
	}
	inner := dp.SubPatches[0]
	if inner.Op != OpLazy {
		t.Errorf("expected inner op 'lazy', got %q", inner.Op)
	}
	if len(inner.SubPatches) != 1 {
		t.Fatalf("expected 1 inner sub-patch, got %d", len(inner.SubPatches))
	}
	if inner.SubPatches[0].Text != "deep" {
		t.Errorf("expected text 'deep', got %q", inner.SubPatches[0].Text)
	}
}

// ---------------------------------------------------------------------------
// EncodeTree — unknown Node type falls through to final return nil
// ---------------------------------------------------------------------------

// fakeNode is a custom vdom.Node that none of the type-switch cases match.
type fakeNode struct{}

func (fakeNode) NodeType() int        { return 9999 }
func (fakeNode) NodeID() int          { return 42 }
func (fakeNode) DescendantsCount() int { return 0 }
func (fakeNode) IsRemoved() bool      { return false }

func TestEncodeTree_UnknownNodeType_ReturnsNil(t *testing.T) {
	wn := EncodeTree(fakeNode{})
	if wn != nil {
		t.Errorf("expected nil for unknown node type, got %+v", wn)
	}
}

func TestEncodeTreeJSON_UnknownNodeType_ReturnsNull(t *testing.T) {
	b, err := EncodeTreeJSON(fakeNode{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != "null" {
		t.Errorf("expected 'null' JSON, got %q", string(b))
	}
}

func TestEncodeInitTreeMessage_UnknownNodeType(t *testing.T) {
	msg, err := EncodeInitTreeMessage(fakeNode{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != "init" {
		t.Errorf("expected type 'init', got %q", msg.Kind)
	}
	if string(msg.Tree) != "null" {
		t.Errorf("expected tree 'null', got %q", string(msg.Tree))
	}
}
