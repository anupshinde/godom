package godom

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Test component types used across tests.

type testComp struct {
	Component
	Name  string
	Count int
	Items []string
}

func (t *testComp) Increment() {
	t.Count++
}

func (t *testComp) SetName(name string) {
	t.Name = name
}

func (t *testComp) Add(a, b int) {
	t.Count = a + b
}

type testChild struct {
	Component
	Text  string `godom:"prop"`
	Index int    `godom:"prop"`
	Local string
}

func (c *testChild) DoSomething() {
	c.Local = "done"
}

func (c *testChild) Bubble() {
	c.Emit("HandleBubble", c.Index)
}

// newTestCI creates a componentInfo for testing.
func newTestCI(comp interface{}) *componentInfo {
	v := reflect.ValueOf(comp)
	return &componentInfo{
		value:    v,
		typ:      v.Elem().Type(),
		children: make(map[string][]*componentInfo),
	}
}

func TestGetState(t *testing.T) {
	comp := &testComp{Name: "Alice", Count: 3, Items: []string{"a", "b"}}
	ci := newTestCI(comp)

	data, err := ci.getState()
	if err != nil {
		t.Fatal(err)
	}

	var state map[string]interface{}
	json.Unmarshal(data, &state)

	if state["Name"] != "Alice" {
		t.Errorf("Name = %v, want Alice", state["Name"])
	}
	// JSON numbers are float64
	if state["Count"] != float64(3) {
		t.Errorf("Count = %v, want 3", state["Count"])
	}
	items := state["Items"].([]interface{})
	if len(items) != 2 || items[0] != "a" {
		t.Errorf("Items = %v, want [a b]", items)
	}
}

func TestGetState_ExcludesComponent(t *testing.T) {
	comp := &testComp{Name: "Bob"}
	ci := newTestCI(comp)

	data, _ := ci.getState()
	var state map[string]interface{}
	json.Unmarshal(data, &state)

	if _, ok := state["Component"]; ok {
		t.Error("getState should exclude the embedded Component field")
	}
}

func TestChangedFields(t *testing.T) {
	comp := &testComp{Name: "Alice", Count: 1}
	ci := newTestCI(comp)

	old := ci.snapshotState()
	comp.Name = "Bob"
	comp.Count = 2
	newer := ci.snapshotState()

	changed := ci.changedFields(old, newer)
	if len(changed) != 2 {
		t.Fatalf("expected 2 changed fields, got %d: %v", len(changed), changed)
	}

	has := map[string]bool{}
	for _, f := range changed {
		has[f] = true
	}
	if !has["Name"] || !has["Count"] {
		t.Errorf("expected Name and Count to be changed, got %v", changed)
	}
}

func TestChangedFields_NoChange(t *testing.T) {
	comp := &testComp{Name: "Alice", Count: 1}
	ci := newTestCI(comp)

	old := ci.snapshotState()
	newer := ci.snapshotState()

	changed := ci.changedFields(old, newer)
	if len(changed) != 0 {
		t.Errorf("expected no changes, got %v", changed)
	}
}

