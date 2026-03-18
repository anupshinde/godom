package godom

import (
	"reflect"
	"strings"
	"testing"

	"github.com/anupshinde/godom/vdom"
)

// ---------------------------------------------------------------------------
// parseTemplate tests
// ---------------------------------------------------------------------------

func TestParseTemplate_SimpleText(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>Hello World</body></html>`
	nodes, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	tn := nodes[0]
	if !tn.IsText {
		t.Fatal("expected text node")
	}
	if tn.TextParts[0].Value != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", tn.TextParts[0].Value)
	}
}

func TestParseTemplate_Element(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body><div class="main"><span>hi</span></div></body></html>`
	nodes, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should have one div element
	div := findTemplateTag(nodes, "div")
	if div == nil {
		t.Fatal("expected div element")
	}
	if len(div.Attrs) == 0 {
		t.Fatal("expected class attribute on div")
	}
	// Should have a span child
	span := findTemplateTag(div.Children, "span")
	if span == nil {
		t.Fatal("expected span child")
	}
}

func TestParseTemplate_Directives(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<span g-text="Name">placeholder</span>
		<input g-bind="Email"/>
		<div g-if="ShowPanel">content</div>
		<div g-show="Visible">content</div>
		<button g-click="Save">Save</button>
		<div g-class:active="IsActive">text</div>
		<div g-style:width="Width">text</div>
		<div g-attr:transform="Transform">text</div>
	</body></html>`

	nodes, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		tag     string
		dirType string
	}{
		{"span", "text"},
		{"input", "bind"},
		{"div", "if"},
		{"button", "click"},
	}

	for _, tt := range tests {
		node := findTemplateTagWithDirective(nodes, tt.tag, tt.dirType)
		if node == nil {
			t.Errorf("expected %s element with %s directive", tt.tag, tt.dirType)
		}
	}
}

func TestParseTemplate_GFor(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<ul>
			<li g-for="todo, i in Todos" g-key="todo.ID">
				<span g-text="todo.Text"></span>
			</li>
		</ul>
	</body></html>`

	nodes, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	ul := findTemplateTag(nodes, "ul")
	if ul == nil {
		t.Fatal("expected ul element")
	}

	// Find the g-for node inside ul
	var forNode *vdom.TemplateNode
	for _, c := range ul.Children {
		if c.IsFor {
			forNode = c
			break
		}
	}
	if forNode == nil {
		t.Fatal("expected g-for node")
	}
	if forNode.ForItem != "todo" {
		t.Errorf("expected item var 'todo', got %q", forNode.ForItem)
	}
	if forNode.ForIndex != "i" {
		t.Errorf("expected index var 'i', got %q", forNode.ForIndex)
	}
	if forNode.ForList != "Todos" {
		t.Errorf("expected list 'Todos', got %q", forNode.ForList)
	}
	if forNode.ForKey != "todo.ID" {
		t.Errorf("expected key 'todo.ID', got %q", forNode.ForKey)
	}
	if len(forNode.ForBody) != 1 {
		t.Fatalf("expected 1 body template, got %d", len(forNode.ForBody))
	}
}

func TestParseTemplate_TextInterpolation(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<p>Hello {{Name}}, you have {{Count}} items</p>
	</body></html>`

	nodes, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	p := findTemplateTag(nodes, "p")
	if p == nil {
		t.Fatal("expected p element")
	}
	if len(p.Children) != 1 {
		t.Fatalf("expected 1 text child, got %d", len(p.Children))
	}
	text := p.Children[0]
	if !text.IsText {
		t.Fatal("expected text node")
	}
	// Should have 5 parts: "Hello ", {{Name}}, ", you have ", {{Count}}, " items"
	if len(text.TextParts) < 4 {
		t.Fatalf("expected at least 4 text parts, got %d: %+v", len(text.TextParts), text.TextParts)
	}
	// Check the dynamic parts
	found := false
	for _, p := range text.TextParts {
		if !p.Static && p.Value == "Name" {
			found = true
		}
	}
	if !found {
		t.Error("expected dynamic part for 'Name'")
	}
}

func TestParseTemplate_Component(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<todo-item :text="todo.Text" :done="todo.Done"></todo-item>
	</body></html>`

	comps := map[string]bool{"todo-item": true}
	nodes, err := vdom.ParseTemplate(html, comps)
	if err != nil {
		t.Fatal(err)
	}

	var comp *vdom.TemplateNode
	for _, n := range nodes {
		if n.IsComponent {
			comp = n
			break
		}
	}
	if comp == nil {
		t.Fatal("expected component node")
	}
	if comp.ComponentTag != "todo-item" {
		t.Errorf("expected tag 'todo-item', got %q", comp.ComponentTag)
	}
	if comp.PropExprs["text"] != "todo.Text" {
		t.Errorf("expected prop 'text' = 'todo.Text', got %q", comp.PropExprs["text"])
	}
	if comp.PropExprs["done"] != "todo.Done" {
		t.Errorf("expected prop 'done' = 'todo.Done', got %q", comp.PropExprs["done"])
	}
}

