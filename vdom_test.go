package godom

import (
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

// counterApp mirrors the counter example's state struct.
type counterApp struct {
	Component
	Count int
	Step  int
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

func makeCounterCI(app *counterApp) *componentInfo {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()

	templates, err := parseTemplate(counterHTML, nil)
	if err != nil {
		panic(err)
	}

	return &componentInfo{
		value:         v,
		typ:           t,
		vdomTemplates: templates,
	}
}

func TestVDOMBuildInit(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	msg := vdomBuildInit(ci)

	if msg.Type != "init" {
		t.Fatalf("expected type 'init', got %q", msg.Type)
	}

	html := string(msg.Html)

	// Should contain the resolved count (5, not 0)
	if !strings.Contains(html, ">5<") {
		t.Errorf("expected count '5' in HTML, got: %s", html)
	}

	// Should contain buttons
	if !strings.Contains(html, "<button") {
		t.Errorf("expected <button> in HTML, got: %s", html)
	}

	// Should contain input with value="1" (Step=1)
	if !strings.Contains(html, `value="1"`) {
		t.Errorf("expected value=\"1\" in HTML, got: %s", html)
	}

	// Should have events registered
	if len(msg.Events) == 0 {
		t.Fatal("expected events in init message")
	}

	// Should have click events for Decrement and Increment
	var hasClick bool
	for _, e := range msg.Events {
		if e.Event == "click" {
			hasClick = true
		}
	}
	if !hasClick {
		t.Error("expected click events")
	}

	// Should have input event for bind
	var hasInput bool
	for _, e := range msg.Events {
		if e.Event == "input" {
			hasInput = true
		}
	}
	if !hasInput {
		t.Error("expected input event for g-bind")
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
	_ = vdomBuildInit(ci)

	// Simulate Increment
	app.Count = 1
	msg := vdomBuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after increment")
	}
	if msg.Type != "patch" {
		t.Fatalf("expected type 'patch', got %q", msg.Type)
	}

	// Should have a text patch changing "0" to "1"
	var hasTextPatch bool
	for _, p := range msg.Patches {
		if p.Op == opText && p.Text == "1" {
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

	_ = vdomBuildInit(ci)

	// No state change
	msg := vdomBuildUpdate(ci)
	if msg != nil {
		t.Errorf("expected nil message when nothing changed, got type=%q patches=%d", msg.Type, len(msg.Patches))
	}
}

func TestVDOMBuildUpdate_BindStep(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	_ = vdomBuildInit(ci)

	// Simulate step change (as if g-bind updated it)
	app.Step = 5
	msg := vdomBuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after step change")
	}

	// Should have a facts patch (value property changed on input)
	var hasFactsPatch bool
	for _, p := range msg.Patches {
		if p.Op == opFacts {
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

	_ = vdomBuildInit(ci)

	// First increment
	app.Count = 2
	msg1 := vdomBuildUpdate(ci)
	if msg1 == nil {
		t.Fatal("expected patch for first increment")
	}

	// Second increment
	app.Count = 4
	msg2 := vdomBuildUpdate(ci)
	if msg2 == nil {
		t.Fatal("expected patch for second increment")
	}

	// Check the second patch has text "4"
	var hasText4 bool
	for _, p := range msg2.Patches {
		if p.Op == opText && p.Text == "4" {
			hasText4 = true
		}
	}
	if !hasText4 {
		t.Error("expected text patch '2' → '4'")
	}
}
