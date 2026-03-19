package render

import (
	"encoding/json"
	"reflect"
	"testing"

	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/anupshinde/godom/internal/vdom"
	"google.golang.org/protobuf/proto"
)

func TestEncodePatchMessage_Text(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchText, NodeID: 3, Data: vdom.PatchTextData{Text: "hello"}},
	}
	msg := EncodePatchMessage(patches)
	if msg.Type != "patch" {
		t.Errorf("expected type 'patch', got %q", msg.Type)
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

	msg2 := &gproto.VDomMessage{}
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

// ---------------------------------------------------------------------------
// Tree encoder tests
// ---------------------------------------------------------------------------

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

func TestEncodeTree_ComponentNode(t *testing.T) {
	// Components are transparent — we encode their subtree
	n := &vdom.ComponentNode{
		NodeBase: vdom.NodeBase{ID: 7},
		SubTree:  &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 8}, Text: "from component"},
	}
	wn := EncodeTree(n)
	if wn.ID != 8 {
		t.Errorf("expected subtree ID 8, got %d", wn.ID)
	}
	if wn.Text != "from component" {
		t.Errorf("expected text 'from component', got %q", wn.Text)
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
	if msg.Type != "init" {
		t.Errorf("expected type 'init', got %q", msg.Type)
	}
	if len(msg.Tree) == 0 {
		t.Error("expected non-empty tree JSON")
	}

	// Verify it's serializable via protobuf
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	msg2 := &gproto.VDomMessage{}
	if err := proto.Unmarshal(data, msg2); err != nil {
		t.Fatal(err)
	}
	if msg2.Type != "init" {
		t.Errorf("expected 'init' after round-trip, got %q", msg2.Type)
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

	templates, err := vdom.ParseTemplate(htmlStr, nil)
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
	if msg.Type != "patch" {
		t.Errorf("expected 'patch', got %q", msg.Type)
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
