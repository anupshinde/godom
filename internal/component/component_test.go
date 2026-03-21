package component

// [COVERAGE GAP] SnapshotState (line 122-128): The log.Fatalf branch is only
// reachable when GetState() returns a json.Marshal error (e.g. a struct with a
// channel field). Since log.Fatalf calls os.Exit(1), testing it would kill the
// test process. Covering this would require subprocess-based testing which adds
// complexity for minimal correctness value — the fatal path is a deliberate
// panic-on-impossible-error guard.

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

// testWithSub has a struct field for JSON round-trip testing in SetProps.
type testWithSub struct {
	Component struct{}
	Sub       testSub `godom:"prop"`
}

type testSub struct {
	X int
	Y string
}

func newTestCI(comp interface{}) *Info {
	v := reflect.ValueOf(comp)
	return &Info{
		Value:    v,
		Typ:      v.Elem().Type(),
		Children: make(map[string][]*Info),
	}
}

func TestGetState(t *testing.T) {
	comp := &testComp{Name: "Alice", Count: 3, Items: []string{"a", "b"}}
	ci := newTestCI(comp)

	data, err := ci.GetState()
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

	data, _ := ci.GetState()
	var state map[string]interface{}
	json.Unmarshal(data, &state)

	if _, ok := state["Component"]; ok {
		t.Error("getState should exclude the embedded Component field")
	}
}

