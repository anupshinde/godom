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