func TestCallMethod_NoArgs(t *testing.T) {
	comp := &testComp{Count: 5}
	ci := newTestCI(comp)

	err := ci.callMethod("Increment", nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Count != 6 {
		t.Errorf("Count = %d, want 6", comp.Count)
	}
}

func TestCallMethod_WithArgs(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	nameJSON, _ := json.Marshal("Charlie")
	err := ci.callMethod("SetName", []json.RawMessage{nameJSON})
	if err != nil {
		t.Fatal(err)
	}
	if comp.Name != "Charlie" {
		t.Errorf("Name = %q, want Charlie", comp.Name)
	}
}

func TestCallMethod_MultipleArgs(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	a, _ := json.Marshal(3)
	b, _ := json.Marshal(7)
	err := ci.callMethod("Add", []json.RawMessage{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if comp.Count != 10 {
		t.Errorf("Count = %d, want 10", comp.Count)
	}
}

func TestCallMethod_NotFound(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	err := ci.callMethod("NonExistent", nil)
	if err == nil {
		t.Error("expected error for non-existent method")
	}
}

func TestCallMethod_WrongArgCount(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	a, _ := json.Marshal(1)
	err := ci.callMethod("Add", []json.RawMessage{a})
	if err == nil {
		t.Error("expected error for wrong arg count")
	}
}

func TestCallMethod_ExtraArgsIgnored(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	a, _ := json.Marshal(3)
	b, _ := json.Marshal(7)
	extra, _ := json.Marshal("above")
	err := ci.callMethod("Add", []json.RawMessage{a, b, extra})
	if err != nil {
		t.Errorf("extra args should be ignored, got error: %v", err)
	}
	if comp.Count != 10 {
		t.Errorf("Count = %d, want 10", comp.Count)
	}
}

func TestSetField(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	err := ci.setField("Name", json.RawMessage(`"Dave"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Name != "Dave" {
		t.Errorf("Name = %q, want Dave", comp.Name)
	}

	err = ci.setField("Count", json.RawMessage(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Count != 42 {
		t.Errorf("Count = %d, want 42", comp.Count)
	}
}

func TestSetField_NotFound(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	err := ci.setField("Missing", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for missing field")
	}
}

func TestPropFieldNames(t *testing.T) {
	typ := reflect.TypeOf(testChild{})
	props := propFieldNames(typ)

	if !props["Text"] || !props["Index"] {
		t.Errorf("expected Text and Index as props, got %v", props)
	}
	if props["Local"] {
		t.Error("Local should not be a prop")
	}
}

func TestSetProps(t *testing.T) {
	child := &testChild{}
	ci := newTestCI(child)
	ci.propFields = propFieldNames(ci.typ)

	ci.setProps(map[string]interface{}{
		"Text":  "hello",
		"Index": 3,
	})

	if child.Text != "hello" {
		t.Errorf("Text = %q, want hello", child.Text)
	}
	if child.Index != 3 {
		t.Errorf("Index = %d, want 3", child.Index)
	}
}

func TestSetProps_TypeConversion(t *testing.T) {
	child := &testChild{}
	ci := newTestCI(child)
	ci.propFields = propFieldNames(ci.typ)

	// float64 → int (JSON numbers come as float64)
	ci.setProps(map[string]interface{}{
		"Index": float64(7),
	})

	if child.Index != 7 {
		t.Errorf("Index = %d, want 7", child.Index)
	}
}

func TestEmit(t *testing.T) {
	parent := &testComp{Count: 0}
	parentCI := newTestCI(parent)

	child := &testChild{Index: 5}
	childCI := newTestCI(child)
	childCI.parent = parentCI

	// Wire up the Component.ci pointer
	child.Component.ci = childCI

	child.Emit("Add", 3, 7)

	if parent.Count != 10 {
		t.Errorf("parent.Count = %d, want 10", parent.Count)
	}
}

func TestEmit_NoMatchingMethod(t *testing.T) {
	parent := &testComp{Count: 0}
	parentCI := newTestCI(parent)

	child := &testChild{}
	childCI := newTestCI(child)
	childCI.parent = parentCI
	child.Component.ci = childCI

	// Should not panic even if method doesn't exist
	child.Emit("NonExistentMethod")

	if parent.Count != 0 {
		t.Errorf("parent.Count should be unchanged, got %d", parent.Count)
	}
}

func TestEmit_NilCI(t *testing.T) {
	child := &testChild{}
	// Component.ci is nil — should not panic
	child.Emit("Whatever")
}

func TestHasField(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	if !ci.hasField("Name") {
		t.Error("expected Name to be a valid field")
	}
	if !ci.hasField("Count") {
		t.Error("expected Count to be a valid field")
	}
	if ci.hasField("Component") {
		t.Error("Component should not count as a field")
	}
	if ci.hasField("missing") {
		t.Error("missing should not be a valid field")
	}
}

func TestHasMethod(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	if !ci.hasMethod("Increment") {
		t.Error("expected Increment to be a valid method")
	}
	if ci.hasMethod("missing") {
		t.Error("missing should not be a valid method")
	}
}

func TestParseCallExpr(t *testing.T) {
	tests := []struct {
		expr     string
		wantName string
		wantArgs []string
	}{
		{"Toggle", "Toggle", nil},
		{"Toggle(i)", "Toggle", []string{"i"}},
		{"Remove(i)", "Remove", []string{"i"}},
		{"Add(a, b)", "Add", []string{"a", "b"}},
		{"NoArgs()", "NoArgs", nil},
	}

	for _, tt := range tests {
		name, args := parseCallExpr(tt.expr)
		if name != tt.wantName {
			t.Errorf("parseCallExpr(%q) name = %q, want %q", tt.expr, name, tt.wantName)
		}
		if len(args) != len(tt.wantArgs) {
			t.Errorf("parseCallExpr(%q) args = %v, want %v", tt.expr, args, tt.wantArgs)
			continue
		}
		for i, a := range args {
			if a != tt.wantArgs[i] {
				t.Errorf("parseCallExpr(%q) arg[%d] = %q, want %q", tt.expr, i, a, tt.wantArgs[i])
			}
		}
	}
}