// ---------------------------------------------------------------------------
// resolveTree tests
// ---------------------------------------------------------------------------

type testCounter struct {
	Count int
	Step  int
	Name  string
}

func TestResolveTree_SimpleElement(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<h1><span g-text="Count">0</span></h1>
		<button g-click="Increment">+</button>
	</body></html>`

	templates, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testCounter{Count: 42, Step: 1, Name: "test"}
	ctx := &vdom.ResolveContext{
		State: reflect.ValueOf(state),
		Vars:  make(map[string]any),
	}

	nodes := vdom.ResolveTree(templates, ctx)

	// Find the span with g-text — should have "42" as text
	found := findNodeText(nodes, "42")
	if !found {
		t.Error("expected text node with '42'")
	}
}

type testTodoApp struct {
	Todos []testTodo
}

type testTodo struct {
	ID   int
	Text string
	Done bool
}

func TestResolveTree_GFor(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<ul>
			<li g-for="todo in Todos">
				<span g-text="todo.Text"></span>
			</li>
		</ul>
	</body></html>`

	templates, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testTodoApp{
		Todos: []testTodo{
			{ID: 1, Text: "Buy milk", Done: false},
			{ID: 2, Text: "Write code", Done: true},
		},
	}
	ctx := &vdom.ResolveContext{
		State: reflect.ValueOf(state),
		Vars:  make(map[string]any),
	}

	nodes := vdom.ResolveTree(templates, ctx)

	// The ul should have 2 li children (expanded from g-for)
	ul := findElement(nodes, "ul")
	if ul == nil {
		t.Fatal("expected ul element")
	}
	liCount := 0
	for _, c := range ul.Children {
		if el, ok := c.(*vdom.ElementNode); ok && el.Tag == "li" {
			liCount++
		}
	}
	if liCount != 2 {
		t.Errorf("expected 2 li elements, got %d", liCount)
	}

	// Check that text was resolved
	if !findNodeText(nodes, "Buy milk") {
		t.Error("expected 'Buy milk' in output")
	}
	if !findNodeText(nodes, "Write code") {
		t.Error("expected 'Write code' in output")
	}
}

func TestResolveTree_GIf(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-if="Done">completed</div>
	</body></html>`

	templates, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Done = true → div should be present
	state := &testTodo{Done: true}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := vdom.ResolveTree(templates, ctx)
	if findElement(nodes, "div") == nil {
		t.Error("expected div when Done=true")
	}

	// Done = false → div should be absent
	state2 := &testTodo{Done: false}
	ctx2 := &vdom.ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	nodes2 := vdom.ResolveTree(templates, ctx2)
	if findElement(nodes2, "div") != nil {
		t.Error("expected no div when Done=false")
	}
}

func TestResolveTree_TextInterpolation(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<p>Count is {{Count}}</p>
	</body></html>`

	templates, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testCounter{Count: 7}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := vdom.ResolveTree(templates, ctx)

	if !findNodeText(nodes, "Count is 7") {
		t.Error("expected interpolated text 'Count is 7'")
	}
}

