package godom

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseForExprParts(t *testing.T) {
	tests := []struct {
		expr      string
		wantItem  string
		wantIndex string
		wantList  string
	}{
		{"todo in Todos", "todo", "", "Todos"},
		{"todo, i in Todos", "todo", "i", "Todos"},
		{"item , idx in Items", "item", "idx", "Items"},
	}

	for _, tt := range tests {
		p := parseForExprParts(tt.expr)
		if p == nil {
			t.Fatalf("parseForExprParts(%q) returned nil", tt.expr)
		}
		if p.item != tt.wantItem {
			t.Errorf("item = %q, want %q", p.item, tt.wantItem)
		}
		if p.index != tt.wantIndex {
			t.Errorf("index = %q, want %q", p.index, tt.wantIndex)
		}
		if p.list != tt.wantList {
			t.Errorf("list = %q, want %q", p.list, tt.wantList)
		}
	}
}

func TestParseForExprParts_Invalid(t *testing.T) {
	if p := parseForExprParts("invalid"); p != nil {
		t.Error("expected nil for invalid expression")
	}
}

func TestParsePropsAttr(t *testing.T) {
	props := parsePropsAttr("index:i,todo:todo")
	if props["index"] != "i" {
		t.Errorf("index = %q, want i", props["index"])
	}
	if props["todo"] != "todo" {
		t.Errorf("todo = %q, want todo", props["todo"])
	}
}

func TestParsePropsAttr_Empty(t *testing.T) {
	props := parsePropsAttr("")
	if props != nil {
		t.Errorf("expected nil for empty, got %v", props)
	}
}

func TestParsePropsAttr_Whitespace(t *testing.T) {
	props := parsePropsAttr(" name : expr , other : val ")
	if props["name"] != "expr" {
		t.Errorf("name = %q, want expr", props["name"])
	}
	if props["other"] != "val" {
		t.Errorf("other = %q, want val", props["other"])
	}
}

