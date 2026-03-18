package godom

import (
	"encoding/json"
	"testing"

	"github.com/anupshinde/godom/vdom"
	"google.golang.org/protobuf/proto"
)

func TestEncodeInitMessage(t *testing.T) {
	msg := encodeInitMessage("<div>hello</div>", nil)
	if msg.Type != "init" {
		t.Errorf("expected type 'init', got %q", msg.Type)
	}
	if string(msg.Html) != "<div>hello</div>" {
		t.Errorf("expected html content, got %q", string(msg.Html))
	}
}

func TestEncodeInitMessage_Serializable(t *testing.T) {
	msg := encodeInitMessage("<div>test</div>", []*EventSetup{
		{Gid: "g1", Event: "click", Msg: []byte("test")},
	})
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty protobuf bytes")
	}
	// Round-trip
	msg2 := &VDomMessage{}
	if err := proto.Unmarshal(data, msg2); err != nil {
		t.Fatal(err)
	}
	if msg2.Type != "init" {
		t.Errorf("expected 'init', got %q", msg2.Type)
	}
	if len(msg2.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(msg2.Events))
	}
}

func TestEncodePatchMessage_Text(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchText, Index: 3, Data: vdom.PatchTextData{Text: "hello"}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	if msg.Type != "patch" {
		t.Errorf("expected type 'patch', got %q", msg.Type)
	}
	if len(msg.Patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(msg.Patches))
	}
	dp := msg.Patches[0]
	if dp.Op != opText {
		t.Errorf("expected op 'text', got %q", dp.Op)
	}
	if dp.Index != 3 {
		t.Errorf("expected index 3, got %d", dp.Index)
	}
	if dp.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", dp.Text)
	}
}

func TestEncodePatchMessage_Facts(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchFacts, Index: 1, Data: vdom.PatchFactsData{
			Diff: vdom.FactsDiff{
				Props:  map[string]any{"className": "active"},
				Styles: map[string]string{"display": "none"},
			},
		}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	dp := msg.Patches[0]
	if dp.Op != opFacts {
		t.Errorf("expected op 'facts', got %q", dp.Op)
	}
	// Decode the JSON to verify structure
	var fd wireFactsDiff
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
		{Type: vdom.PatchAppend, Index: 0, Data: vdom.PatchAppendData{
			Nodes: []vdom.Node{&vdom.TextNode{Text: "new child"}},
		}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	dp := msg.Patches[0]
	if dp.Op != opAppend {
		t.Errorf("expected op 'append', got %q", dp.Op)
	}
	if len(dp.HtmlContent) == 0 {
		t.Error("expected HTML content for append")
	}
}

func TestEncodePatchMessage_RemoveLast(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRemoveLast, Index: 0, Data: vdom.PatchRemoveLastData{Count: 3}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	dp := msg.Patches[0]
	if dp.Op != opRemoveLast {
		t.Errorf("expected op 'remove-last', got %q", dp.Op)
	}
	if dp.Count != 3 {
		t.Errorf("expected count 3, got %d", dp.Count)
	}
}

func TestEncodePatchMessage_Redraw(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchRedraw, Index: 2, Data: vdom.PatchRedrawData{
			Node: &vdom.ElementNode{
				Tag: "span",
				Children: []vdom.Node{&vdom.TextNode{Text: "replaced"}},
			},
		}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	dp := msg.Patches[0]
	if dp.Op != opRedraw {
		t.Errorf("expected op 'redraw', got %q", dp.Op)
	}
	html := string(dp.HtmlContent)
	if html == "" {
		t.Error("expected HTML content for redraw")
	}
}

func TestEncodePatchMessage_Plugin(t *testing.T) {
	patches := []vdom.Patch{
		{Type: vdom.PatchPlugin, Index: 5, Data: vdom.PatchPluginData{
			Data: map[string]int{"value": 42},
		}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	dp := msg.Patches[0]
	if dp.Op != opPlugin {
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
		{Type: vdom.PatchLazy, Index: 0, Data: vdom.PatchLazyData{
			Patches: []vdom.Patch{
				{Type: vdom.PatchText, Index: 1, Data: vdom.PatchTextData{Text: "inner"}},
			},
		}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})
	dp := msg.Patches[0]
	if dp.Op != opLazy {
		t.Errorf("expected op 'lazy', got %q", dp.Op)
	}
	if len(dp.SubPatches) != 1 {
		t.Fatalf("expected 1 sub-patch, got %d", len(dp.SubPatches))
	}
	if dp.SubPatches[0].Op != opText {
		t.Errorf("expected sub-patch op 'text', got %q", dp.SubPatches[0].Op)
	}
}

func TestEncodePatchMessage_Serializable(t *testing.T) {
	// Verify the full message survives protobuf round-trip
	patches := []vdom.Patch{
		{Type: vdom.PatchText, Index: 1, Data: vdom.PatchTextData{Text: "hello"}},
		{Type: vdom.PatchFacts, Index: 2, Data: vdom.PatchFactsData{
			Diff: vdom.FactsDiff{Styles: map[string]string{"color": "red"}},
		}},
		{Type: vdom.PatchRemoveLast, Index: 0, Data: vdom.PatchRemoveLastData{Count: 1}},
	}
	msg := encodePatchMessage(patches, &gidCounter{})

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	msg2 := &VDomMessage{}
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
	msg := encodePatchMessage(patches, &gidCounter{})
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
		if dp.Op == opText && dp.Text == "10" {
			hasTextPatch = true
		}
	}
	if !hasTextPatch {
		t.Error("expected text patch '5' → '10'")
	}
}
