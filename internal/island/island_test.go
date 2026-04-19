package island

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Test component types.

type testComp struct {
	Component struct{} // dummy — matches the field name check
	Name      string
	Count     int
	Items     []string
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
	Component struct{}
	Text      string `godom:"prop"`
	Index     int    `godom:"prop"`
	Local     string
}

func (c *testChild) DoSomething() {
	c.Local = "done"
}

// testNumeric has various numeric field types for SetField string-parsing tests.
type testNumeric struct {
	Component struct{}
	IntVal    int
	Int8Val   int8
	UintVal   uint
	Uint8Val  uint8
	FloatVal  float64
	Float32   float32
	BoolVal   bool
	unexported string //nolint
}

func newTestCI(comp interface{}) *Info {
	v := reflect.ValueOf(comp)
	return &Info{
		Value:    v,
		Typ:      v.Elem().Type(),
	}
}

func TestCallMethod_NoArgs(t *testing.T) {
	comp := &testComp{Count: 5}
	ci := newTestCI(comp)

	err := ci.CallMethod("Increment", nil)
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
	err := ci.CallMethod("SetName", []json.RawMessage{nameJSON})
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
	err := ci.CallMethod("Add", []json.RawMessage{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if comp.Count != 10 {
		t.Errorf("Count = %d, want 10", comp.Count)
	}
}

func TestCallMethod_NotFound(t *testing.T) {
	comp := &testComp{Count: 5}
	ci := newTestCI(comp)

	err := ci.CallMethod("NonExistent", nil)
	if err == nil {
		t.Error("expected error for non-existent method")
	}
	if comp.Count != 5 {
		t.Errorf("Count = %d, want 5 (should be unchanged)", comp.Count)
	}
}

func TestCallMethod_WrongArgCount(t *testing.T) {
	comp := &testComp{Count: 5}
	ci := newTestCI(comp)

	a, _ := json.Marshal(1)
	err := ci.CallMethod("Add", []json.RawMessage{a})
	if err == nil {
		t.Error("expected error for wrong arg count")
	}
	if comp.Count != 5 {
		t.Errorf("Count = %d, want 5 (should be unchanged)", comp.Count)
	}
}

func TestCallMethod_ExtraArgsIgnored(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	a, _ := json.Marshal(3)
	b, _ := json.Marshal(7)
	extra, _ := json.Marshal("above")
	err := ci.CallMethod("Add", []json.RawMessage{a, b, extra})
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

	err := ci.SetField("Name", json.RawMessage(`"Dave"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Name != "Dave" {
		t.Errorf("Name = %q, want Dave", comp.Name)
	}

	err = ci.SetField("Count", json.RawMessage(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Count != 42 {
		t.Errorf("Count = %d, want 42", comp.Count)
	}
}

func TestSetField_NotFound(t *testing.T) {
	comp := &testComp{Name: "original", Count: 5}
	ci := newTestCI(comp)

	err := ci.SetField("Missing", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for missing field")
	}
	// Verify existing fields weren't corrupted
	if comp.Name != "original" || comp.Count != 5 {
		t.Errorf("Name=%q Count=%d, want original/5 (unchanged)", comp.Name, comp.Count)
	}
}

func TestHasField(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	if !ci.HasField("Name") {
		t.Error("expected Name to be a valid field")
	}
	if !ci.HasField("Count") {
		t.Error("expected Count to be a valid field")
	}
	if ci.HasField("Component") {
		t.Error("Component should not count as a field")
	}
	if ci.HasField("missing") {
		t.Error("missing should not be a valid field")
	}
}

func TestHasMethod(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	if !ci.HasMethod("Increment") {
		t.Error("expected Increment to be a valid method")
	}
	if ci.HasMethod("missing") {
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
		name, args := ParseCallExpr(tt.expr)
		if name != tt.wantName {
			t.Errorf("ParseCallExpr(%q) name = %q, want %q", tt.expr, name, tt.wantName)
		}
		if len(args) != len(tt.wantArgs) {
			t.Errorf("ParseCallExpr(%q) args = %v, want %v", tt.expr, args, tt.wantArgs)
			continue
		}
		for i, a := range args {
			if a != tt.wantArgs[i] {
				t.Errorf("ParseCallExpr(%q) arg[%d] = %q, want %q", tt.expr, i, a, tt.wantArgs[i])
			}
		}
	}
}

// --- CallMethod bad arg unmarshal (continued) ---

// --- CallMethod bad arg unmarshal ---

func TestCallMethod_BadArgJSON(t *testing.T) {
	comp := &testComp{Count: 99}
	ci := newTestCI(comp)

	// Add expects int args, pass invalid JSON for the int param
	err := ci.CallMethod("Add", []json.RawMessage{[]byte(`"not-an-int"`), []byte(`1`)})
	if err == nil {
		t.Error("expected error for bad arg JSON")
	}
	// Verify state was not mutated by the failed call
	if comp.Count != 99 {
		t.Errorf("Count = %d, want 99 (should be unchanged after error)", comp.Count)
	}
}

// --- SetField: string→numeric parsing (HTML input paths) ---

func TestSetField_StringToInt(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	// HTML sends "42" as a JSON string, but field is int
	err := ci.SetField("IntVal", json.RawMessage(`"42"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.IntVal != 42 {
		t.Errorf("IntVal = %d, want 42", comp.IntVal)
	}
}

func TestSetField_StringToInt8(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	err := ci.SetField("Int8Val", json.RawMessage(`"7"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Int8Val != 7 {
		t.Errorf("Int8Val = %d, want 7", comp.Int8Val)
	}
}

func TestSetField_StringToUint(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	err := ci.SetField("UintVal", json.RawMessage(`"99"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.UintVal != 99 {
		t.Errorf("UintVal = %d, want 99", comp.UintVal)
	}
}

func TestSetField_StringToUint8(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	err := ci.SetField("Uint8Val", json.RawMessage(`"12"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Uint8Val != 12 {
		t.Errorf("Uint8Val = %d, want 12", comp.Uint8Val)
	}
}

func TestSetField_StringToFloat64(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	err := ci.SetField("FloatVal", json.RawMessage(`"3.14"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.FloatVal != 3.14 {
		t.Errorf("FloatVal = %f, want 3.14", comp.FloatVal)
	}
}

func TestSetField_StringToFloat32(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	err := ci.SetField("Float32", json.RawMessage(`"2.5"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Float32 != 2.5 {
		t.Errorf("Float32 = %f, want 2.5", comp.Float32)
	}
}

func TestSetField_EmptyStringToZeroValue(t *testing.T) {
	comp := &testNumeric{IntVal: 10, UintVal: 20, FloatVal: 3.0}
	ci := newTestCI(comp)

	// Empty string should set numeric fields to zero value
	for _, field := range []string{"IntVal", "UintVal", "FloatVal"} {
		err := ci.SetField(field, json.RawMessage(`""`))
		if err != nil {
			t.Fatalf("SetField(%q, empty) error: %v", field, err)
		}
	}
	if comp.IntVal != 0 {
		t.Errorf("IntVal = %d, want 0", comp.IntVal)
	}
	if comp.UintVal != 0 {
		t.Errorf("UintVal = %d, want 0", comp.UintVal)
	}
	if comp.FloatVal != 0 {
		t.Errorf("FloatVal = %f, want 0", comp.FloatVal)
	}
}

func TestSetField_StringToNonNumeric_Error(t *testing.T) {
	comp := &testNumeric{BoolVal: true}
	ci := newTestCI(comp)

	// BoolVal is bool — passing a string like "abc" can't be directly unmarshalled
	// and there's no special string→bool parsing, so it should error
	err := ci.SetField("BoolVal", json.RawMessage(`"abc"`))
	if err == nil {
		t.Error("expected error for unparseable string to bool field")
	}
	// Verify field was not mutated
	if comp.BoolVal != true {
		t.Error("BoolVal should remain true after failed SetField")
	}
}

func TestSetField_InvalidIntString_Error(t *testing.T) {
	comp := &testNumeric{IntVal: 42}
	ci := newTestCI(comp)

	err := ci.SetField("IntVal", json.RawMessage(`"not-a-number"`))
	if err == nil {
		t.Error("expected error for non-numeric string to int field")
	}
	if comp.IntVal != 42 {
		t.Errorf("IntVal = %d, want 42 (should be unchanged after error)", comp.IntVal)
	}
}

func TestSetField_InvalidUintString_Error(t *testing.T) {
	comp := &testNumeric{UintVal: 42}
	ci := newTestCI(comp)

	err := ci.SetField("UintVal", json.RawMessage(`"not-a-number"`))
	if err == nil {
		t.Error("expected error for non-numeric string to uint field")
	}
	if comp.UintVal != 42 {
		t.Errorf("UintVal = %d, want 42 (should be unchanged after error)", comp.UintVal)
	}
}

func TestSetField_InvalidFloatString_Error(t *testing.T) {
	comp := &testNumeric{FloatVal: 3.14}
	ci := newTestCI(comp)

	err := ci.SetField("FloatVal", json.RawMessage(`"not-a-number"`))
	if err == nil {
		t.Error("expected error for non-numeric string to float field")
	}
	if comp.FloatVal != 3.14 {
		t.Errorf("FloatVal = %f, want 3.14 (should be unchanged after error)", comp.FloatVal)
	}
}

func TestSetField_IntOverflow(t *testing.T) {
	// ParseInt(s, 10, 64) parses "200" fine, but int8 range is -128..127.
	// SetInt on an int8 field silently truncates. This test documents
	// that the current behavior is truncation (not an error).
	comp := &testNumeric{}
	ci := newTestCI(comp)

	err := ci.SetField("Int8Val", json.RawMessage(`"200"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 200 overflows int8 — Go's reflect.SetInt truncates to -56
	if comp.Int8Val != -56 {
		t.Errorf("Int8Val = %d, want -56 (200 truncated to int8)", comp.Int8Val)
	}
}

func TestSetField_NegativeUint(t *testing.T) {
	// Negative string to uint field — ParseUint should fail, error returned.
	comp := &testNumeric{UintVal: 5}
	ci := newTestCI(comp)

	err := ci.SetField("UintVal", json.RawMessage(`"-1"`))
	if err == nil {
		t.Error("expected error for negative string to uint field")
	}
	if comp.UintVal != 5 {
		t.Errorf("UintVal = %d, want 5 (should be unchanged after error)", comp.UintVal)
	}
}

// --- ParseCallExpr edge cases ---

func TestParseCallExpr_WithWhitespace(t *testing.T) {
	name, args := ParseCallExpr("  Toggle(a, b)  ")
	if name != "Toggle" {
		t.Errorf("name = %q, want Toggle", name)
	}
	if len(args) != 2 || args[0] != "a" || args[1] != "b" {
		t.Errorf("args = %v, want [a b]", args)
	}
}

func TestParseCallExpr_EmptyString(t *testing.T) {
	name, args := ParseCallExpr("")
	if name != "" {
		t.Errorf("name = %q, want empty", name)
	}
	if args != nil {
		t.Errorf("args = %v, want nil", args)
	}
}

func TestParseCallExpr_UnclosedParen(t *testing.T) {
	// "Method(a, b" — no closing paren. TrimSuffix is a no-op,
	// so "a, b" is kept as the args string. Documents this behavior.
	name, args := ParseCallExpr("Method(a, b")
	if name != "Method" {
		t.Errorf("name = %q, want Method", name)
	}
	// Current behavior: silently accepts, args are parsed as-is
	if len(args) != 2 || args[0] != "a" || args[1] != "b" {
		t.Errorf("args = %v, want [a b]", args)
	}
}

func TestHasField_Unexported(t *testing.T) {
	comp := &testNumeric{}
	ci := newTestCI(comp)

	// "unexported" exists on the struct but is not exported — should return false
	if ci.HasField("unexported") {
		t.Error("unexported field should not be reported as a valid field")
	}
}

func TestCallMethod_MalformedJSON(t *testing.T) {
	comp := &testComp{Count: 5}
	ci := newTestCI(comp)

	// Completely malformed JSON, not just wrong type
	err := ci.CallMethod("SetName", []json.RawMessage{[]byte(`{{{`)})
	if err == nil {
		t.Error("expected error for malformed JSON arg")
	}
	if comp.Count != 5 {
		t.Errorf("Count = %d, want 5 (should be unchanged)", comp.Count)
	}
	if comp.Name != "" {
		t.Errorf("Name = %q, want empty (should be unchanged)", comp.Name)
	}
}

func TestSetField_MalformedJSON(t *testing.T) {
	comp := &testComp{Name: "original"}
	ci := newTestCI(comp)

	err := ci.SetField("Name", json.RawMessage(`{{{`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
	if comp.Name != "original" {
		t.Errorf("Name = %q, want original (should be unchanged)", comp.Name)
	}
}

func TestSetField_UnexportedField(t *testing.T) {
	comp := &testNumeric{IntVal: 10}
	ci := newTestCI(comp)

	err := ci.SetField("unexported", json.RawMessage(`"value"`))
	if err == nil {
		t.Error("expected error for unexported field")
	}
	// Verify no side effects
	if comp.IntVal != 10 {
		t.Errorf("IntVal = %d, want 10 (should be unchanged)", comp.IntVal)
	}
}

// --- Map bracket access tests for SetField ---

type testMapComp struct {
	Component struct{}
	Inputs    map[string]any
	Counts    map[string]int
}

func TestSetField_MapBracket(t *testing.T) {
	comp := &testMapComp{Inputs: map[string]any{}}
	ci := newTestCI(comp)

	err := ci.SetField("Inputs[first]", json.RawMessage(`"hello"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Inputs["first"] != "hello" {
		t.Errorf("Inputs[first] = %v, want 'hello'", comp.Inputs["first"])
	}
}

func TestSetField_MapBracket_NilMap(t *testing.T) {
	comp := &testMapComp{} // Inputs is nil
	ci := newTestCI(comp)

	err := ci.SetField("Inputs[key]", json.RawMessage(`"value"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Inputs["key"] != "value" {
		t.Errorf("Inputs[key] = %v, want 'value'", comp.Inputs["key"])
	}
}

func TestSetField_MapBracket_TypedValue(t *testing.T) {
	comp := &testMapComp{Counts: map[string]int{}}
	ci := newTestCI(comp)

	err := ci.SetField("Counts[total]", json.RawMessage(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Counts["total"] != 42 {
		t.Errorf("Counts[total] = %v, want 42", comp.Counts["total"])
	}
}

func TestSetField_MapBracket_NotAMap(t *testing.T) {
	comp := &testComp{Name: "Alice"}
	ci := newTestCI(comp)

	err := ci.SetField("Name[key]", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for bracket access on non-map field")
	}
}

func TestSetField_MapBracket_MissingField(t *testing.T) {
	comp := &testMapComp{}
	ci := newTestCI(comp)

	err := ci.SetField("Missing[key]", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for missing field")
	}
}

// --- Dotted path tests for SetField ---

type testInnerComp struct {
	Value string
	Count int
}

type testNestedComp struct {
	Component struct{}
	Inner     testInnerComp
	PtrInner  *testInnerComp
}

func TestSetField_DottedPath(t *testing.T) {
	comp := &testNestedComp{}
	ci := newTestCI(comp)

	err := ci.SetField("Inner.Value", json.RawMessage(`"hello"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Inner.Value != "hello" {
		t.Errorf("Inner.Value = %q, want 'hello'", comp.Inner.Value)
	}
}

func TestSetField_DottedPath_Int(t *testing.T) {
	comp := &testNestedComp{}
	ci := newTestCI(comp)

	err := ci.SetField("Inner.Count", json.RawMessage(`7`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.Inner.Count != 7 {
		t.Errorf("Inner.Count = %d, want 7", comp.Inner.Count)
	}
}

func TestSetField_DottedPath_MissingField(t *testing.T) {
	comp := &testNestedComp{}
	ci := newTestCI(comp)

	err := ci.SetField("Inner.Missing", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for missing nested field")
	}
}

func TestSetField_DottedPath_Pointer(t *testing.T) {
	comp := &testNestedComp{PtrInner: &testInnerComp{}}
	ci := newTestCI(comp)

	err := ci.SetField("PtrInner.Value", json.RawMessage(`"through pointer"`))
	if err != nil {
		t.Fatal(err)
	}
	if comp.PtrInner.Value != "through pointer" {
		t.Errorf("PtrInner.Value = %q, want 'through pointer'", comp.PtrInner.Value)
	}
}

func TestSetField_DottedPath_NilPointer(t *testing.T) {
	comp := &testNestedComp{} // PtrInner is nil
	ci := newTestCI(comp)

	err := ci.SetField("PtrInner.Value", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for nil pointer in path")
	}
}

func TestSetField_DottedPath_NotAStruct(t *testing.T) {
	comp := &testComp{Name: "Alice"}
	ci := newTestCI(comp)

	err := ci.SetField("Name.Sub", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for dotted path on non-struct field")
	}
}

// --- AddMarkedFields / DrainMarkedFields tests ---

func TestAddMarkedFields_Single(t *testing.T) {
	ci := &Info{}
	ci.AddMarkedFields("Name")
	fields := ci.DrainMarkedFields()
	if len(fields) != 1 || fields[0] != "Name" {
		t.Errorf("DrainMarkedFields() = %v, want [Name]", fields)
	}
}

func TestAddMarkedFields_Multiple(t *testing.T) {
	ci := &Info{}
	ci.AddMarkedFields("Name", "Count")
	ci.AddMarkedFields("Items")
	fields := ci.DrainMarkedFields()
	if len(fields) != 3 || fields[0] != "Name" || fields[1] != "Count" || fields[2] != "Items" {
		t.Errorf("DrainMarkedFields() = %v, want [Name Count Items]", fields)
	}
}

func TestDrainMarkedFields_Empty(t *testing.T) {
	ci := &Info{}
	fields := ci.DrainMarkedFields()
	if fields != nil {
		t.Errorf("DrainMarkedFields() = %v, want nil", fields)
	}
}

func TestDrainMarkedFields_ClearsAfterDrain(t *testing.T) {
	ci := &Info{}
	ci.AddMarkedFields("Name")
	ci.DrainMarkedFields()
	fields := ci.DrainMarkedFields()
	if fields != nil {
		t.Errorf("second DrainMarkedFields() = %v, want nil", fields)
	}
}

// --- Map bracket access: bad JSON unmarshal for map value ---

func TestSetField_MapBracket_BadJSON(t *testing.T) {
	comp := &testMapComp{Counts: map[string]int{}}
	ci := newTestCI(comp)

	// "not-a-number" is a string, but Counts values are int — unmarshal should fail
	err := ci.SetField("Counts[key]", json.RawMessage(`"not-a-number"`))
	if err == nil {
		t.Error("expected error for bad JSON value in map bracket access")
	}
	// Verify map was not mutated
	if _, ok := comp.Counts["key"]; ok {
		t.Error("map should not have key after failed SetField")
	}
}

// --- ExecJS ---

func TestExecJS_CallsExecJSFn(t *testing.T) {
	ci := newTestCI(&testComp{})
	var gotID int32
	var gotExpr string
	ci.ExecJSFn = func(id int32, expr string) {
		gotID = id
		gotExpr = expr
	}

	ci.ExecJS("location.href", func(result []byte, err string) {})

	if gotID != 1 {
		t.Errorf("expected id=1, got %d", gotID)
	}
	if gotExpr != "location.href" {
		t.Errorf("expected expr='location.href', got %q", gotExpr)
	}
}

func TestExecJS_IncrementingIDs(t *testing.T) {
	ci := newTestCI(&testComp{})
	var ids []int32
	ci.ExecJSFn = func(id int32, expr string) {
		ids = append(ids, id)
	}

	ci.ExecJS("a", func([]byte, string) {})
	ci.ExecJS("b", func([]byte, string) {})
	ci.ExecJS("c", func([]byte, string) {})

	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Errorf("expected incrementing IDs [1 2 3], got %v", ids)
	}
}

func TestExecJS_RegistersCallback(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSFn = func(id int32, expr string) {}

	called := false
	ci.ExecJS("test", func(result []byte, err string) {
		called = true
	})

	// Callback should be in the map
	ci.JSCallbackMu.Lock()
	cb, ok := ci.JSCallbacks[1]
	ci.JSCallbackMu.Unlock()

	if !ok || cb == nil {
		t.Fatal("expected callback to be registered with id=1")
	}

	// Calling it should set our flag
	cb(nil, "")
	if !called {
		t.Error("expected callback to be callable")
	}
}

func TestExecJS_NilExecJSFn(t *testing.T) {
	ci := newTestCI(&testComp{})
	// ExecJSFn not set — should not panic

	called := false
	ci.ExecJS("test", func(result []byte, err string) {
		called = true
	})

	// Callback registered but not called (no broadcaster)
	if called {
		t.Error("callback should not be called when ExecJSFn is nil")
	}
}

func TestExecJS_NilCallback(t *testing.T) {
	ci := newTestCI(&testComp{})
	var gotID int32
	ci.ExecJSFn = func(id int32, expr string) {
		gotID = id
	}

	// nil callback should not panic
	ci.ExecJS("test", nil)

	if gotID != 1 {
		t.Errorf("expected ExecJSFn to be called even with nil callback, got id=%d", gotID)
	}
}

func TestExecJS_Disabled(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSDisabled = true

	var gotErr string
	ci.ExecJS("test", func(result []byte, err string) {
		gotErr = err
	})

	if gotErr != "ExecJS is disabled" {
		t.Errorf("expected disabled error, got %q", gotErr)
	}
}

func TestExecJS_DisabledNilCallback(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSDisabled = true

	// nil callback + disabled should not panic
	ci.ExecJS("test", nil)
}

func TestExecJS_DisabledDoesNotCallExecJSFn(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSDisabled = true
	called := false
	ci.ExecJSFn = func(id int32, expr string) {
		called = true
	}

	ci.ExecJS("test", func([]byte, string) {})

	if called {
		t.Error("ExecJSFn should not be called when disabled")
	}
}

// --- HandleJSResult ---

func TestHandleJSResult_DispatchesToCallback(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSFn = func(id int32, expr string) {}

	var gotResult []byte
	var gotErr string
	ci.ExecJS("test", func(result []byte, err string) {
		gotResult = result
		gotErr = err
	})

	ci.HandleJSResult(1, []byte(`"hello"`), "")

	if string(gotResult) != `"hello"` {
		t.Errorf("expected result '\"hello\"', got %q", string(gotResult))
	}
	if gotErr != "" {
		t.Errorf("expected empty error, got %q", gotErr)
	}
}

func TestHandleJSResult_WithError(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSFn = func(id int32, expr string) {}

	var gotErr string
	ci.ExecJS("test", func(result []byte, err string) {
		gotErr = err
	})

	ci.HandleJSResult(1, nil, "eval failed")

	if gotErr != "eval failed" {
		t.Errorf("expected 'eval failed', got %q", gotErr)
	}
}

func TestHandleJSResult_UnknownID(t *testing.T) {
	ci := newTestCI(&testComp{})

	// No callback registered for id=999 — should not panic
	ci.HandleJSResult(999, []byte(`"test"`), "")
}

func TestHandleJSResult_MultipleBrowserResponses(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSFn = func(id int32, expr string) {}

	count := 0
	ci.ExecJS("test", func(result []byte, err string) {
		count++
	})

	// Simulate 3 browsers responding to the same call
	ci.HandleJSResult(1, []byte(`"a"`), "")
	ci.HandleJSResult(1, []byte(`"b"`), "")
	ci.HandleJSResult(1, []byte(`"c"`), "")

	if count != 3 {
		t.Errorf("expected 3 callback invocations (one per browser), got %d", count)
	}
}

func TestHandleJSResult_NilCallback(t *testing.T) {
	ci := newTestCI(&testComp{})
	ci.ExecJSFn = func(id int32, expr string) {}

	ci.ExecJS("test", nil)

	// nil callback registered — HandleJSResult should not panic
	ci.HandleJSResult(1, []byte(`"test"`), "")
}

// --- HasMethod ---

func TestHasMethod_Exists(t *testing.T) {
	ci := newTestCI(&testComp{})
	if !ci.HasMethod("Increment") {
		t.Error("expected HasMethod('Increment') = true")
	}
}

func TestHasMethod_NotExists(t *testing.T) {
	ci := newTestCI(&testComp{})
	if ci.HasMethod("NonExistent") {
		t.Error("expected HasMethod('NonExistent') = false")
	}
}

func TestHasMethod_Unexported(t *testing.T) {
	ci := newTestCI(&testComp{})
	// unexported methods should not be found
	if ci.HasMethod("increment") {
		t.Error("expected HasMethod('increment') = false for unexported")
	}
}
