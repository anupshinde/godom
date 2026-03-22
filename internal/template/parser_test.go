package template

import (
	"fmt"
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
		p := ParseForExprParts(tt.expr)
		if p == nil {
			t.Fatalf("ParseForExprParts(%q) returned nil", tt.expr)
		}
		if p.Item != tt.wantItem {
			t.Errorf("Item = %q, want %q", p.Item, tt.wantItem)
		}
		if p.Index != tt.wantIndex {
			t.Errorf("Index = %q, want %q", p.Index, tt.wantIndex)
		}
		if p.List != tt.wantList {
			t.Errorf("List = %q, want %q", p.List, tt.wantList)
		}
	}
}

func TestParseForExprParts_Invalid(t *testing.T) {
	if p := ParseForExprParts("invalid"); p != nil {
		t.Error("expected nil for invalid expression")
	}
}

func TestParsePropsAttr(t *testing.T) {
	props := ParsePropsAttr("index:i,todo:todo")
	if props["index"] != "i" {
		t.Errorf("index = %q, want i", props["index"])
	}
	if props["todo"] != "todo" {
		t.Errorf("todo = %q, want todo", props["todo"])
	}
}

func TestParsePropsAttr_Empty(t *testing.T) {
	props := ParsePropsAttr("")
	if props != nil {
		t.Errorf("expected nil for empty, got %v", props)
	}
}

func TestParsePropsAttr_Whitespace(t *testing.T) {
	props := ParsePropsAttr(" name : expr , other : val ")
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
		if got := ExprRoot(tt.expr); got != tt.want {
			t.Errorf("ExprRoot(%q) = %q, want %q", tt.expr, got, tt.want)
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
		got := ExtractProps(tt.attrs)
		if got != tt.want {
			t.Errorf("ExtractProps(%q) = %q, want %q", tt.attrs, got, tt.want)
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
		got := ExtractGAttrs(tt.attrs)
		if got != tt.want {
			t.Errorf("ExtractGAttrs(%q) = %q, want %q", tt.attrs, got, tt.want)
		}
	}
}

func TestTransferAttrsToRoot(t *testing.T) {
	h := `<li class="item">content</li>`
	got := TransferAttrsToRoot(h, `g-for="x in Y"`)
	if !strings.Contains(got, `g-for="x in Y"`) {
		t.Errorf("expected g-for attr in result: %s", got)
	}
	if !strings.Contains(got, `class="item"`) {
		t.Errorf("expected original class preserved: %s", got)
	}
}

func TestTransferAttrsToRoot_SelfClosing(t *testing.T) {
	h := `<input type="text" />`
	got := TransferAttrsToRoot(h, `g-bind="Name"`)
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

	result, err := ExpandComponents(`<div><my-comp></my-comp></div>`, fsys)
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

	result, err := ExpandComponents(`<my-item g-for="x in Items"></my-item>`, fsys)
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

	result, err := ExpandComponents(`<my-item :name="item.Name" :index="i"></my-item>`, fsys)
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

	result, err := ExpandComponents(`<my-tag />`, fsys)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "<div>content</div>") {
		t.Errorf("expected expansion: %s", result)
	}
}

func TestExpandComponents_MissingFile(t *testing.T) {
	fsys := fstest.MapFS{}

	_, err := ExpandComponents(`<my-tag></my-tag>`, fsys)
	if err == nil {
		t.Error("expected error for missing component file")
	}
}

// --- Additional coverage tests ---

func TestTransferAttrsToRoot_NoClosingBracket(t *testing.T) {
	// Input with no ">" should be returned unchanged
	got := TransferAttrsToRoot("no bracket here", `g-text="X"`)
	if got != "no bracket here" {
		t.Errorf("expected unchanged input, got: %s", got)
	}
}

func TestExpandComponents_MissingClosingTag(t *testing.T) {
	fsys := fstest.MapFS{
		"my-comp.html": {Data: []byte(`<span>hello</span>`)},
	}
	_, err := ExpandComponents(`<div><my-comp>content</div>`, fsys)
	if err == nil {
		t.Error("expected error for missing closing tag")
	}
	if err != nil && !strings.Contains(err.Error(), "missing closing tag") {
		t.Errorf("expected 'missing closing tag' in error, got: %v", err)
	}
}

func TestExpandComponents_Recursive(t *testing.T) {
	fsys := fstest.MapFS{
		"outer-comp.html": {Data: []byte(`<div><inner-comp></inner-comp></div>`)},
		"inner-comp.html": {Data: []byte(`<span>inner</span>`)},
	}
	result, err := ExpandComponents(`<outer-comp></outer-comp>`, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<span>inner</span>") {
		t.Errorf("expected recursive expansion, got: %s", result)
	}
	if strings.Contains(result, "outer-comp") || strings.Contains(result, "inner-comp") {
		t.Errorf("custom tags should be replaced, got: %s", result)
	}
}

func TestExpandComponents_SelfClosingWithGAttrsAndProps(t *testing.T) {
	fsys := fstest.MapFS{
		"my-item.html": {Data: []byte(`<li>item</li>`)},
	}
	result, err := ExpandComponents(`<my-item g-text="Name" :val="x" />`, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `g-text="Name"`) {
		t.Errorf("expected g-text transferred, got: %s", result)
	}
	if !strings.Contains(result, `g-props="val:x"`) {
		t.Errorf("expected g-props transferred, got: %s", result)
	}
}