func TestResolveTree_GShow(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-show="Done">hidden when false</div>
	</body></html>`

	templates, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Done = false → display: none
	state := &testTodo{Done: false}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := vdom.ResolveTree(templates, ctx)
	div := findElement(nodes, "div")
	if div == nil {
		t.Fatal("expected div element (g-show keeps element in DOM)")
	}
	if div.Facts.Styles == nil || div.Facts.Styles["display"] != "none" {
		t.Error("expected display:none when Done=false")
	}

	// Done = true → no display:none
	state2 := &testTodo{Done: true}
	ctx2 := &vdom.ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	nodes2 := vdom.ResolveTree(templates, ctx2)
	div2 := findElement(nodes2, "div")
	if div2 == nil {
		t.Fatal("expected div element")
	}
	if div2.Facts.Styles != nil && div2.Facts.Styles["display"] == "none" {
		t.Error("expected no display:none when Done=true")
	}
}

func TestResolveTree_GClass(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div class="base" g-class:active="Done">text</div>
	</body></html>`

	templates, err := vdom.ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Done = true → class should include "active"
	state := &testTodo{Done: true}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := vdom.ResolveTree(templates, ctx)
	div := findElement(nodes, "div")
	if div == nil {
		t.Fatal("expected div")
	}
	className, _ := div.Facts.Props["className"].(string)
	if !strings.Contains(className, "active") {
		t.Errorf("expected className to contain 'active', got %q", className)
	}
	if !strings.Contains(className, "base") {
		t.Errorf("expected className to contain 'base', got %q", className)
	}

	// Done = false → class should NOT include "active"
	state2 := &testTodo{Done: false}
	ctx2 := &vdom.ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	nodes2 := vdom.ResolveTree(templates, ctx2)
	div2 := findElement(nodes2, "div")
	className2, _ := div2.Facts.Props["className"].(string)
	if strings.Contains(className2, "active") {
		t.Errorf("expected className without 'active', got %q", className2)
	}
}

// ---------------------------------------------------------------------------
// renderToHTML tests
// ---------------------------------------------------------------------------

func TestRenderToHTML_Simple(t *testing.T) {
	nodes := []vdom.Node{
		&vdom.ElementNode{
			Tag: "div",
			Facts: vdom.Facts{
				Props: map[string]any{"className": "main"},
			},
			Children: []vdom.Node{
				&vdom.TextNode{Text: "Hello"},
			},
		},
	}

	gid := &gidCounter{}
	html := renderToHTML(nodes, gid)

	if !strings.Contains(html, `class="main"`) {
		t.Errorf("expected class attribute, got %q", html)
	}
	if !strings.Contains(html, "Hello") {
		t.Errorf("expected text content, got %q", html)
	}
	if !strings.Contains(html, "</div>") {
		t.Errorf("expected closing div, got %q", html)
	}
}

func TestRenderToHTML_VoidElement(t *testing.T) {
	nodes := []vdom.Node{
		&vdom.ElementNode{
			Tag: "input",
			Facts: vdom.Facts{
				Props: map[string]any{"value": "test"},
			},
		},
	}

	gid := &gidCounter{}
	html := renderToHTML(nodes, gid)

	if strings.Contains(html, "</input>") {
		t.Errorf("input should not have closing tag, got %q", html)
	}
}

func TestRenderToHTML_Styles(t *testing.T) {
	nodes := []vdom.Node{
		&vdom.ElementNode{
			Tag: "div",
			Facts: vdom.Facts{
				Styles: map[string]string{
					"display": "none",
					"width":   "100px",
				},
			},
		},
	}

	gid := &gidCounter{}
	html := renderToHTML(nodes, gid)

	if !strings.Contains(html, `style="`) {
		t.Errorf("expected style attribute, got %q", html)
	}
	if !strings.Contains(html, "display: none") {
		t.Errorf("expected display:none in style, got %q", html)
	}
}

