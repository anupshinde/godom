package template

import (
	"reflect"
	"strings"
	"testing"

	"github.com/anupshinde/godom/internal/component"
)

type valTestComp struct {
	Component struct{} // dummy — matches the field name check in component.Info
	Name      string
	Count     int
	Visible   bool
	InputText string
	Todos     []valTestTodo
	Inputs    map[string]string
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
func (v *valTestComp) Computed() string                        { return "" }

func newValTestCI() *component.Info {
	comp := &valTestComp{}
	v := reflect.ValueOf(comp)
	return &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
}

func TestValidateDirectives_ValidField(t *testing.T) {
	html := `<span g-text="Name"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ValidMethod(t *testing.T) {
	html := `<button g-click="Save">Save</button>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ValidMethodWithArgs(t *testing.T) {
	html := `<li g-for="todo, i in Todos"><button g-click="Toggle(i)">T</button></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_UnknownField(t *testing.T) {
	html := `<span g-text="Missing"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestValidateDirectives_UnknownMethod(t *testing.T) {
	html := `<button g-click="Unknown">X</button>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method")
	}
}

func TestValidateDirectives_ForLoop(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-text="todo.Text"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ForLoopUnknownList(t *testing.T) {
	html := `<li g-for="item in Missing"><span g-text="item.Text"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown list field in g-for")
	}
}

func TestValidateDirectives_DottedPath(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-text="todo.Text"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_InvalidDottedPath(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-text="todo.Missing"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for invalid dotted path")
	}
}

func TestValidateDirectives_Keydown(t *testing.T) {
	html := `<input g-keydown="Enter:Save" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_KeydownMultiple(t *testing.T) {
	html := `<div g-keydown="Enter:Save;Escape:Remove(0)"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_KeydownMultipleInvalid(t *testing.T) {
	html := `<div g-keydown="Enter:Save;Escape:Unknown"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in multi-binding")
	}
}

func TestValidateDirectives_MouseEvents(t *testing.T) {
	html := `<canvas g-mousedown="HandleMouse" g-mousemove="HandleMouse" g-mouseup="HandleMouse" g-wheel="HandleWheel"></canvas>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_MouseEventUnknown(t *testing.T) {
	html := `<canvas g-mousedown="Unknown"></canvas>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-mousedown")
	}
}

func TestValidateDirectives_Literals(t *testing.T) {
	html := `<span g-text="true"></span><span g-text="42"></span><span g-text="'hello'"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_CheckedAndShow(t *testing.T) {
	html := `<input g-checked="Visible" /><div g-show="Visible"></div><div g-if="Visible"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ClassDirective(t *testing.T) {
	html := `<li g-for="todo in Todos"><span g-class:done="todo.Done"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_StyleDirective(t *testing.T) {
	html := `<div g-style:background-color="Name"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ChildComponentFallback(t *testing.T) {
	// Simulate a child component registered with a field "Local"
	type childComp struct {
		Component struct{}
		Local     string
	}
	childCI := newValTestCI()
	childCI.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childComp{}),
	}

	// "Local" doesn't exist on the parent, but exists on the child
	html := `<span g-text="Local"></span>`
	if err := ValidateDirectives(html, childCI); err != nil {
		t.Errorf("unexpected error (should fallback to child): %v", err)
	}
}

func TestValidateDirectives_Draggable(t *testing.T) {
	html := `<li g-for="todo, i in Todos"><span g-draggable="i"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DraggableWithGroup(t *testing.T) {
	html := `<div g-draggable:palette="'red'"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_Dropzone(t *testing.T) {
	html := `<div g-dropzone="'canvas'"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DropEvent(t *testing.T) {
	html := `<div g-drop="Reorder"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DropEventWithGroup(t *testing.T) {
	html := `<div g-drop:palette="Add"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_DropEventUnknownMethod(t *testing.T) {
	html := `<div g-drop="Unknown"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-drop")
	}
}

func TestValidateDirectives_DraggableUnknownField(t *testing.T) {
	html := `<div g-draggable="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-draggable")
	}
}

// --- Nested g-for validation tests ---

type valNestedComp struct {
	Component struct{}
	Groups    []valNestedGroup
}

type valNestedGroup struct {
	Name    string
	Options []string
}

func (v *valNestedComp) Save() {}

func TestValidateDirectives_NestedFor(t *testing.T) {
	html := `<div g-for="group in Groups"><span g-text="group.Name"></span><li g-for="opt in group.Options"><span g-text="opt"></span></li></div>`
	comp := &valNestedComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_NestedForUnknownPath(t *testing.T) {
	html := `<div g-for="group in Groups"><li g-for="opt in group.Missing"></li></div>`
	comp := &valNestedComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
	// "group.Missing" — "group" is a valid loop var, so this passes validation
	// (we trust the loop variable's type at the validate level)
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
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
		if got := IsLiteral(tt.s); got != tt.want {
			t.Errorf("IsLiteral(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

// === Additional coverage tests ===

// --- g-bind validation (validateBindExpr — was 0% covered) ---

func TestValidateDirectives_BindValidField(t *testing.T) {
	html := `<input g-bind="Name" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_BindMethodNotField(t *testing.T) {
	// g-bind requires a field for two-way binding; "Save" is a method
	html := `<input g-bind="Save" />`
	ci := newValTestCI()
	err := ValidateDirectives(html, ci)
	if err == nil {
		t.Error("expected error for g-bind referencing a method")
	}
	if err != nil && !strings.Contains(err.Error(), "method") {
		t.Errorf("expected error mentioning 'method', got: %v", err)
	}
}

func TestValidateDirectives_BindEmptyExpr(t *testing.T) {
	html := `<input g-bind="" />`
	ci := newValTestCI()
	err := ValidateDirectives(html, ci)
	if err == nil {
		t.Error("expected error for empty g-bind expression")
	}
}

func TestValidateDirectives_BindDottedPathInLoop(t *testing.T) {
	// g-bind with a dotted path through a loop variable
	html := `<li g-for="todo in Todos"><input g-bind="todo.Text" /></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_BindBracketSyntax(t *testing.T) {
	// g-bind="Inputs[first]" should validate — bracket syntax extracts root "Inputs"
	html := `<input g-bind="Inputs[first]" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error for bracket bind: %v", err)
	}
}

func TestValidateDirectives_BindUnknownField(t *testing.T) {
	html := `<input g-bind="Missing" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-bind")
	}
}

// --- validateFieldExpr: bracket syntax, negation, computed method, empty expr ---

func TestValidateDirectives_FieldBracketSyntax(t *testing.T) {
	// g-text="Inputs[first]" — bracket syntax extracts root "Inputs" for field lookup
	html := `<span g-text="Inputs[first]"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error for bracket field expr: %v", err)
	}
}

func TestValidateDirectives_Negation(t *testing.T) {
	// g-show with negated expression "!Visible"
	html := `<div g-show="!Visible"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_NegationUnknown(t *testing.T) {
	html := `<div g-show="!Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for negated unknown field")
	}
}

func TestValidateDirectives_ComputedMethod(t *testing.T) {
	// Computed() is a zero-arg, single-return method — should be valid in g-text
	html := `<span g-text="Computed"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_MethodNotComputed(t *testing.T) {
	// Save() has no return value — not a valid computed expression
	html := `<span g-text="Save"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for method with no return value used as field expression")
	}
}

func TestValidateDirectives_EmptyTextExpr(t *testing.T) {
	html := `<span g-text=""></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for empty g-text expression")
	}
}

func TestValidateDirectives_EmptyValueExpr(t *testing.T) {
	html := `<input g-value="" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for empty g-value expression")
	}
}

// --- validateAgainstChildren: click and keydown fallback paths ---

type childMethodComp struct {
	Component struct{}
	Local     string
}

func (c *childMethodComp) ChildAction() {}
func (c *childMethodComp) ChildDrop(from, to float64) {}

func TestValidateDirectives_ChildFallback_Click(t *testing.T) {
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	// ChildAction doesn't exist on parent, but exists on child
	html := `<button g-click="ChildAction">X</button>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (should fallback to child): %v", err)
	}
}

func TestValidateDirectives_ChildFallback_Keydown(t *testing.T) {
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	html := `<input g-keydown="Enter:ChildAction" />`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (should fallback to child keydown): %v", err)
	}
}

func TestValidateDirectives_ChildFallback_Drop(t *testing.T) {
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	html := `<div g-drop="ChildDrop"></div>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (should fallback to child drop): %v", err)
	}
}

func TestValidateDirectives_ChildFallback_FieldNotOnEither(t *testing.T) {
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	// "Nonexistent" is not on parent or child
	html := `<span g-text="Nonexistent"></span>`
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error when field is on neither parent nor child")
	}
}

func TestValidateDirectives_ChildFallback_MethodNotOnEither(t *testing.T) {
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	html := `<button g-click="Nonexistent">X</button>`
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error when method is on neither parent nor child")
	}
}

// --- validateMethodRef: invalid arg propagation ---

func TestValidateDirectives_MethodWithInvalidArg(t *testing.T) {
	html := `<button g-click="Toggle(unknown)">X</button>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown argument in method call")
	}
}

func TestValidateDirectives_MethodWithDottedLoopVarArg(t *testing.T) {
	// Dotted arg through loop variable
	html := `<li g-for="todo in Todos"><button g-click="Remove(todo.Done)">X</button></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_MethodWithFieldArg(t *testing.T) {
	// Arg referencing a top-level field
	html := `<button g-click="Toggle(Count)">X</button>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- validateKeydownExpr: trailing semicolon ---

func TestValidateDirectives_KeydownTrailingSemicolon(t *testing.T) {
	html := `<input g-keydown="Enter:Save;" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- buildChildCIs: nil registry ---

func TestValidateDirectives_NilRegistry(t *testing.T) {
	comp := &valTestComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: nil, // nil registry
	}
	html := `<span g-text="Name"></span>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_NilRegistryUnknownField(t *testing.T) {
	comp := &valTestComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: nil,
	}
	html := `<span g-text="Missing"></span>`
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field with nil registry")
	}
}

// --- validateTypePath and resolveFieldType: pointer deref, non-struct intermediate ---

type valPtrComp struct {
	Component struct{}
	Inner     *valInnerStruct
	Items     []valPtrItem
}

type valInnerStruct struct {
	Value string
}

type valPtrItem struct {
	Sub *valSubStruct
}

type valSubStruct struct {
	Options []string
}

func (v *valPtrComp) Save() {}

func newValPtrCI() *component.Info {
	comp := &valPtrComp{}
	v := reflect.ValueOf(comp)
	return &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
}

func TestValidateDirectives_PointerDeref(t *testing.T) {
	// Inner is *valInnerStruct — validateTypePath should deref the pointer
	html := `<span g-text="Inner.Value"></span>`
	ci := newValPtrCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_PointerDerefInvalidField(t *testing.T) {
	html := `<span g-text="Inner.Missing"></span>`
	ci := newValPtrCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for invalid field through pointer")
	}
}

type valMapComp struct {
	Component struct{}
	Data      map[string]string
}

func (v *valMapComp) Save() {}

func newValMapCI() *component.Info {
	comp := &valMapComp{}
	v := reflect.ValueOf(comp)
	return &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
}

func TestValidateDirectives_NonStructIntermediate(t *testing.T) {
	// Data is map[string]string — validateTypePath can't validate further, returns nil
	html := `<span g-text="Data.Key"></span>`
	ci := newValMapCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (non-struct intermediate should be tolerated): %v", err)
	}
}

// --- resolveFieldType via nested g-for with pointer deref ---

func TestValidateDirectives_NestedForPointerDeref(t *testing.T) {
	// g-for through a pointer field: item.Sub.Options where Sub is *valSubStruct
	html := `<div g-for="item in Items"><li g-for="opt in item.Sub.Options"><span g-text="opt"></span></li></div>`
	ci := newValPtrCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- resolveFieldType: non-struct intermediate via nested g-for ---

type valMapListComp struct {
	Component struct{}
	Items     []valMapListItem
}

type valMapListItem struct {
	Data map[string][]string
}

func (v *valMapListComp) Save() {}

func TestValidateDirectives_NestedForNonStructIntermediate(t *testing.T) {
	// item.Data is a map — resolveFieldType returns nil at the map boundary
	// Since resolved is nil, the inner loop var "opt" gets itemType nil
	comp := &valMapListComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
	html := `<div g-for="item in Items"><li g-for="opt in item.Data.Something"><span g-text="opt"></span></li></div>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- resolveFieldType: unknown field via nested g-for ---

func TestValidateDirectives_NestedForUnknownField(t *testing.T) {
	// item.Missing does not exist on valPtrItem — resolveFieldType returns nil
	html := `<div g-for="item in Items"><li g-for="opt in item.Missing.Something"><span g-text="opt"></span></li></div>`
	ci := newValPtrCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- collectLoopVars: prop alias to loop var item, index, and top-level field ---

func TestValidateDirectives_PropAliasToLoopVarItem(t *testing.T) {
	// :todo="todo" maps prop "todo" to the loop item variable
	html := `<li g-for="todo in Todos"><div g-props="item:todo"></div><span g-text="item.Text"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_PropAliasToLoopVarIndex(t *testing.T) {
	// :idx="i" maps prop "idx" to the loop index variable
	html := `<li g-for="todo, i in Todos"><div g-props="idx:i"></div><span g-text="idx"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_PropAliasToTopLevelField(t *testing.T) {
	// :count="Count" maps prop "count" to a top-level field
	html := `<li g-for="todo in Todos"><div g-props="count:Count"></div><span g-text="count"></span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- validateForExpr: dotted path through loop variable ---

func TestValidateDirectives_ForDottedPathLoopVar(t *testing.T) {
	// g-for with list field as dotted path through a loop variable
	html := `<div g-for="group in Groups"><li g-for="opt in group.Options"><span g-text="opt"></span></li></div>`
	comp := &valNestedComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- validateForExpr: invalid syntax ---

func TestValidateDirectives_ForInvalidSyntax(t *testing.T) {
	html := `<li g-for="invalid_no_in_keyword"><span>x</span></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for invalid g-for syntax")
	}
}

// --- g-attr and g-plugin directives (default case coverage) ---

func TestValidateDirectives_AttrDirective(t *testing.T) {
	html := `<a g-attr:href="Name">link</a>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_AttrDirectiveUnknown(t *testing.T) {
	html := `<a g-attr:href="Missing">link</a>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-attr")
	}
}

func TestValidateDirectives_HideDirective(t *testing.T) {
	html := `<div g-hide="Visible"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- No directives in HTML ---

func TestValidateDirectives_NoDirectives(t *testing.T) {
	html := `<div><span>plain html</span></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Mouse event child fallback ---

func TestValidateDirectives_ChildFallback_MouseEvent(t *testing.T) {
	// ChildAction exists as a method on the child component.
	// g-mousedown="ChildAction" should pass via child fallback, just like g-click does.
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	html := `<canvas g-mousedown="ChildAction"></canvas>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("child has ChildAction method — mousedown child fallback should pass: %v", err)
	}
}

// --- validateBindExpr: child component fallback ---

func TestValidateDirectives_BindChildFallback(t *testing.T) {
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	// "Local" exists on child but not parent
	html := `<input g-bind="Local" />`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (should fallback to child for bind): %v", err)
	}
}

// === Negative tests and edge cases ===

// --- g-drop.group child fallback ---

func TestValidateDirectives_DropGroupChildFallback(t *testing.T) {
	// ChildDrop exists as a method on the child component.
	// g-drop:canvas="ChildDrop" should pass via child fallback, just like g-drop="ChildDrop" does.
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childMethodComp{}),
	}
	html := `<div g-drop:canvas="ChildDrop"></div>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("child has ChildDrop method — g-drop:group child fallback should pass: %v", err)
	}
}

// --- Multi-arg method used as computed value ---

func TestValidateDirectives_MultiArgMethodAsComputed(t *testing.T) {
	// Toggle(int) has 1 param + receiver = NumIn 2 → not a computed value
	html := `<span g-text="Toggle"></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for multi-arg method used as field expression")
	}
}

// --- g-click with empty parens ---

func TestValidateDirectives_ClickEmptyParens(t *testing.T) {
	// Save() with explicit empty parens — ParseCallExpr returns name="Save", args=nil
	html := `<button g-click="Save()">X</button>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- g-keydown without key prefix (bare method name) ---

func TestValidateDirectives_KeydownBareMethod(t *testing.T) {
	// No colon → the whole part is the method name
	html := `<input g-keydown="Save" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_KeydownBareMethodUnknown(t *testing.T) {
	html := `<input g-keydown="Unknown" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown bare method in g-keydown")
	}
}

// --- Whitespace-only expressions ---

func TestValidateDirectives_WhitespaceOnlyExpr(t *testing.T) {
	// After TrimSpace, becomes "" → "empty directive expression"
	html := `<span g-text=" "></span>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for whitespace-only expression")
	}
}

// --- Multiple method args, one invalid ---

func TestValidateDirectives_MethodMultipleArgsOneInvalid(t *testing.T) {
	// Drop(from, to float64, position string) — first two args valid, third unknown
	html := `<li g-for="todo, i in Todos"><div g-drop="Drop(i, Count, unknown)"></div></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown third argument in method call")
	}
}

func TestValidateDirectives_MethodMultipleArgsAllValid(t *testing.T) {
	html := `<li g-for="todo, i in Todos"><div g-drop="Drop(i, Count, Name)"></div></li>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- g-bind with dotted method root ---

func TestValidateDirectives_BindDottedMethodRoot(t *testing.T) {
	// "Save.Something" — root is "Save", which is a method not a field
	html := `<input g-bind="Save.Something" />`
	ci := newValTestCI()
	err := ValidateDirectives(html, ci)
	if err == nil {
		t.Error("expected error for g-bind referencing a method with dotted path")
	}
	if err != nil && !strings.Contains(err.Error(), "method") {
		t.Errorf("expected error mentioning 'method', got: %v", err)
	}
}

// --- Loop var with non-struct item type + dotted access ---

func TestValidateDirectives_LoopVarNonStructDottedAccess(t *testing.T) {
	// group.Options is []string → "opt" has itemType string (non-struct)
	// "opt.Length" — root is loop var, lv.itemType is string (not struct)
	// Code returns nil (can't validate further)
	html := `<div g-for="group in Groups"><li g-for="opt in group.Options"><span g-text="opt.Length"></span></li></div>`
	comp := &valNestedComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
	// This passes because non-struct loop var dotted paths can't be validated
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (non-struct loop var dotted path should be tolerated): %v", err)
	}
}

// --- Loop var with nil itemType + dotted access ---

func TestValidateDirectives_LoopVarNilItemTypeDottedAccess(t *testing.T) {
	// g-for over a field that isn't a slice → itemType is nil
	// Use an unknown list so itemType ends up nil, but the loop var still exists
	// Actually, if the list field is unknown, validateForExpr will error first.
	// Instead, use a nested for through a map (which gives nil resolved type):
	comp := &valMapListComp{}
	v := reflect.ValueOf(comp)
	ci := &component.Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*component.Info),
		Registry: make(map[string]*component.Reg),
	}
	// item.Data is map → resolveFieldType returns nil → inner loop var "sub" has nil itemType
	// "sub.Anything" → root "sub" is loop var, lv.itemType is nil → return nil (tolerated)
	html := `<div g-for="item in Items"><div g-for="sub in item.Data.Things"><span g-text="sub.Anything"></span></div></div>`
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error (nil itemType dotted path should be tolerated): %v", err)
	}
}

// --- Multiple directives: one valid, one invalid ---

func TestValidateDirectives_MultipleDirectivesOneInvalid(t *testing.T) {
	html := `<div g-text="Name" g-show="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error when one of multiple directives references unknown field")
	}
}

// --- g-for error is not rescued by child fallback ---

func TestValidateDirectives_ForErrorNotRescuedByChild(t *testing.T) {
	// g-for errors return directly — no child fallback
	type childWithItems struct {
		Component struct{}
		Items     []string
	}
	ci := newValTestCI()
	ci.Registry["my-child"] = &component.Reg{
		Typ: reflect.TypeOf(childWithItems{}),
	}
	// "Items" doesn't exist on parent — g-for returns error immediately
	html := `<li g-for="item in Items"><span g-text="item"></span></li>`
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error: g-for doesn't fall through to child validation")
	}
}

// --- g-value valid field ---

func TestValidateDirectives_ValueField(t *testing.T) {
	html := `<input g-value="Name" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDirectives_ValueUnknown(t *testing.T) {
	html := `<input g-value="Missing" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-value")
	}
}

// --- g-if unknown field ---

func TestValidateDirectives_IfUnknown(t *testing.T) {
	html := `<div g-if="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-if")
	}
}

// --- g-checked unknown field ---

func TestValidateDirectives_CheckedUnknown(t *testing.T) {
	html := `<input g-checked="Missing" />`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-checked")
	}
}

// --- g-style unknown field ---

func TestValidateDirectives_StyleUnknown(t *testing.T) {
	html := `<div g-style:color="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-style")
	}
}

// --- g-class unknown field ---

func TestValidateDirectives_ClassUnknown(t *testing.T) {
	html := `<div g-class:active="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-class")
	}
}

// --- g-hide unknown field ---

func TestValidateDirectives_HideUnknown(t *testing.T) {
	html := `<div g-hide="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-hide")
	}
}

// --- g-wheel unknown method ---

func TestValidateDirectives_WheelUnknown(t *testing.T) {
	html := `<canvas g-wheel="Unknown"></canvas>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-wheel")
	}
}

// --- g-mousemove unknown method ---

func TestValidateDirectives_MousemoveUnknown(t *testing.T) {
	html := `<canvas g-mousemove="Unknown"></canvas>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-mousemove")
	}
}

// --- g-mouseup unknown method ---

func TestValidateDirectives_MouseupUnknown(t *testing.T) {
	html := `<canvas g-mouseup="Unknown"></canvas>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-mouseup")
	}
}

// --- g-drop unknown method with group ---

func TestValidateDirectives_DropGroupUnknownMethod(t *testing.T) {
	html := `<div g-drop:canvas="Unknown"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown method in g-drop:group")
	}
}

// --- g-draggable:group unknown field ---

func TestValidateDirectives_DraggableGroupUnknown(t *testing.T) {
	html := `<div g-draggable:palette="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-draggable:group")
	}
}

// --- g-dropzone unknown field ---

func TestValidateDirectives_DropzoneUnknown(t *testing.T) {
	html := `<div g-dropzone="Missing"></div>`
	ci := newValTestCI()
	if err := ValidateDirectives(html, ci); err == nil {
		t.Error("expected error for unknown field in g-dropzone")
	}
}