func TestChangedFields(t *testing.T) {
	comp := &testComp{Name: "Alice", Count: 1}
	ci := newTestCI(comp)

	old := ci.SnapshotState()
	comp.Name = "Bob"
	comp.Count = 2
	newer := ci.SnapshotState()

	changed := ci.ChangedFields(old, newer)
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

	old := ci.SnapshotState()
	newer := ci.SnapshotState()

	changed := ci.ChangedFields(old, newer)
	if len(changed) != 0 {
		t.Errorf("expected no changes, got %v", changed)
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

func TestPropFieldNames(t *testing.T) {
	typ := reflect.TypeOf(testChild{})
	props := PropFieldNames(typ)

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
	ci.PropFields = PropFieldNames(ci.Typ)

	ci.SetProps(map[string]interface{}{
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
	ci.PropFields = PropFieldNames(ci.Typ)

	// float64 → int (JSON numbers come as float64)
	ci.SetProps(map[string]interface{}{
		"Index": float64(7),
	})

	if child.Index != 7 {
		t.Errorf("Index = %d, want 7", child.Index)
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

// --- AllExportedFieldNames ---

func TestAllExportedFieldNames(t *testing.T) {
	names := AllExportedFieldNames(reflect.TypeOf(testComp{}))
	want := map[string]bool{"Name": true, "Count": true, "Items": true}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected field %q", n)
		}
	}
}

func TestAllExportedFieldNames_ExcludesComponent(t *testing.T) {
	names := AllExportedFieldNames(reflect.TypeOf(testComp{}))
	for _, n := range names {
		if n == "Component" {
			t.Error("should not include Component")
		}
	}
}

// --- GetState with unexported fields ---

func TestGetState_SkipsUnexportedFields(t *testing.T) {
	comp := &testNumeric{IntVal: 5}
	ci := newTestCI(comp)

	data, err := ci.GetState()
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state["unexported"]; ok {
		t.Error("unexported field should not appear in state")
	}
}

// --- ChangedFields edge cases ---

func TestChangedFields_InvalidOldJSON(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	changed := ci.ChangedFields([]byte("not json"), []byte(`{"Name":"x"}`))
	if changed != nil {
		t.Errorf("expected nil for invalid old JSON, got %v", changed)
	}
}

func TestChangedFields_InvalidNewJSON(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	changed := ci.ChangedFields([]byte(`{"Name":"x"}`), []byte("not json"))
	if changed != nil {
		t.Errorf("expected nil for invalid new JSON, got %v", changed)
	}
}

func TestChangedFields_FieldRemoved(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	old := []byte(`{"Name":"Alice","Count":1}`)
	newer := []byte(`{"Name":"Alice"}`)
	changed := ci.ChangedFields(old, newer)

	has := map[string]bool{}
	for _, f := range changed {
		has[f] = true
	}
	if !has["Count"] {
		t.Errorf("expected Count to be in changed (removed), got %v", changed)
	}
}

func TestChangedFields_FieldAdded(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	old := []byte(`{"Name":"Alice"}`)
	newer := []byte(`{"Name":"Alice","Count":1}`)
	changed := ci.ChangedFields(old, newer)

	has := map[string]bool{}
	for _, f := range changed {
		has[f] = true
	}
	if !has["Count"] {
		t.Errorf("expected Count to be in changed (added), got %v", changed)
	}
}

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

// --- SetProps edge cases ---

func TestSetProps_SkipsNonPropFields(t *testing.T) {
	child := &testChild{Local: "original"}
	ci := newTestCI(child)
	ci.PropFields = PropFieldNames(ci.Typ)

	ci.SetProps(map[string]interface{}{
		"Local": "overwritten",
	})

	if child.Local != "original" {
		t.Errorf("Local = %q, want original (should be skipped)", child.Local)
	}
}

func TestSetProps_SkipsInvalidFieldName(t *testing.T) {
	child := &testChild{Text: "original", Index: 1, Local: "kept"}
	ci := newTestCI(child)
	ci.PropFields = PropFieldNames(ci.Typ)

	// Should not panic and should not corrupt existing fields
	ci.SetProps(map[string]interface{}{
		"NonExistent": "value",
	})

	if child.Text != "original" || child.Index != 1 || child.Local != "kept" {
		t.Errorf("existing fields were corrupted: Text=%q Index=%d Local=%q",
			child.Text, child.Index, child.Local)
	}
}

func TestSetProps_NilPropFieldsAllowsAll(t *testing.T) {
	child := &testChild{}
	ci := newTestCI(child)
	// PropFields is nil — all fields should be settable
	ci.PropFields = nil

	ci.SetProps(map[string]interface{}{
		"Text":  "hello",
		"Local": "world",
	})

	if child.Text != "hello" {
		t.Errorf("Text = %q, want hello", child.Text)
	}
	if child.Local != "world" {
		t.Errorf("Local = %q, want world", child.Local)
	}
}

func TestSetProps_JSONRoundTrip(t *testing.T) {
	comp := &testWithSub{}
	ci := newTestCI(comp)
	ci.PropFields = map[string]bool{"Sub": true}

	// Pass a map — not directly assignable to testSub, requires JSON round-trip
	ci.SetProps(map[string]interface{}{
		"Sub": map[string]interface{}{"X": 10, "Y": "hello"},
	})

	if comp.Sub.X != 10 {
		t.Errorf("Sub.X = %d, want 10", comp.Sub.X)
	}
	if comp.Sub.Y != "hello" {
		t.Errorf("Sub.Y = %q, want hello", comp.Sub.Y)
	}
}

func TestSetProps_SkipsUnsettableField(t *testing.T) {
	// With nil PropFields (allow all), try to set an unexported field
	// and a completely non-existent field — both should be silently skipped.
	comp := &testNumeric{IntVal: 77}
	ci := newTestCI(comp)
	ci.PropFields = nil

	ci.SetProps(map[string]interface{}{
		"unexported":  "value",
		"NonExistent": "value",
	})

	// Verify existing fields weren't corrupted
	if comp.IntVal != 77 {
		t.Errorf("IntVal = %d, want 77 (should be unchanged)", comp.IntVal)
	}
}

func TestSetProps_JSONMarshalFailure(t *testing.T) {
	// Pass a value that can't be JSON-marshalled (channel) and is not
	// assignable/convertible to the target field type — exercises the
	// json.Marshal error path in SetProps.
	comp := &testWithSub{}
	ci := newTestCI(comp)
	ci.PropFields = map[string]bool{"Sub": true}

	ch := make(chan int)
	ci.SetProps(map[string]interface{}{
		"Sub": ch,
	})

	// Sub should remain zero-valued since the marshal fails
	if comp.Sub.X != 0 || comp.Sub.Y != "" {
		t.Errorf("Sub = %+v, want zero value", comp.Sub)
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

// --- Additional negative tests ---

func TestPropFieldNames_NoTags(t *testing.T) {
	// A struct with no godom:"prop" tags should return an empty (non-nil) map.
	props := PropFieldNames(reflect.TypeOf(testComp{}))
	if len(props) != 0 {
		t.Errorf("expected empty props map, got %v", props)
	}
}

func TestAllExportedFieldNames_OnlyUnexported(t *testing.T) {
	// A struct with only Component and unexported fields should return empty.
	type onlyPrivate struct {
		Component struct{}
		hidden    string //nolint
	}
	names := AllExportedFieldNames(reflect.TypeOf(onlyPrivate{}))
	if len(names) != 0 {
		t.Errorf("expected no exported fields, got %v", names)
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

func TestChangedFields_EmptyObjects(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	changed := ci.ChangedFields([]byte(`{}`), []byte(`{}`))
	if len(changed) != 0 {
		t.Errorf("expected no changes between empty objects, got %v", changed)
	}
}

func TestChangedFields_OnlyUnchangedFields(t *testing.T) {
	comp := &testComp{}
	ci := newTestCI(comp)

	// Same values in both — no changes expected
	old := []byte(`{"Name":"Alice","Count":1,"Items":["a"]}`)
	newer := []byte(`{"Name":"Alice","Count":1,"Items":["a"]}`)
	changed := ci.ChangedFields(old, newer)
	if len(changed) != 0 {
		t.Errorf("expected no changes, got %v", changed)
	}
}

func TestGetState_ExcludesOnlyComponentField(t *testing.T) {
	// Verify GetState returns exactly the expected set of fields —
	// not just that Component is absent, but that nothing extra appears.
	comp := &testComp{Name: "Alice", Count: 3, Items: []string{"a"}}
	ci := newTestCI(comp)

	data, err := ci.GetState()
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{"Name": true, "Count": true, "Items": true}
	if len(state) != len(expected) {
		t.Errorf("got %d fields %v, want exactly %v", len(state), state, expected)
	}
	for key := range state {
		if !expected[key] {
			t.Errorf("unexpected field %q in state", key)
		}
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

func TestSetField_DottedPath_NotAStruct(t *testing.T) {
	comp := &testComp{Name: "Alice"}
	ci := newTestCI(comp)

	err := ci.SetField("Name.Sub", json.RawMessage(`"x"`))
	if err == nil {
		t.Error("expected error for dotted path on non-struct field")
	}
}