func TestExpandComponents_NoCustomElements(t *testing.T) {
	// Plain HTML with no custom elements should pass through unchanged
	fsys := fstest.MapFS{}
	input := `<div><span>hello</span></div>`
	result, err := ExpandComponents(input, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if result != input {
		t.Errorf("expected unchanged output, got: %s", result)
	}
}

func TestExpandComponents_AttrsNoGAttrsNoProps(t *testing.T) {
	// Custom element with only plain attrs (no g-* or :prop) — attrs string is non-empty
	// but extractGAttrs and extractProps return empty
	fsys := fstest.MapFS{
		"my-tag.html": {Data: []byte(`<div>content</div>`)},
	}
	result, err := ExpandComponents(`<my-tag class="foo" id="bar"></my-tag>`, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<div>content</div>") {
		t.Errorf("expected expansion, got: %s", result)
	}
	// Should NOT have g-props or g-* injected
	if strings.Contains(result, "g-props") {
		t.Errorf("should not have g-props, got: %s", result)
	}
}

// === Negative tests and edge cases ===

func TestExpandComponents_MaxDepthExhaustion(t *testing.T) {
	// After 10 iterations, remaining custom elements are left in place (no error)
	// Build a chain deeper than 10
	fsys := fstest.MapFS{}
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("level-%d", i)
		nextName := fmt.Sprintf("level-%d", i+1)
		fsys[name+".html"] = &fstest.MapFile{Data: []byte(fmt.Sprintf(`<div><level-%d></level-%d></div>`, i+1, i+1))}
		_ = nextName
	}
	// The last level just has content
	fsys["level-12.html"] = &fstest.MapFile{Data: []byte(`<span>bottom</span>`)}

	result, err := ExpandComponents(`<level-0></level-0>`, fsys)
	if err != nil {
		t.Fatal(err)
	}
	// After 10 iterations, some custom elements should remain unexpanded
	// (depth 10+ won't be processed)
	if !strings.Contains(result, "level-") {
		t.Error("expected some custom elements to remain after max depth exhaustion")
	}
}

func TestParsePropsAttr_MalformedPairNoColon(t *testing.T) {
	// Pair without a colon is skipped — SplitN produces len 1
	props := ParsePropsAttr("nocolon,valid:value")
	if props["valid"] != "value" {
		t.Errorf("valid pair should parse, got: %v", props)
	}
	if _, ok := props["nocolon"]; ok {
		t.Error("pair without colon should be skipped")
	}
}

func TestParsePropsAttr_AllMalformed(t *testing.T) {
	props := ParsePropsAttr("nocolon")
	// Map is created but empty (not nil)
	if len(props) != 0 {
		t.Errorf("expected empty map for all-malformed input, got: %v", props)
	}
}

func TestIsLiteral_NegativeNumber(t *testing.T) {
	// strconv.Atoi("-1") succeeds
	if !IsLiteral("-1") {
		t.Error("expected -1 to be a literal")
	}
}

func TestIsLiteral_MismatchedQuotes(t *testing.T) {
	if IsLiteral(`"hello'`) {
		t.Error("mismatched quotes should not be literal")
	}
	if IsLiteral(`'hello"`) {
		t.Error("mismatched quotes should not be literal")
	}
}

func TestIsLiteral_SingleChar(t *testing.T) {
	// Single character is too short for quoted string (len < 2)
	if IsLiteral(`"`) {
		t.Error("single quote char should not be literal")
	}
}

func TestIsLiteral_EmptyQuotedString(t *testing.T) {
	// "" is len 2, s[0]=='"', s[len-1]=='"' → literal
	if !IsLiteral(`""`) {
		t.Error("empty double-quoted string should be literal")
	}
	if !IsLiteral(`''`) {
		t.Error("empty single-quoted string should be literal")
	}
}

func TestIsLiteral_EmptyString(t *testing.T) {
	if IsLiteral("") {
		t.Error("empty string should not be literal")
	}
}

func TestTransferAttrsToRoot_ExactOneBracket(t *testing.T) {
	// ">" at index 0 — should not panic.
	// BUG: code does htmlStr[idx-1] without bounds check, panics with index -1.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("TransferAttrsToRoot panics on bare '>': %v", r)
		}
	}()
	_ = TransferAttrsToRoot(">", `g-text="X"`)
}

func TestExtractGAttrs_Empty(t *testing.T) {
	got := ExtractGAttrs("")
	if got != "" {
		t.Errorf("expected empty string for empty attrs, got: %q", got)
	}
}

func TestExtractProps_MultipleColonsInValue(t *testing.T) {
	// :url="item.Host:Port" — the regex should capture the whole value
	// propAttrRe: `:([a-zA-Z][a-zA-Z0-9_]*)\s*=\s*"([^"]*)"`
	// "item.Host:Port" doesn't contain quotes so [^"]* matches it all
	got := ExtractProps(`:url="item.Host:Port"`)
	if got != "url:item.Host:Port" {
		t.Errorf("expected url:item.Host:Port, got: %q", got)
	}
}

func TestExprRoot_LeadingWhitespace(t *testing.T) {
	// TrimSpace should handle leading/trailing whitespace
	if got := ExprRoot(" Name "); got != "Name" {
		t.Errorf("ExprRoot(\" Name \") = %q, want Name", got)
	}
}

func TestExprRoot_DottedWithWhitespace(t *testing.T) {
	if got := ExprRoot(" todo.Done "); got != "todo" {
		t.Errorf("ExprRoot(\" todo.Done \") = %q, want todo", got)
	}
}

