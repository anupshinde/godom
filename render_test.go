package godom

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
)

// --- Expression resolution tests ---

func TestResolveExprVal_Literals(t *testing.T) {
	tests := []struct {
		expr string
		want interface{}
	}{
		{"true", true},
		{"false", false},
		{"42", 42},
		{"0", 0},
		{`"hello"`, "hello"},
		{`'world'`, "world"},
	}

	for _, tt := range tests {
		got := resolveExprVal(tt.expr, nil, nil)
		if got != tt.want {
			t.Errorf("resolveExprVal(%q) = %v (%T), want %v (%T)", tt.expr, got, got, tt.want, tt.want)
		}
	}
}

func TestResolveExprVal_FieldAccess(t *testing.T) {
	state := map[string]interface{}{
		"Name":  "Alice",
		"Count": 5,
	}

	if got := resolveExprVal("Name", state, nil); got != "Alice" {
		t.Errorf("got %v, want Alice", got)
	}
	if got := resolveExprVal("Count", state, nil); got != 5 {
		t.Errorf("got %v, want 5", got)
	}
	if got := resolveExprVal("Missing", state, nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResolveExprVal_DottedPath(t *testing.T) {
	state := map[string]interface{}{
		"User": map[string]interface{}{
			"Name": "Bob",
			"Address": map[string]interface{}{
				"City": "NYC",
			},
		},
	}

	if got := resolveExprVal("User.Name", state, nil); got != "Bob" {
		t.Errorf("got %v, want Bob", got)
	}
	if got := resolveExprVal("User.Address.City", state, nil); got != "NYC" {
		t.Errorf("got %v, want NYC", got)
	}
	if got := resolveExprVal("User.Missing", state, nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResolveExprVal_ContextPrecedence(t *testing.T) {
	state := map[string]interface{}{"item": "from-state"}
	ctx := map[string]interface{}{"item": "from-ctx"}

	// Context should win over state
	if got := resolveExprVal("item", state, ctx); got != "from-ctx" {
		t.Errorf("got %v, want from-ctx", got)
	}

	// State used when not in context
	if got := resolveExprVal("item", state, nil); got != "from-state" {
		t.Errorf("got %v, want from-state", got)
	}
}

func TestResolveExprVal_ContextDottedPath(t *testing.T) {
	state := map[string]interface{}{}
	ctx := map[string]interface{}{
		"todo": map[string]interface{}{
			"Text": "Buy milk",
			"Done": true,
		},
	}

	if got := resolveExprVal("todo.Text", state, ctx); got != "Buy milk" {
		t.Errorf("got %v, want Buy milk", got)
	}
	if got := resolveExprVal("todo.Done", state, ctx); got != true {
		t.Errorf("got %v, want true", got)
	}
}

func TestResolveExprVal_Whitespace(t *testing.T) {
	state := map[string]interface{}{"Name": "Alice"}

	if got := resolveExprVal("  Name  ", state, nil); got != "Alice" {
		t.Errorf("got %v, want Alice", got)
	}
}

func TestResolvePath(t *testing.T) {
	val := map[string]interface{}{
		"A": map[string]interface{}{
			"B": "deep",
		},
	}

	if got := resolvePath(val, []string{"A", "B"}); got != "deep" {
		t.Errorf("got %v, want deep", got)
	}

	// Empty path returns value as-is
	if got := resolvePath("hello", nil); got != "hello" {
		t.Errorf("got %v, want hello", got)
	}

	// Non-map returns nil
	if got := resolvePath("hello", []string{"x"}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestToBool(t *testing.T) {
	tests := []struct {
		val  interface{}
		want bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{float64(0), false},
		{float64(1), true},
		{float64(-1), true},
		{0, false},
		{1, true},
		{"", false},
		{"hello", true},
		{[]int{1}, true}, // non-nil slice → true
	}

	for _, tt := range tests {
		if got := toBool(tt.val); got != tt.want {
			t.Errorf("toBool(%v) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

// --- Command generation tests ---

func TestSingleCmd_Text(t *testing.T) {
	b := binding{GID: "g1", Dir: "text", Expr: "Name"}
	state := map[string]interface{}{"Name": "Alice"}

	cmd := singleCmd(b, state, nil)
	if cmd.Op != "text" || cmd.Id != "g1" || cmd.GetStrVal() != "Alice" {
		t.Errorf("got op=%q id=%q strVal=%q", cmd.Op, cmd.Id, cmd.GetStrVal())
	}
}

func TestSingleCmd_Value(t *testing.T) {
	b := binding{GID: "g2", Dir: "bind", Expr: "Email"}
	state := map[string]interface{}{"Email": "a@b.com"}

	cmd := singleCmd(b, state, nil)
	if cmd.Op != "value" || cmd.GetStrVal() != "a@b.com" {
		t.Errorf("got op=%q strVal=%q", cmd.Op, cmd.GetStrVal())
	}
}

func TestSingleCmd_Checked(t *testing.T) {
	b := binding{GID: "g3", Dir: "checked", Expr: "Done"}
	state := map[string]interface{}{"Done": true}

	cmd := singleCmd(b, state, nil)
	if cmd.Op != "checked" || cmd.GetBoolVal() != true {
		t.Errorf("got op=%q boolVal=%v", cmd.Op, cmd.GetBoolVal())
	}
}

func TestSingleCmd_Display(t *testing.T) {
	bShow := binding{GID: "g4", Dir: "show", Expr: "Visible"}
	bIf := binding{GID: "g5", Dir: "if", Expr: "Visible"}
	state := map[string]interface{}{"Visible": false}

	cmdShow := singleCmd(bShow, state, nil)
	if cmdShow.Op != "display" || cmdShow.GetBoolVal() != false {
		t.Errorf("show: got op=%q boolVal=%v", cmdShow.Op, cmdShow.GetBoolVal())
	}
	// Verify it's the boolVal oneof that's set (not just default)
	if _, ok := cmdShow.Val.(*Command_BoolVal); !ok {
		t.Errorf("show: expected BoolVal oneof, got %T", cmdShow.Val)
	}

	cmdIf := singleCmd(bIf, state, nil)
	if cmdIf.Op != "display" {
		t.Errorf("if: got op=%q", cmdIf.Op)
	}
	if _, ok := cmdIf.Val.(*Command_BoolVal); !ok {
		t.Errorf("if: expected BoolVal oneof, got %T", cmdIf.Val)
	}
}

func TestSingleCmd_Class(t *testing.T) {
	b := binding{GID: "g6", Dir: "class:active", Expr: "IsActive"}
	state := map[string]interface{}{"IsActive": true}

	cmd := singleCmd(b, state, nil)
	if cmd.Op != "class" || cmd.Name != "active" || cmd.GetBoolVal() != true {
		t.Errorf("got op=%q name=%q boolVal=%v", cmd.Op, cmd.Name, cmd.GetBoolVal())
	}
}

func TestSingleCmd_Style(t *testing.T) {
	b := binding{GID: "g8", Dir: "style:background-color", Expr: "BgColor"}
	state := map[string]interface{}{"BgColor": "red"}

	cmd := singleCmd(b, state, nil)
	if cmd.Op != "style" || cmd.Name != "background-color" || cmd.GetStrVal() != "red" {
		t.Errorf("got op=%q name=%q strVal=%q", cmd.Op, cmd.Name, cmd.GetStrVal())
	}
}

func TestSingleCmd_WithContext(t *testing.T) {
	b := binding{GID: "g7", Dir: "text", Expr: "todo.Text"}
	state := map[string]interface{}{}
	ctx := map[string]interface{}{
		"todo": map[string]interface{}{"Text": "Buy milk"},
	}

	cmd := singleCmd(b, state, ctx)
	if cmd.GetStrVal() != "Buy milk" {
		t.Errorf("got strVal=%q, want Buy milk", cmd.GetStrVal())
	}
}

func TestComputeBindingCmds(t *testing.T) {
	bindings := []binding{
		{GID: "g1", Dir: "text", Expr: "Name"},
		{GID: "g2", Dir: "text", Expr: "Age"},
	}
	state := map[string]interface{}{"Name": "Bob", "Age": float64(30)}

	cmds := computeBindingCmds(bindings, state, nil)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[0].GetStrVal() != "Bob" {
		t.Errorf("first cmd strVal = %q, want Bob", cmds[0].GetStrVal())
	}
}

func TestSingleEventCmd_Click(t *testing.T) {
	e := eventBinding{GID: "g1", Event: "click", Method: "Save", Args: nil}
	state := map[string]interface{}{}

	evt := singleEventCmd(e, state, nil)
	if evt.Id != "g1" || evt.On != "click" {
		t.Errorf("got id=%q on=%q", evt.Id, evt.On)
	}

	var wsMsg WSMessage
	if err := proto.Unmarshal(evt.Msg, &wsMsg); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}
	if wsMsg.Method != "Save" {
		t.Errorf("msg method = %q, want Save", wsMsg.Method)
	}
}

func TestSingleEventCmd_WithArgs(t *testing.T) {
	e := eventBinding{GID: "g2", Event: "click", Method: "Toggle", Args: []string{"i"}}
	state := map[string]interface{}{}
	ctx := map[string]interface{}{"i": 3}

	evt := singleEventCmd(e, state, ctx)

	var wsMsg WSMessage
	if err := proto.Unmarshal(evt.Msg, &wsMsg); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}
	if len(wsMsg.Args) != 1 || string(wsMsg.Args[0]) != "3" {
		t.Errorf("msg args = %v, want [3]", wsMsg.Args)
	}
}

func TestSingleEventCmd_Bind(t *testing.T) {
	e := eventBinding{GID: "g3", Event: "input", Method: "__bind", Args: []string{"Email"}}

	evt := singleEventCmd(e, nil, nil)
	if evt.On != "input" {
		t.Errorf("on = %q, want input", evt.On)
	}

	var wsMsg WSMessage
	if err := proto.Unmarshal(evt.Msg, &wsMsg); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}
	if wsMsg.Type != "bind" || wsMsg.Field != "Email" {
		t.Errorf("msg type=%q field=%q, want bind/Email", wsMsg.Type, wsMsg.Field)
	}
}

func TestSingleEventCmd_Keydown(t *testing.T) {
	e := eventBinding{GID: "g4", Event: "keydown", Key: "Enter", Method: "Submit"}

	evt := singleEventCmd(e, nil, nil)
	if evt.Key != "Enter" {
		t.Errorf("key = %q, want Enter", evt.Key)
	}
}

func TestSingleScopedEventCmd(t *testing.T) {
	e := eventBinding{GID: "g5", Event: "click", Method: "Toggle"}

	evt := singleScopedEventCmd(e, nil, nil, "g3", 2)

	var wsMsg WSMessage
	if err := proto.Unmarshal(evt.Msg, &wsMsg); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}
	if wsMsg.Scope != "g3:2" {
		t.Errorf("scope = %q, want g3:2", wsMsg.Scope)
	}
}

func TestBuildItemCtx(t *testing.T) {
	ft := &forTemplate{
		ItemVar:  "todo",
		IndexVar: "i",
		Props:    map[string]string{"text": "todo"},
	}
	state := map[string]interface{}{}
	item := map[string]interface{}{"Text": "hello"}

	ctx := buildItemCtx(ft, state, item, 5)

	if !reflect.DeepEqual(ctx["todo"], item) {
		t.Error("item var not set")
	}
	if ctx["i"] != 5 {
		t.Error("index var not set")
	}
	if !reflect.DeepEqual(ctx["text"], item) {
		t.Errorf("prop alias not resolved: %v", ctx["text"])
	}
}

func TestBuildItemCtx_NoIndex(t *testing.T) {
	ft := &forTemplate{
		ItemVar:  "item",
		IndexVar: "",
	}

	ctx := buildItemCtx(ft, nil, "val", 0)
	if _, ok := ctx[""]; ok {
		t.Error("empty index var should not be in context")
	}
	if ctx["item"] != "val" {
		t.Error("item var not set")
	}
}

type cmdTestComp struct {
	Component
	Name  string
	Items []cmdTestItem
}

type cmdTestItem struct {
	Text string
	Done bool
}

func TestComputeInitMessage(t *testing.T) {
	comp := &cmdTestComp{Name: "Test", Items: []cmdTestItem{{Text: "A"}, {Text: "B"}}}
	ci := newTestCI(comp)

	html := `<html><body>
		<span g-text="Name"></span>
		<li g-for="item in Items"><span g-text="item.Text"></span></li>
	</body></html>`

	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb

	msg, err := computeInitMessage(pb, ci)
	if err != nil {
		t.Fatal(err)
	}

	if msg.Type != "init" {
		t.Errorf("type = %v, want init", msg.Type)
	}

	if len(msg.Commands) < 1 {
		t.Error("expected at least 1 command")
	}

	// Should have seeded prevLists
	if ci.prevLists == nil {
		t.Error("prevLists should be initialized")
	}
}

func TestComputeUpdateMessage(t *testing.T) {
	comp := &cmdTestComp{Name: "Old"}
	ci := newTestCI(comp)

	html := `<html><body><span g-text="Name"></span></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb

	comp.Name = "New"
	msg := computeUpdateMessage(pb, ci, []string{"Name"})

	if msg == nil {
		t.Fatal("expected update message")
	}
	if msg.Type != "update" {
		t.Errorf("type = %v, want update", msg.Type)
	}

	if len(msg.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(msg.Commands))
	}
	if msg.Commands[0].GetStrVal() != "New" {
		t.Errorf("strVal = %q, want New", msg.Commands[0].GetStrVal())
	}
}

func TestComputeUpdateMessage_NoChanges(t *testing.T) {
	comp := &cmdTestComp{Name: "Same"}
	ci := newTestCI(comp)

	html := `<html><body><span g-text="Name"></span></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb

	msg := computeUpdateMessage(pb, ci, []string{"Missing"})
	if msg != nil {
		t.Error("expected nil for no matching changes")
	}
}

func TestComputeListDiff_Append(t *testing.T) {
	comp := &cmdTestComp{Items: []cmdTestItem{{Text: "A"}}}
	ci := newTestCI(comp)

	html := `<html><body><li g-for="item in Items"><span g-text="item.Text"></span></li></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb
	ft := pb.ForLoops[0]
	state := stateMap(ci)

	// Seed prevLists with one item (keyed by g-for GID)
	ci.prevLists = map[string][]string{ft.GID: {`{"Done":false,"Text":"A"}`}}

	// Add a second item
	comp.Items = append(comp.Items, cmdTestItem{Text: "B"})
	state = stateMap(ci)

	cmds, _ := computeListDiff(ft, state, ci)

	hasAppend := false
	for _, c := range cmds {
		if c.Op == "list-append" {
			hasAppend = true
			if len(c.Items) != 1 {
				t.Errorf("expected 1 appended item, got %d", len(c.Items))
			}
		}
	}
	if !hasAppend {
		t.Error("expected list-append command")
	}
}

func TestComputeListDiff_Truncate(t *testing.T) {
	comp := &cmdTestComp{Items: []cmdTestItem{{Text: "A"}, {Text: "B"}}}
	ci := newTestCI(comp)

	html := `<html><body><li g-for="item in Items"><span g-text="item.Text"></span></li></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb
	ft := pb.ForLoops[0]

	// Seed prevLists with two items (keyed by g-for GID)
	ci.prevLists = map[string][]string{
		ft.GID: {`{"Done":false,"Text":"A"}`, `{"Done":false,"Text":"B"}`},
	}

	// Remove the second item
	comp.Items = comp.Items[:1]
	state := stateMap(ci)

	cmds, _ := computeListDiff(ft, state, ci)

	hasTruncate := false
	for _, c := range cmds {
		if c.Op == "list-truncate" {
			hasTruncate = true
			if c.GetNumVal() != 1 {
				t.Errorf("expected truncate count 1, got %v", c.GetNumVal())
			}
		}
	}
	if !hasTruncate {
		t.Error("expected list-truncate command")
	}
}

func TestComputeListDiff_ItemChanged(t *testing.T) {
	comp := &cmdTestComp{Items: []cmdTestItem{{Text: "A", Done: false}}}
	ci := newTestCI(comp)

	html := `<html><body><li g-for="item in Items"><span g-text="item.Text"></span></li></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb
	ft := pb.ForLoops[0]

	// Seed prevLists (keyed by g-for GID)
	ci.prevLists = map[string][]string{ft.GID: {`{"Done":false,"Text":"A"}`}}

	// Change the item
	comp.Items[0].Done = true
	state := stateMap(ci)

	cmds, _ := computeListDiff(ft, state, ci)

	if len(cmds) == 0 {
		t.Error("expected update commands for changed item")
	}
	// Should not be a full list re-render
	for _, c := range cmds {
		if c.Op == "list" {
			t.Error("expected surgical update, not full list re-render")
		}
	}
}

func TestComputeListDiff_EmptyToEmpty(t *testing.T) {
	comp := &cmdTestComp{Items: nil}
	ci := newTestCI(comp)

	html := `<html><body><li g-for="item in Items"><span g-text="item.Text"></span></li></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb
	ft := pb.ForLoops[0]

	ci.prevLists = map[string][]string{ft.GID: {}}
	state := stateMap(ci)

	cmds, evts := computeListDiff(ft, state, ci)
	if cmds != nil {
		t.Error("expected nil cmds for empty → empty")
	}
	if evts != nil {
		t.Error("expected nil evts for empty → empty")
	}
}

func TestEnsureChildInstances(t *testing.T) {
	parentComp := &testComp{}
	parent := newTestCI(parentComp)
	parent.registry = map[string]*componentReg{
		"test-child": {typ: reflect.TypeOf(testChild{})},
	}
	parent.children = make(map[string][]*componentInfo)

	ft := &forTemplate{GID: "g1", ComponentTag: "test-child"}

	ensureChildInstances(ft, parent, 3)
	if len(parent.children["g1"]) != 3 {
		t.Errorf("expected 3 children, got %d", len(parent.children["g1"]))
	}

	// Trim down
	ensureChildInstances(ft, parent, 1)
	if len(parent.children["g1"]) != 1 {
		t.Errorf("expected 1 child after trim, got %d", len(parent.children["g1"]))
	}

	// Grow back
	ensureChildInstances(ft, parent, 2)
	if len(parent.children["g1"]) != 2 {
		t.Errorf("expected 2 children, got %d", len(parent.children["g1"]))
	}
}

func TestEnsureChildInstances_SetsComponentCI(t *testing.T) {
	parentComp := &testComp{}
	parent := newTestCI(parentComp)
	parent.registry = map[string]*componentReg{
		"test-child": {typ: reflect.TypeOf(testChild{})},
	}
	parent.children = make(map[string][]*componentInfo)

	ft := &forTemplate{GID: "g1", ComponentTag: "test-child"}
	ensureChildInstances(ft, parent, 1)

	child := parent.children["g1"][0]
	// The child's Component.ci should be set for Emit to work
	compField := child.value.Elem().FieldByName("Component")
	comp := compField.Interface().(Component)
	if comp.ci != child {
		t.Error("child Component.ci should point to the child componentInfo")
	}
	if child.parent != parent {
		t.Error("child.parent should point to parent")
	}
}

func TestSetChildProps(t *testing.T) {
	child := &testChild{}
	childCI := newTestCI(child)
	childCI.propFields = propFieldNames(childCI.typ)

	props := map[string]string{"Text": "todo", "Index": "i"}
	parentState := map[string]interface{}{}
	parentCtx := map[string]interface{}{
		"todo": "Buy milk",
		"i":    float64(2),
	}

	setChildProps(childCI, props, parentState, parentCtx)

	if child.Text != "Buy milk" {
		t.Errorf("Text = %q, want Buy milk", child.Text)
	}
	if child.Index != 2 {
		t.Errorf("Index = %d, want 2", child.Index)
	}
}

func TestSingleCmd_Draggable(t *testing.T) {
	b := binding{GID: "g9", Dir: "draggable", Expr: "i"}
	ctx := map[string]interface{}{"i": 3}

	cmd := singleCmd(b, nil, ctx)
	if cmd.Op != "draggable" || cmd.Id != "g9" || cmd.GetStrVal() != "3" {
		t.Errorf("got op=%q id=%q strVal=%q", cmd.Op, cmd.Id, cmd.GetStrVal())
	}
	if cmd.Name != "" {
		t.Errorf("got name=%q, want empty (no group)", cmd.Name)
	}
}

func TestSingleCmd_DraggableWithGroup(t *testing.T) {
	b := binding{GID: "g10", Dir: "draggable:palette", Expr: "'red'"}

	cmd := singleCmd(b, nil, nil)
	if cmd.Op != "draggable" || cmd.Name != "palette" || cmd.GetStrVal() != "red" {
		t.Errorf("got op=%q name=%q strVal=%q", cmd.Op, cmd.Name, cmd.GetStrVal())
	}
}

func TestSingleCmd_Dropzone(t *testing.T) {
	b := binding{GID: "g11", Dir: "dropzone", Expr: "'canvas'"}

	cmd := singleCmd(b, nil, nil)
	if cmd.Op != "dropzone" || cmd.GetStrVal() != "canvas" {
		t.Errorf("got op=%q strVal=%q", cmd.Op, cmd.GetStrVal())
	}
}

func TestSingleEventCmd_Drop(t *testing.T) {
	e := eventBinding{GID: "g12", Event: "drop", Key: "palette", Method: "Add"}

	evt := singleEventCmd(e, nil, nil)
	if evt.On != "drop" || evt.Key != "palette" {
		t.Errorf("got on=%q key=%q", evt.On, evt.Key)
	}
}

// --- Nested g-for tests ---

type nestedTestComp struct {
	Component
	Groups []nestedTestGroup
}

type nestedTestGroup struct {
	Name  string
	Items []nestedTestItem
}

type nestedTestItem struct {
	Label string
}

func TestComputeSubLoopCmd(t *testing.T) {
	comp := &nestedTestComp{
		Groups: []nestedTestGroup{
			{Name: "A", Items: []nestedTestItem{{Label: "A1"}, {Label: "A2"}}},
			{Name: "B", Items: []nestedTestItem{{Label: "B1"}}},
		},
	}
	ci := newTestCI(comp)

	html := `<html><body><div g-for="group in Groups"><span g-text="group.Name"></span><li g-for="item in group.Items"><span g-text="item.Label"></span></li></div></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb

	// Full init should produce commands for nested lists
	msg, err := computeInitMessage(pb, ci)
	if err != nil {
		t.Fatal(err)
	}

	// Find the outer list command
	var outerList *Command
	for _, c := range msg.Commands {
		if c.Op == "list" {
			outerList = c
			break
		}
	}
	if outerList == nil {
		t.Fatal("expected outer list command")
	}
	if len(outerList.Items) != 2 {
		t.Fatalf("expected 2 outer items, got %d", len(outerList.Items))
	}

	// First outer item should have a sub-list command for its 2 inner items
	item0Cmds := outerList.Items[0].Cmds
	var innerList0 *Command
	for _, c := range item0Cmds {
		if c.Op == "list" {
			innerList0 = c
			break
		}
	}
	if innerList0 == nil {
		t.Fatal("expected inner list command for first group")
	}
	if len(innerList0.Items) != 2 {
		t.Errorf("expected 2 inner items for group A, got %d", len(innerList0.Items))
	}

	// Second outer item should have 1 inner item
	item1Cmds := outerList.Items[1].Cmds
	var innerList1 *Command
	for _, c := range item1Cmds {
		if c.Op == "list" {
			innerList1 = c
			break
		}
	}
	if innerList1 == nil {
		t.Fatal("expected inner list command for second group")
	}
	if len(innerList1.Items) != 1 {
		t.Errorf("expected 1 inner item for group B, got %d", len(innerList1.Items))
	}

	// Verify the inner list GIDs are distinct (contain the outer index)
	if innerList0.Id == innerList1.Id {
		t.Errorf("inner list GIDs should differ: both are %q", innerList0.Id)
	}
}

func TestComputeSubLoopCmd_InnerBindings(t *testing.T) {
	comp := &nestedTestComp{
		Groups: []nestedTestGroup{
			{Name: "G", Items: []nestedTestItem{{Label: "X"}}},
		},
	}
	ci := newTestCI(comp)

	html := `<html><body><div g-for="group in Groups"><li g-for="item in group.Items"><span g-text="item.Label"></span></li></div></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	ci.pb = pb

	msg, err := computeInitMessage(pb, ci)
	if err != nil {
		t.Fatal(err)
	}

	// Navigate to inner list item's commands
	var outerList *Command
	for _, c := range msg.Commands {
		if c.Op == "list" {
			outerList = c
			break
		}
	}
	if outerList == nil || len(outerList.Items) != 1 {
		t.Fatal("expected 1 outer item")
	}

	var innerList *Command
	for _, c := range outerList.Items[0].Cmds {
		if c.Op == "list" {
			innerList = c
			break
		}
	}
	if innerList == nil || len(innerList.Items) != 1 {
		t.Fatal("expected 1 inner item")
	}

	// The inner item should have a text command setting "X"
	innerItemCmds := innerList.Items[0].Cmds
	found := false
	for _, c := range innerItemCmds {
		if c.Op == "text" && c.GetStrVal() == "X" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected text command with value 'X' for inner item binding")
	}
}

func TestValToStr(t *testing.T) {
	tests := []struct {
		val  interface{}
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(5), "5"},
		{float64(5.5), "5.5"},
		{float64(1000000), "1000000"},
		{true, "true"},
		{false, "false"},
		{42, "42"},
	}

	for _, tt := range tests {
		got := valToStr(tt.val)
		if got != tt.want {
			t.Errorf("valToStr(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}
