package godom

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/anupshinde/godom/internal/component"
)

// Test types that need the real godom.Component (for Emit).

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

func newTestCI(comp interface{}) *component.Info {
	v := reflect.ValueOf(comp)
	return &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
	}
}

func TestEmit(t *testing.T) {
	parent := &testComp{Count: 0}
	parentCI := newTestCI(parent)

	child := &testChild{Index: 5}
	childCI := newTestCI(child)
	childCI.Parent = parentCI

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
	childCI.Parent = parentCI
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

// Verify Emit with JSON-serialized args round-trips correctly.
func TestEmit_ArgsRoundTrip(t *testing.T) {
	parent := &testComp{}
	parentCI := newTestCI(parent)

	child := &testChild{}
	childCI := newTestCI(child)
	childCI.Parent = parentCI
	child.Component.ci = childCI

	nameJSON, _ := json.Marshal("Charlie")
	_ = parentCI.CallMethod("SetName", []json.RawMessage{nameJSON})

	if parent.Name != "Charlie" {
		t.Errorf("Name = %q, want Charlie", parent.Name)
	}
}