func TestExprRoot(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{"InputText", "InputText"},
		{"todo.Done", "todo"},
		{"item.Address.City", "item"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := exprRoot(tt.expr); got != tt.want {
			t.Errorf("exprRoot(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}

func TestParsePageHTML_AssignsGIDs(t *testing.T) {
	html := `<html><body><span g-text="Name">placeholder</span></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	if len(pb.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(pb.Bindings))
	}

	b := pb.Bindings[0]
	if b.Dir != "text" {
		t.Errorf("dir = %q, want text", b.Dir)
	}
	if b.Expr != "Name" {
		t.Errorf("expr = %q, want Name", b.Expr)
	}
	if b.GID == "" {
		t.Error("expected GID to be assigned")
	}

	// HTML should contain the data-gid attribute
	if !strings.Contains(pb.HTML, "data-gid") {
		t.Error("expected data-gid in output HTML")
	}
}

func TestParsePageHTML_MultipleDirectives(t *testing.T) {
	html := `<html><body>
		<span g-text="Name"></span>
		<input g-bind="Email" />
		<div g-show="Visible"></div>
	</body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	if len(pb.Bindings) != 3 {
		t.Fatalf("expected 3 bindings, got %d", len(pb.Bindings))
	}
}

func TestParsePageHTML_Events(t *testing.T) {
	html := `<html><body><button g-click="Save">Save</button></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	if len(pb.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pb.Events))
	}

	e := pb.Events[0]
	if e.Event != "click" {
		t.Errorf("event = %q, want click", e.Event)
	}
	if e.Method != "Save" {
		t.Errorf("method = %q, want Save", e.Method)
	}
}

func TestParsePageHTML_KeydownEvent(t *testing.T) {
	html := `<html><body><input g-keydown="Enter:Submit" /></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	if len(pb.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pb.Events))
	}

	e := pb.Events[0]
	if e.Event != "keydown" {
		t.Errorf("event = %q, want keydown", e.Event)
	}
	if e.Key != "Enter" {
		t.Errorf("key = %q, want Enter", e.Key)
	}
	if e.Method != "Submit" {
		t.Errorf("method = %q, want Submit", e.Method)
	}
}

func TestParsePageHTML_BindGeneratesInputEvent(t *testing.T) {
	html := `<html><body><input g-bind="Name" /></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	// g-bind should produce both a binding AND an input event
	if len(pb.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(pb.Bindings))
	}
	if len(pb.Events) != 1 {
		t.Fatalf("expected 1 event (input), got %d", len(pb.Events))
	}

	e := pb.Events[0]
	if e.Event != "input" {
		t.Errorf("event = %q, want input", e.Event)
	}
	if e.Method != "__bind" {
		t.Errorf("method = %q, want __bind", e.Method)
	}
}

func TestParsePageHTML_ForLoop(t *testing.T) {
	html := `<html><body><ul><li g-for="item, i in Items"><span g-text="item.Name"></span></li></ul></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	if len(pb.ForLoops) != 1 {
		t.Fatalf("expected 1 for loop, got %d", len(pb.ForLoops))
	}

	fl := pb.ForLoops[0]
	if fl.ItemVar != "item" {
		t.Errorf("ItemVar = %q, want item", fl.ItemVar)
	}
	if fl.IndexVar != "i" {
		t.Errorf("IndexVar = %q, want i", fl.IndexVar)
	}
	if fl.ListField != "Items" {
		t.Errorf("ListField = %q, want Items", fl.ListField)
	}
	if len(fl.Bindings) != 1 {
		t.Fatalf("expected 1 binding in template, got %d", len(fl.Bindings))
	}

	// Template HTML should have __IDX__ placeholders
	if !strings.Contains(fl.TemplateHTML, "__IDX__") {
		t.Error("expected __IDX__ in template HTML")
	}

	// Main HTML should have comment anchors
	if !strings.Contains(pb.HTML, "g-for:") {
		t.Error("expected g-for anchor comments in output HTML")
	}
}

func TestParsePageHTML_FieldToBindingsIndex(t *testing.T) {
	html := `<html><body>
		<span g-text="Name"></span>
		<span g-text="Name"></span>
		<span g-text="Age"></span>
	</body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	nameIndices := pb.FieldToBindings["Name"]
	if len(nameIndices) != 2 {
		t.Errorf("expected 2 indices for Name, got %d", len(nameIndices))
	}

	ageIndices := pb.FieldToBindings["Age"]
	if len(ageIndices) != 1 {
		t.Errorf("expected 1 index for Age, got %d", len(ageIndices))
	}
}

func TestParsePageHTML_FieldToForLoopsIndex(t *testing.T) {
	html := `<html><body><li g-for="item in Items"><span g-text="item.Name"></span></li></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	itemsIndices := pb.FieldToForLoops["Items"]
	if len(itemsIndices) != 1 {
		t.Errorf("expected 1 for loop for Items, got %d", len(itemsIndices))
	}
}

func TestHasDirectiveAttr(t *testing.T) {
	html := `<html><body><span g-text="X"></span><div class="normal"></div></body></html>`
	pb, err := parsePageHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	// Only the span with g-text should get a binding
	if len(pb.Bindings) != 1 {
		t.Errorf("expected 1 binding, got %d", len(pb.Bindings))
	}
}

// --- Template expansion tests ---

func TestExtractProps(t *testing.T) {
	tests := []struct {
		attrs string
		want  string
	}{
		{`:name="item.Name" :index="i"`, "name:item.Name,index:i"},
		{`:todo="todo"`, "todo:todo"},
		{`class="foo"`, ""},
		{``, ""},
	}

	for _, tt := range tests {
		got := extractProps(tt.attrs)
		if got != tt.want {
			t.Errorf("extractProps(%q) = %q, want %q", tt.attrs, got, tt.want)
		}
	}
}

func TestExtractGAttrs(t *testing.T) {
	tests := []struct {
		attrs string
		want  string
	}{
		{`g-for="item in Items" class="list"`, `g-for="item in Items"`},
		{`g-text="Name" g-show="Visible"`, `g-text="Name" g-show="Visible"`},
		{`class="foo" id="bar"`, ""},
		{`g-class:done="Done"`, `g-class:done="Done"`},
	}

	for _, tt := range tests {
		got := extractGAttrs(tt.attrs)
		if got != tt.want {
			t.Errorf("extractGAttrs(%q) = %q, want %q", tt.attrs, got, tt.want)
		}
	}
}

func TestTransferAttrsToRoot(t *testing.T) {
	h := `<li class="item">content</li>`
	got := transferAttrsToRoot(h, `g-for="x in Y"`)
	if !strings.Contains(got, `g-for="x in Y"`) {
		t.Errorf("expected g-for attr in result: %s", got)
	}
	if !strings.Contains(got, `class="item"`) {
		t.Errorf("expected original class preserved: %s", got)
	}
}

func TestTransferAttrsToRoot_SelfClosing(t *testing.T) {
	h := `<input type="text" />`
	got := transferAttrsToRoot(h, `g-bind="Name"`)
	if !strings.Contains(got, `g-bind="Name"`) {
		t.Errorf("expected g-bind in result: %s", got)
	}
	if !strings.Contains(got, `/>`) {
		t.Errorf("expected self-closing preserved: %s", got)
	}
}

func TestExpandComponents(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":   {Data: []byte(`<div><my-comp></my-comp></div>`)},
		"my-comp.html": {Data: []byte(`<span>hello</span>`)},
	}

	result, err := expandComponents(`<div><my-comp></my-comp></div>`, fsys, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "<span>hello</span>") {
		t.Errorf("expected component expansion: %s", result)
	}
	if strings.Contains(result, "my-comp") {
		t.Errorf("custom tag should be replaced: %s", result)
	}
}

func TestExpandComponents_WithGAttrs(t *testing.T) {
	fsys := fstest.MapFS{
		"my-item.html": {Data: []byte(`<li>item</li>`)},
	}

	result, err := expandComponents(`<my-item g-for="x in Items"></my-item>`, fsys, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, `g-for="x in Items"`) {
		t.Errorf("g-for should be transferred to root: %s", result)
	}
}

func TestExpandComponents_WithProps(t *testing.T) {
	fsys := fstest.MapFS{
		"my-item.html": {Data: []byte(`<li>item</li>`)},
	}

	result, err := expandComponents(`<my-item :name="item.Name" :index="i"></my-item>`, fsys, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "g-props=") {
		t.Errorf("expected g-props attribute: %s", result)
	}
}

func TestExpandComponents_SelfClosing(t *testing.T) {
	fsys := fstest.MapFS{
		"my-tag.html": {Data: []byte(`<div>content</div>`)},
	}

	result, err := expandComponents(`<my-tag />`, fsys, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "<div>content</div>") {
		t.Errorf("expected expansion: %s", result)
	}
}

func TestExpandComponents_MissingFile(t *testing.T) {
	fsys := fstest.MapFS{}

	_, err := expandComponents(`<my-tag></my-tag>`, fsys, nil)
	if err == nil {
		t.Error("expected error for missing component file")
	}
}

func TestFindIndexHTML_Root(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte(`<html></html>`)},
	}

	found, err := findIndexHTML(fsys)
	if err != nil {
		t.Fatal(err)
	}

	data, err := found.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	data.Close()
}

func TestFindIndexHTML_Subdir(t *testing.T) {
	fsys := fstest.MapFS{
		"ui/index.html": {Data: []byte(`<html></html>`)},
	}

	found, err := findIndexHTML(fsys)
	if err != nil {
		t.Fatal(err)
	}

	data, err := found.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	data.Close()
}

func TestFindIndexHTML_NotFound(t *testing.T) {
	fsys := fstest.MapFS{
		"other.html": {Data: []byte(`<html></html>`)},
	}

	_, err := findIndexHTML(fsys)
	if err == nil {
		t.Error("expected error when index.html not found")
	}
}
