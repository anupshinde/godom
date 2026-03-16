package godom

import (
	"reflect"
	"testing"
)

type valTestComp struct {
	Component
	Name      string
	Count     int
	Visible   bool
	InputText string
	Todos     []valTestTodo
}

type valTestTodo struct {
	Text string
	Done bool
}

func (v *valTestComp) Save()                                  {}
func (v *valTestComp) Toggle(i int)                            {}
func (v *valTestComp) Remove(i int)                            {}
func (v *valTestComp) HandleMouse(x, y float64)                {}
func (v *valTestComp) HandleWheel(deltaY float64)              {}
func (v *valTestComp) Reorder(from, to float64)                {}
func (v *valTestComp) Add(color string)                        {}
func (v *valTestComp) Drop(from, to float64, position string)  {}

func newValTestCI() *componentInfo {
	comp := &valTestComp{}
	v := reflect.ValueOf(comp)
	return &componentInfo{
		value:    v,
		typ:      v.Elem().Type(),
		children: make(map[string][]*componentInfo),
		registry: make(map[string]*componentReg),
	}
}

func TestValidateDirectives_ValidField(t *testing.T) {
	html := `<span g-text="Name"></span>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ValidMethod(t *testing.T) {
	html := `<button g-click="Save">Save</button>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ValidMethodWithArgs(t *testing.T) {
	html := `<li g-for="todo, i in Todos"><button g-click="Toggle(i)">T</button></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_UnknownField(t *testing.T) {
	html := `<span g-text="Missing"></span>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestValidateDirectives_UnknownMethod(t *testing.T) {
	html := `<button g-click="Unknown">X</button>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method")
	}
}

func TestValidateDirectives_ForLoop(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-text="todo.Text"></span></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ForLoopUnknownList(t *testing.T) {
	html := `<li g-for="item in Missing"><span g-text="item.Text"></span></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown list field in g-for")
	}
}

func TestValidateDirectives_DottedPath(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-text="todo.Text"></span></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_InvalidDottedPath(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-text="todo.Missing"></span></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for invalid dotted path")
	}
}

func TestValidateDirectives_Keydown(t *testing.T) {
	html := `<input g-keydown="Enter:Save" />`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_KeydownMultiple(t *testing.T) {
	html := `<div g-keydown="Enter:Save;Escape:Remove(0)"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_KeydownMultipleInvalid(t *testing.T) {
	html := `<div g-keydown="Enter:Save;Escape:Unknown"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in multi-binding")
	}
}

func TestValidateDirectives_MouseEvents(t *testing.T) {
	html := `<canvas g-mousedown="HandleMouse" g-mousemove="HandleMouse" g-mouseup="HandleMouse" g-wheel="HandleWheel"></canvas>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_MouseEventUnknown(t *testing.T) {
	html := `<canvas g-mousedown="Unknown"></canvas>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-mousedown")
	}
}

func TestValidateDirectives_Literals(t *testing.T) {
	html := `<span g-text="true"></span><span g-text="42"></span><span g-text="'hello'"></span>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_CheckedAndShow(t *testing.T) {
	html := `<input g-checked="Visible" /><div g-show="Visible"></div><div g-if="Visible"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ClassDirective(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-class:done="todo.Done"></span></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_StyleDirective(t *testing.T) {
	html := `<div g-style:background-color="Name"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ChildComponentFallback(t *testing.T) {
	// Simulate a child component registered with a field "Local"
	type childComp struct {
		Component
		Local string
	}
	childCI := newValTestCI()
	childCI.registry["my-child"] = &componentReg{
		typ: reflect.TypeOf(childComp{}),
	}

	// "Local" doesn't exist on the parent, but exists on the child
	html := `<span g-text="Local"></span>`
	if err := validateDirectives(html, childCI); err != nil {
		t.Errorf("unexpected error (should fallback to child): %v", err)
	}
}

func TestValidateDirectives_Draggable(t *testing.T) {
	html := `<li g-for="todo, i in Todos"><span g-draggable="i"></span></li>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DraggableWithGroup(t *testing.T) {
	html := `<div g-draggable.palette="'red'"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_Dropzone(t *testing.T) {
	html := `<div g-dropzone="'canvas'"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DropEvent(t *testing.T) {
	html := `<div g-drop="Reorder"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DropEventWithGroup(t *testing.T) {
	html := `<div g-drop.palette="Add"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DropEventUnknownMethod(t *testing.T) {
	html := `<div g-drop="Unknown"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-drop")
	}
}

func TestValidateDirectives_DraggableUnknownField(t *testing.T) {
	html := `<div g-draggable="Missing"></div>`
	ci := newValTestCI()
	if err := validateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-draggable")
	}
}

func TestIsLiteral(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"true", true},
		{"false", true},
		{"42", true},
		{"0", true},
		{`"hello"`, true},
		{`'world'`, true},
		{"Name", false},
		{"todo.Done", false},
	}

	for _, tt := range tests {
		if got := isLiteral(tt.s); got != tt.want {
			t.Errorf("isLiteral(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}
