package server

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/render"
	"github.com/anupshinde/godom/internal/vdom"
	"google.golang.org/protobuf/proto"
)

// counterApp mirrors the counter example's state struct.
type counterApp struct {
	Component struct{} // dummy — matches the field name check
	Count     int
	Step      int
}

func (a *counterApp) Increment() {
	a.Count += a.Step
}

func (a *counterApp) Decrement() {
	a.Count -= a.Step
}

const counterHTML = `<!DOCTYPE html><html><head><title>Counter</title></head><body>
    <h1><span g-text="Count">0</span></h1>
    <div class="controls">
        <button g-click="Decrement">−</button>
        <button g-click="Increment">+</button>
    </div>
    <div class="step">
        <label>Step size:</label>
        <input type="number" min="1" max="100" g-bind="Step"/>
    </div>
</body></html>`

func makeCounterCI(app *counterApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()

	templates, err := vdom.ParseTemplate(counterHTML, nil)
	if err != nil {
		panic(err)
	}

	return &component.Info{
		Value:         v,
		Typ:           t,
		VDOMTemplates: templates,
	}
}

func TestVDOMBuildInit(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	msg := BuildInit(ci)

	if msg.Type != "init" {
		t.Fatalf("expected type 'init', got %q", msg.Type)
	}

	if len(msg.Tree) == 0 {
		t.Fatal("expected non-empty tree JSON")
	}

	// Decode tree and verify structure
	var tree render.WireNode
	if err := json.Unmarshal(msg.Tree, &tree); err != nil {
		t.Fatal(err)
	}

	if tree.Tag != "body" {
		t.Errorf("expected root tag 'body', got %q", tree.Tag)
	}

	// Should have children (h1, div.controls, div.step)
	if len(tree.Children) == 0 {
		t.Error("expected children in tree")
	}

	// Find the text node with "5" (the resolved count)
	found := findTextInTree(&tree, "5")
	if !found {
		t.Error("expected count '5' in tree")
	}

	// Should have events (click)
	foundClick := findEventInTree(&tree, "click")
	if !foundClick {
		t.Error("expected click event in tree")
	}

	// Should be serializable
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty protobuf bytes")
	}
}

func TestVDOMBuildUpdate_Increment(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	// Initial render
	_ = BuildInit(ci)

	// Simulate Increment
	app.Count = 1
	msg := BuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after increment")
	}
	if msg.Type != "patch" {
		t.Fatalf("expected type 'patch', got %q", msg.Type)
	}

	// Should have a text patch changing "0" to "1"
	var hasTextPatch bool
	for _, p := range msg.Patches {
		if p.Op == render.OpText && p.Text == "1" {
			hasTextPatch = true
		}
	}
	if !hasTextPatch {
		t.Errorf("expected text patch '0' → '1', patches: %+v", msg.Patches)
	}

	// Should be serializable
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty protobuf bytes")
	}
}

func TestVDOMBuildUpdate_NoChange(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// No state change
	msg := BuildUpdate(ci)
	if msg != nil {
		t.Errorf("expected nil message when nothing changed, got type=%q patches=%d", msg.Type, len(msg.Patches))
	}
}

func TestVDOMBuildUpdate_BindStep(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// Simulate step change (as if g-bind updated it)
	app.Step = 5
	msg := BuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after step change")
	}

	// Should have a facts patch (value property changed on input)
	var hasFactsPatch bool
	for _, p := range msg.Patches {
		if p.Op == render.OpFacts {
			hasFactsPatch = true
		}
	}
	if !hasFactsPatch {
		t.Errorf("expected facts patch for step change, patches: %+v", msg.Patches)
	}
}

func TestVDOMBuildUpdate_MultipleIncrements(t *testing.T) {
	app := &counterApp{Step: 2, Count: 0}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// First increment
	app.Count = 2
	msg1 := BuildUpdate(ci)
	if msg1 == nil {
		t.Fatal("expected patch for first increment")
	}

	// Second increment
	app.Count = 4
	msg2 := BuildUpdate(ci)
	if msg2 == nil {
		t.Fatal("expected patch for second increment")
	}

	// Check the second patch has text "4"
	var hasText4 bool
	for _, p := range msg2.Patches {
		if p.Op == render.OpText && p.Text == "4" {
			hasText4 = true
		}
	}
	if !hasText4 {
		t.Error("expected text patch '2' → '4'")
	}
}

// --- Helpers ---

func findTextInTree(node *render.WireNode, text string) bool {
	if node.Type == "text" && node.Text == text {
		return true
	}
	for _, child := range node.Children {
		if findTextInTree(child, text) {
			return true
		}
	}
	return false
}

func findEventInTree(node *render.WireNode, eventName string) bool {
	for _, ev := range node.Events {
		if ev.On == eventName {
			return true
		}
	}
	for _, child := range node.Children {
		if findEventInTree(child, eventName) {
			return true
		}
	}
	return false
}