func TestRenderToHTML_EndToEnd(t *testing.T) {
	// Parse → resolve → render round-trip
	htmlInput := `<!DOCTYPE html><html><head></head><body>
		<h1><span g-text="Count">0</span></h1>
		<button g-click="Increment">+</button>
		<input g-bind="Name"/>
	</body></html>`

	templates, err := vdom.ParseTemplate(htmlInput, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testCounter{Count: 99, Name: "Alice"}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := vdom.ResolveTree(templates, ctx)

	gid := &gidCounter{}
	output := renderToHTML(nodes, gid)

	if !strings.Contains(output, "99") {
		t.Errorf("expected '99' in output, got %q", output)
	}
	if !strings.Contains(output, `value="Alice"`) {
		t.Errorf("expected value='Alice' in output, got %q", output)
	}
}

// ---------------------------------------------------------------------------
// Node type tests
// ---------------------------------------------------------------------------

func TestDescendantsCount(t *testing.T) {
	tree := &vdom.ElementNode{
		Tag: "div",
		Children: []vdom.Node{
			&vdom.TextNode{Text: "hello"},
			&vdom.ElementNode{
				Tag: "span",
				Children: []vdom.Node{
					&vdom.TextNode{Text: "world"},
				},
			},
		},
	}

	count := vdom.ComputeDescendants(tree)
	// div has 2 direct children + span has 1 = 3
	if count != 3 {
		t.Errorf("expected 3 descendants, got %d", count)
	}
	if tree.Descendants != 3 {
		t.Errorf("expected cached descendants = 3, got %d", tree.Descendants)
	}
}

// ---------------------------------------------------------------------------
// parseTextInterpolations tests
// ---------------------------------------------------------------------------

func TestParseTextInterpolations(t *testing.T) {
	tests := []struct {
		input string
		want  []vdom.TextPart
	}{
		{
			"plain text",
			[]vdom.TextPart{{Static: true, Value: "plain text"}},
		},
		{
			"{{Name}}",
			[]vdom.TextPart{{Static: false, Value: "Name"}},
		},
		{
			"Hello {{Name}}!",
			[]vdom.TextPart{
				{Static: true, Value: "Hello "},
				{Static: false, Value: "Name"},
				{Static: true, Value: "!"},
			},
		},
		{
			"A {{B}} C {{D}} E",
			[]vdom.TextPart{
				{Static: true, Value: "A "},
				{Static: false, Value: "B"},
				{Static: true, Value: " C "},
				{Static: false, Value: "D"},
				{Static: true, Value: " E"},
			},
		},
		{
			"single {brace} not interpolated",
			[]vdom.TextPart{{Static: true, Value: "single {brace} not interpolated"}},
		},
	}

	for _, tt := range tests {
		got := vdom.ParseTextInterpolations(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("ParseTextInterpolations(%q): got %d parts, want %d\n  got: %+v", tt.input, len(got), len(tt.want), got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParseTextInterpolations(%q)[%d]: got %+v, want %+v", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// parseForExpr tests
// ---------------------------------------------------------------------------

func TestParseForExpr(t *testing.T) {
	tests := []struct {
		input                      string
		wantItem, wantIndex, wantList string
	}{
		{"todo in Todos", "todo", "", "Todos"},
		{"todo, i in Todos", "todo", "i", "Todos"},
		{"item in Items", "item", "", "Items"},
		{"opt, j in group.Options", "opt", "j", "group.Options"},
	}

	for _, tt := range tests {
		item, index, list := vdom.ParseForExpr(tt.input)
		if item != tt.wantItem || index != tt.wantIndex || list != tt.wantList {
			t.Errorf("ParseForExpr(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, item, index, list, tt.wantItem, tt.wantIndex, tt.wantList)
		}
	}
}

// ---------------------------------------------------------------------------
// mergeAdjacentText tests
// ---------------------------------------------------------------------------

func TestMergeAdjacentText(t *testing.T) {
	nodes := []vdom.Node{
		&vdom.TextNode{Text: "a"},
		&vdom.TextNode{Text: "b"},
		&vdom.TextNode{Text: "c"},
	}
	merged := vdom.MergeAdjacentText(nodes)
	if len(merged) != 1 {
		t.Fatalf("expected 1 node, got %d", len(merged))
	}
	if merged[0].(*vdom.TextNode).Text != "abc" {
		t.Errorf("expected 'abc', got %q", merged[0].(*vdom.TextNode).Text)
	}
}

func TestMergeAdjacentText_InterleaveElements(t *testing.T) {
	nodes := []vdom.Node{
		&vdom.TextNode{Text: "a"},
		&vdom.ElementNode{Tag: "div"},
		&vdom.TextNode{Text: "b"},
		&vdom.TextNode{Text: "c"},
	}
	merged := vdom.MergeAdjacentText(nodes)
	if len(merged) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(merged))
	}
	if merged[0].(*vdom.TextNode).Text != "a" {
		t.Errorf("first text should be 'a', got %q", merged[0].(*vdom.TextNode).Text)
	}
	if merged[2].(*vdom.TextNode).Text != "bc" {
		t.Errorf("last text should be 'bc', got %q", merged[2].(*vdom.TextNode).Text)
	}
}

func TestResolveTree_EmptyForMergesAdjacentWhitespace(t *testing.T) {
	// Simulates: <div>\n  <g-for (empty list)>\n</div>
	// The whitespace before and after the empty g-for should merge into one TextNode.
	templates := []*vdom.TemplateNode{
		{IsText: true, TextParts: []vdom.TextPart{{Static: true, Value: "\n  "}}},
		{IsFor: true, ForItem: "x", ForList: "Items"},
		{IsText: true, TextParts: []vdom.TextPart{{Static: true, Value: "\n"}}},
	}

	type comp struct {
		Component
		Items []string
	}
	c := &comp{Items: nil}
	ctx := &vdom.ResolveContext{
		State: reflect.ValueOf(c),
		Vars:  make(map[string]any),
	}

	nodes := vdom.ResolveTree(templates, ctx)
	// Two whitespace text nodes should be merged into one
	if len(nodes) != 1 {
		t.Fatalf("expected 1 merged text node, got %d nodes", len(nodes))
	}
	tn, ok := nodes[0].(*vdom.TextNode)
	if !ok {
		t.Fatal("expected a TextNode")
	}
	if tn.Text != "\n  \n" {
		t.Errorf("expected merged text '\\n  \\n', got %q", tn.Text)
	}
}

func TestMergeAdjacentText_DropsEmptyText(t *testing.T) {
	nodes := []vdom.Node{
		&vdom.TextNode{Text: "a"},
		&vdom.TextNode{Text: ""},
		&vdom.ElementNode{Tag: "div"},
		&vdom.TextNode{Text: ""},
		&vdom.TextNode{Text: "b"},
	}
	merged := vdom.MergeAdjacentText(nodes)
	// Empty text nodes should be dropped; "a" stands alone, "b" stands alone
	if len(merged) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(merged))
	}
	if merged[0].(*vdom.TextNode).Text != "a" {
		t.Errorf("first = %q, want 'a'", merged[0].(*vdom.TextNode).Text)
	}
	if merged[2].(*vdom.TextNode).Text != "b" {
		t.Errorf("last = %q, want 'b'", merged[2].(*vdom.TextNode).Text)
	}
}

func TestResolveElementNode_GTextEmpty(t *testing.T) {
	// When g-text resolves to "", the element should have no children
	// because the browser creates no DOM text node for empty innerHTML.
	tmpl := &vdom.TemplateNode{
		Tag:        "div",
		Directives: []vdom.Directive{{Type: "text", Expr: "Name"}},
	}
	type comp struct {
		Component
		Name string
	}
	c := &comp{Name: ""}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(c), Vars: make(map[string]any)}
	nodes := vdom.ResolveTemplateNode(tmpl, ctx)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 element, got %d", len(nodes))
	}
	el := nodes[0].(*vdom.ElementNode)
	if len(el.Children) != 0 {
		t.Errorf("expected 0 children for empty g-text, got %d", len(el.Children))
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func findTemplateTag(nodes []*vdom.TemplateNode, tag string) *vdom.TemplateNode {
	for _, n := range nodes {
		if n.Tag == tag {
			return n
		}
		if found := findTemplateTag(n.Children, tag); found != nil {
			return found
		}
	}
	return nil
}

func findTemplateTagWithDirective(nodes []*vdom.TemplateNode, tag, dirType string) *vdom.TemplateNode {
	for _, n := range nodes {
		if n.Tag == tag {
			for _, d := range n.Directives {
				if d.Type == dirType {
					return n
				}
			}
		}
		if found := findTemplateTagWithDirective(n.Children, tag, dirType); found != nil {
			return found
		}
	}
	return nil
}

func findElement(nodes []vdom.Node, tag string) *vdom.ElementNode {
	for _, n := range nodes {
		switch n := n.(type) {
		case *vdom.ElementNode:
			if n.Tag == tag {
				return n
			}
			if found := findElement(n.Children, tag); found != nil {
				return found
			}
		}
	}
	return nil
}

func findNodeText(nodes []vdom.Node, text string) bool {
	for _, n := range nodes {
		switch n := n.(type) {
		case *vdom.TextNode:
			if strings.Contains(n.Text, text) {
				return true
			}
		case *vdom.ElementNode:
			if findNodeText(n.Children, text) {
				return true
			}
		case *vdom.KeyedElementNode:
			for _, kc := range n.Children {
				if findNodeText([]vdom.Node{kc.Node}, text) {
					return true
				}
			}
		}
	}
	return false
}
