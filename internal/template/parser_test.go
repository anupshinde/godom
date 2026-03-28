package template

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

// --- Template expansion tests ---

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

	result, err := ExpandComponents(`<div><my-comp></my-comp></div>`, fsys, ".")
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

	result, err := ExpandComponents(`<my-item g-for="x in Items"></my-item>`, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, `g-for="x in Items"`) {
		t.Errorf("g-for should be transferred to root: %s", result)
	}
}

func TestExpandComponents_SelfClosing(t *testing.T) {
	fsys := fstest.MapFS{
		"my-tag.html": {Data: []byte(`<div>content</div>`)},
	}

	result, err := ExpandComponents(`<my-tag />`, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "<div>content</div>") {
		t.Errorf("expected expansion: %s", result)
	}
}

func TestExpandComponents_MissingFile(t *testing.T) {
	fsys := fstest.MapFS{}

	_, err := ExpandComponents(`<my-tag></my-tag>`, fsys, ".")
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
	_, err := ExpandComponents(`<div><my-comp>content</div>`, fsys, ".")
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
	result, err := ExpandComponents(`<outer-comp></outer-comp>`, fsys, ".")
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

func TestExpandComponents_NoCustomElements(t *testing.T) {
	// Plain HTML with no custom elements should pass through unchanged
	fsys := fstest.MapFS{}
	input := `<div><span>hello</span></div>`
	result, err := ExpandComponents(input, fsys, ".")
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
	result, err := ExpandComponents(`<my-tag class="foo" id="bar"></my-tag>`, fsys, ".")
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

func TestExpandComponents_SkipsGTags(t *testing.T) {
	// g-* tags are framework directives, not custom components — they should
	// be left in place and not trigger a file lookup.
	fsys := fstest.MapFS{}
	input := `<div><g-slot instance="sidebar"></g-slot><span>after</span></div>`
	result, err := ExpandComponents(input, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "g-slot") {
		t.Errorf("expected g-slot to remain, got: %s", result)
	}
	if !strings.Contains(result, "<span>after</span>") {
		t.Errorf("expected sibling to remain, got: %s", result)
	}
}

func TestExpandComponents_GTagBeforeCustomElement(t *testing.T) {
	// g-slot appears before a real custom element — g-slot is skipped,
	// custom element is expanded.
	fsys := fstest.MapFS{
		"my-comp.html": {Data: []byte(`<span>expanded</span>`)},
	}
	input := `<div><g-slot instance="x"></g-slot><my-comp></my-comp></div>`
	result, err := ExpandComponents(input, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "g-slot") {
		t.Errorf("expected g-slot to remain, got: %s", result)
	}
	if !strings.Contains(result, "<span>expanded</span>") {
		t.Errorf("expected my-comp to be expanded, got: %s", result)
	}
}

func TestExpandComponents_SkipsGTagSelfClosing(t *testing.T) {
	// Self-closing g-* tags like <g-slot instance="x"/> should also be skipped.
	fsys := fstest.MapFS{}
	input := `<div><g-slot instance="x"/><span>ok</span></div>`
	result, err := ExpandComponents(input, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "g-slot") {
		t.Errorf("expected g-slot to remain, got: %s", result)
	}
	if !strings.Contains(result, "<span>ok</span>") {
		t.Errorf("expected sibling to remain, got: %s", result)
	}
}

func TestExpandComponents_MultipleGTagsBeforeCustomElement(t *testing.T) {
	// Multiple g-* tags should all be skipped without consuming the expansion budget.
	fsys := fstest.MapFS{
		"my-comp.html": {Data: []byte(`<b>hi</b>`)},
	}
	input := `<g-slot instance="a"></g-slot><g-slot instance="b"></g-slot><g-slot instance="c"></g-slot><my-comp></my-comp>`
	result, err := ExpandComponents(input, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	// All 3 g-slots should remain
	if strings.Count(result, "g-slot") != 6 { // 3 open + 3 close tags
		t.Errorf("expected all 3 g-slots to remain, got: %s", result)
	}
	// my-comp should be expanded
	if !strings.Contains(result, "<b>hi</b>") {
		t.Errorf("expected my-comp to be expanded, got: %s", result)
	}
}

func TestExpandComponents_GTagDoesNotConsumeExpansionBudget(t *testing.T) {
	// g-* tags use `expansions--` to avoid consuming the budget.
	// Verify that 10 g-slots + 1 custom element still works (budget = 10 expansions).
	fsys := fstest.MapFS{
		"my-comp.html": {Data: []byte(`<em>done</em>`)},
	}
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString(fmt.Sprintf(`<g-slot instance="s%d"></g-slot>`, i))
	}
	sb.WriteString(`<my-comp></my-comp>`)
	result, err := ExpandComponents(sb.String(), fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<em>done</em>") {
		t.Errorf("expected my-comp to be expanded despite 10 g-slots, got: %s", result)
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

	result, err := ExpandComponents(`<level-0></level-0>`, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	// After 10 iterations, some custom elements should remain unexpanded
	// (depth 10+ won't be processed)
	if !strings.Contains(result, "level-") {
		t.Error("expected some custom elements to remain after max depth exhaustion")
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


