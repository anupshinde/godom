package vdom

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseTemplate tests
// ---------------------------------------------------------------------------

func TestParseTemplate_SimpleText(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>Hello World</body></html>`
	nodes, err := ParseTemplate(html, nil)
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
	nodes, err := ParseTemplate(html, nil)
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

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		tag     string
		dirType string
		name    string // expected Name field (for class/style/attr)
		expr    string // expected Expr field
	}{
		{"span", "text", "", "Name"},
		{"input", "bind", "", "Email"},
		{"div", "if", "", "ShowPanel"},
		{"div", "show", "", "Visible"},
		{"button", "click", "", "Save"},
		{"div", "class", "active", "IsActive"},
		{"div", "style", "width", "Width"},
		{"div", "attr", "transform", "Transform"},
	}

	for _, tt := range tests {
		node := findTemplateTagWithDirective(nodes, tt.tag, tt.dirType)
		if node == nil {
			t.Errorf("expected %s element with %s directive", tt.tag, tt.dirType)
			continue
		}
		for _, d := range node.Directives {
			if d.Type == tt.dirType {
				if tt.name != "" && d.Name != tt.name {
					t.Errorf("directive %s: expected Name=%q, got %q", tt.dirType, tt.name, d.Name)
				}
				if d.Expr != tt.expr {
					t.Errorf("directive %s: expected Expr=%q, got %q", tt.dirType, tt.expr, d.Expr)
				}
				break
			}
		}
	}
}

func TestParseTemplate_Directives_Negative(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div id="main" class="box" data-x="1" g-text="Name" g-click="Save">text</div>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	div := findTemplateTag(nodes, "div")
	if div == nil {
		t.Fatal("expected div")
	}

	// Plain attrs (id, class, data-x) must NOT appear in directives.
	for _, d := range div.Directives {
		if d.Type == "id" || d.Type == "class" || d.Type == "data-x" {
			t.Errorf("plain attr %q leaked into directives", d.Type)
		}
	}

	// Directive attrs (g-text, g-click) must NOT appear in plain attrs.
	for _, a := range div.Attrs {
		if a.Key == "g-text" || a.Key == "g-click" {
			t.Errorf("directive %q leaked into plain attrs", a.Key)
		}
	}

	// Verify the plain attrs ARE present.
	attrKeys := make(map[string]string)
	for _, a := range div.Attrs {
		attrKeys[a.Key] = a.Val
	}
	if attrKeys["id"] != "main" {
		t.Errorf("expected id='main' in attrs, got %q", attrKeys["id"])
	}
	if attrKeys["class"] != "box" {
		t.Errorf("expected class='box' in attrs, got %q", attrKeys["class"])
	}
	if attrKeys["data-x"] != "1" {
		t.Errorf("expected data-x='1' in attrs, got %q", attrKeys["data-x"])
	}

	// Verify the directives ARE present.
	dirTypes := make(map[string]bool)
	for _, d := range div.Directives {
		dirTypes[d.Type] = true
	}
	if !dirTypes["text"] {
		t.Error("expected 'text' directive")
	}
	if !dirTypes["click"] {
		t.Error("expected 'click' directive")
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

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	ul := findTemplateTag(nodes, "ul")
	if ul == nil {
		t.Fatal("expected ul element")
	}

	// Find the g-for node inside ul
	var forNode *TemplateNode
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

	nodes, err := ParseTemplate(html, nil)
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
	nodes, err := ParseTemplate(html, comps)
	if err != nil {
		t.Fatal(err)
	}

	var comp *TemplateNode
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

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testCounter{Count: 42, Step: 1, Name: "test"}
	ctx := &ResolveContext{
		State: reflect.ValueOf(state),
		Vars:  make(map[string]any),
	}

	nodes := ResolveTree(templates, ctx)

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

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testTodoApp{
		Todos: []testTodo{
			{ID: 1, Text: "Buy milk", Done: false},
			{ID: 2, Text: "Write code", Done: true},
		},
	}
	ctx := &ResolveContext{
		State: reflect.ValueOf(state),
		Vars:  make(map[string]any),
	}

	nodes := ResolveTree(templates, ctx)

	// The ul should have 2 li children (expanded from g-for)
	ul := findElement(nodes, "ul")
	if ul == nil {
		t.Fatal("expected ul element")
	}
	liCount := 0
	for _, c := range ul.Children {
		if el, ok := c.(*ElementNode); ok && el.Tag == "li" {
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

func TestResolveTree_GForWithIndex(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<ol>
			<li g-for="todo, i in Todos">
				<span g-text="i"></span>: <span g-text="todo.Text"></span>
			</li>
		</ol>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testTodoApp{
		Todos: []testTodo{
			{ID: 1, Text: "First"},
			{ID: 2, Text: "Second"},
			{ID: 3, Text: "Third"},
		},
	}
	ctx := &ResolveContext{
		State: reflect.ValueOf(state),
		Vars:  make(map[string]any),
	}

	nodes := ResolveTree(templates, ctx)

	ol := findElement(nodes, "ol")
	if ol == nil {
		t.Fatal("expected ol element")
	}

	// Should have 3 li children
	liCount := 0
	for _, c := range ol.Children {
		if el, ok := c.(*ElementNode); ok && el.Tag == "li" {
			liCount++
		}
	}
	if liCount != 3 {
		t.Errorf("expected 3 li elements, got %d", liCount)
	}

	// Index values should resolve: 0, 1, 2
	if !findNodeText(nodes, "0") {
		t.Error("expected index '0' in output")
	}
	if !findNodeText(nodes, "2") {
		t.Error("expected index '2' in output")
	}

	// Item fields should resolve
	if !findNodeText(nodes, "First") {
		t.Error("expected 'First' in output")
	}
	if !findNodeText(nodes, "Third") {
		t.Error("expected 'Third' in output")
	}
}

func TestResolveTree_GForEmptyList(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<ul>
			<li g-for="todo in Todos">
				<span g-text="todo.Text"></span>
			</li>
		</ul>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testTodoApp{Todos: nil}
	ctx := &ResolveContext{
		State: reflect.ValueOf(state),
		Vars:  make(map[string]any),
	}

	nodes := ResolveTree(templates, ctx)
	ul := findElement(nodes, "ul")
	if ul == nil {
		t.Fatal("expected ul element")
	}

	// No li children should be generated
	for _, c := range ul.Children {
		if el, ok := c.(*ElementNode); ok && el.Tag == "li" {
			t.Error("expected no li elements for empty list")
		}
	}
}

func TestResolveTree_GFor_Negative(t *testing.T) {
	t.Run("non-slice field produces no items", func(t *testing.T) {
		// g-for referencing a string field (not a slice) should produce nothing.
		html := `<!DOCTYPE html><html><head></head><body>
			<ul><li g-for="x in Text"><span g-text="x"></span></li></ul>
		</body></html>`
		templates, err := ParseTemplate(html, nil)
		if err != nil {
			t.Fatal(err)
		}
		state := &testTodo{Text: "hello"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		ul := findElement(nodes, "ul")
		if ul == nil {
			t.Fatal("expected ul element")
		}
		for _, c := range ul.Children {
			if el, ok := c.(*ElementNode); ok && el.Tag == "li" {
				t.Error("expected no li elements when g-for references a non-slice field")
			}
		}
	})

	t.Run("missing field produces no items", func(t *testing.T) {
		// g-for referencing a nonexistent field should produce nothing.
		html := `<!DOCTYPE html><html><head></head><body>
			<ul><li g-for="x in NoSuchField"><span g-text="x"></span></li></ul>
		</body></html>`
		templates, err := ParseTemplate(html, nil)
		if err != nil {
			t.Fatal(err)
		}
		state := &testTodo{Text: "hello"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		ul := findElement(nodes, "ul")
		if ul == nil {
			t.Fatal("expected ul element")
		}
		for _, c := range ul.Children {
			if el, ok := c.(*ElementNode); ok && el.Tag == "li" {
				t.Error("expected no li elements when g-for references a missing field")
			}
		}
	})
}

func TestResolveTree_GIf(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-if="Done">completed</div>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Done = true → div should be present
	state := &testTodo{Done: true}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := ResolveTree(templates, ctx)
	if findElement(nodes, "div") == nil {
		t.Error("expected div when Done=true")
	}

	// Done = false → div should be absent
	state2 := &testTodo{Done: false}
	ctx2 := &ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	nodes2 := ResolveTree(templates, ctx2)
	if findElement(nodes2, "div") != nil {
		t.Error("expected no div when Done=false")
	}
}

func TestResolveTree_TextInterpolation(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<p>Count is {{Count}}</p>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testCounter{Count: 7}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := ResolveTree(templates, ctx)

	if !findNodeText(nodes, "Count is 7") {
		t.Error("expected interpolated text 'Count is 7'")
	}
}

func TestResolveTree_GShow(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-show="Done">hidden when false</div>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Done = false → display: none
	state := &testTodo{Done: false}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := ResolveTree(templates, ctx)
	div := findElement(nodes, "div")
	if div == nil {
		t.Fatal("expected div element (g-show keeps element in DOM)")
	}
	if div.Facts.Styles == nil || div.Facts.Styles["display"] != "none" {
		t.Error("expected display:none when Done=false")
	}

	// Done = true → no display:none
	state2 := &testTodo{Done: true}
	ctx2 := &ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	nodes2 := ResolveTree(templates, ctx2)
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

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Done = true → class should include "active"
	state := &testTodo{Done: true}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := ResolveTree(templates, ctx)
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
	ctx2 := &ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	nodes2 := ResolveTree(templates, ctx2)
	div2 := findElement(nodes2, "div")
	className2, _ := div2.Facts.Props["className"].(string)
	if strings.Contains(className2, "active") {
		t.Errorf("expected className without 'active', got %q", className2)
	}
}

func TestResolveTree_GClass_Negative(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div class="base" g-class:active="Done">text</div>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("g-class does not leak into attrs", func(t *testing.T) {
		state := &testTodo{Done: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		div := findElement(nodes, "div")
		if div == nil {
			t.Fatal("expected div")
		}
		// "active" should be in className (Props), not in Attrs.
		if div.Facts.Attrs != nil {
			if _, ok := div.Facts.Attrs["active"]; ok {
				t.Error("g-class 'active' leaked into Attrs")
			}
			if _, ok := div.Facts.Attrs["g-class:active"]; ok {
				t.Error("raw g-class:active directive leaked into Attrs")
			}
		}
	})

	t.Run("false class not in className", func(t *testing.T) {
		state := &testTodo{Done: false}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		div := findElement(nodes, "div")
		if div == nil {
			t.Fatal("expected div")
		}
		className, _ := div.Facts.Props["className"].(string)
		// Should have "base" only, not "active".
		if strings.Contains(className, "active") {
			t.Errorf("false g-class should not appear in className, got %q", className)
		}
		if className != "base" {
			t.Errorf("expected className='base', got %q", className)
		}
	})
}

type testMultiClass struct {
	IsActive   bool
	IsSelected bool
	IsDisabled bool
}

func TestResolveTree_GClassMultiple(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div class="btn" g-class:active="IsActive" g-class:selected="IsSelected" g-class:disabled="IsDisabled">text</div>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all true", func(t *testing.T) {
		state := &testMultiClass{IsActive: true, IsSelected: true, IsDisabled: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		div := findElement(nodes, "div")
		if div == nil {
			t.Fatal("expected div")
		}
		className, _ := div.Facts.Props["className"].(string)
		for _, cls := range []string{"btn", "active", "selected", "disabled"} {
			if !strings.Contains(className, cls) {
				t.Errorf("expected className to contain %q, got %q", cls, className)
			}
		}
	})

	t.Run("some true some false", func(t *testing.T) {
		state := &testMultiClass{IsActive: true, IsSelected: false, IsDisabled: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		div := findElement(nodes, "div")
		if div == nil {
			t.Fatal("expected div")
		}
		className, _ := div.Facts.Props["className"].(string)
		if !strings.Contains(className, "btn") {
			t.Errorf("expected 'btn' in className, got %q", className)
		}
		if !strings.Contains(className, "active") {
			t.Errorf("expected 'active' in className, got %q", className)
		}
		if strings.Contains(className, "selected") {
			t.Errorf("expected no 'selected' in className, got %q", className)
		}
		if !strings.Contains(className, "disabled") {
			t.Errorf("expected 'disabled' in className, got %q", className)
		}
	})

	t.Run("all false", func(t *testing.T) {
		state := &testMultiClass{IsActive: false, IsSelected: false, IsDisabled: false}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTree(templates, ctx)
		div := findElement(nodes, "div")
		if div == nil {
			t.Fatal("expected div")
		}
		className, _ := div.Facts.Props["className"].(string)
		if className != "btn" {
			t.Errorf("expected className='btn' only, got %q", className)
		}
	})
}

// ---------------------------------------------------------------------------
// Node type tests
// ---------------------------------------------------------------------------

func TestDescendantsCount(t *testing.T) {
	tree := &ElementNode{
		Tag: "div",
		Children: []Node{
			&TextNode{Text: "hello"},
			&ElementNode{
				Tag: "span",
				Children: []Node{
					&TextNode{Text: "world"},
				},
			},
		},
	}

	count := ComputeDescendants(tree)
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
		want  []TextPart
	}{
		{
			"plain text",
			[]TextPart{{Static: true, Value: "plain text"}},
		},
		{
			"{{Name}}",
			[]TextPart{{Static: false, Value: "Name"}},
		},
		{
			"Hello {{Name}}!",
			[]TextPart{
				{Static: true, Value: "Hello "},
				{Static: false, Value: "Name"},
				{Static: true, Value: "!"},
			},
		},
		{
			"A {{B}} C {{D}} E",
			[]TextPart{
				{Static: true, Value: "A "},
				{Static: false, Value: "B"},
				{Static: true, Value: " C "},
				{Static: false, Value: "D"},
				{Static: true, Value: " E"},
			},
		},
		{
			"single {brace} not interpolated",
			[]TextPart{{Static: true, Value: "single {brace} not interpolated"}},
		},
	}

	for _, tt := range tests {
		got := ParseTextInterpolations(tt.input)
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
		input                        string
		wantItem, wantIndex, wantList string
	}{
		{"todo in Todos", "todo", "", "Todos"},
		{"todo, i in Todos", "todo", "i", "Todos"},
		{"item in Items", "item", "", "Items"},
		{"opt, j in group.Options", "opt", "j", "group.Options"},
	}

	for _, tt := range tests {
		item, index, list := ParseForExpr(tt.input)
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
	nodes := []Node{
		&TextNode{Text: "a"},
		&TextNode{Text: "b"},
		&TextNode{Text: "c"},
	}
	merged := MergeAdjacentText(nodes)
	if len(merged) != 1 {
		t.Fatalf("expected 1 node, got %d", len(merged))
	}
	if merged[0].(*TextNode).Text != "abc" {
		t.Errorf("expected 'abc', got %q", merged[0].(*TextNode).Text)
	}
}

func TestMergeAdjacentText_InterleaveElements(t *testing.T) {
	nodes := []Node{
		&TextNode{Text: "a"},
		&ElementNode{Tag: "div"},
		&TextNode{Text: "b"},
		&TextNode{Text: "c"},
	}
	merged := MergeAdjacentText(nodes)
	if len(merged) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(merged))
	}
	if merged[0].(*TextNode).Text != "a" {
		t.Errorf("first text should be 'a', got %q", merged[0].(*TextNode).Text)
	}
	if merged[2].(*TextNode).Text != "bc" {
		t.Errorf("last text should be 'bc', got %q", merged[2].(*TextNode).Text)
	}
}

func TestResolveTree_EmptyForMergesAdjacentWhitespace(t *testing.T) {
	// Simulates: <div>\n  <g-for (empty list)>\n</div>
	// The whitespace before and after the empty g-for should merge into one TextNode.
	templates := []*TemplateNode{
		{IsText: true, TextParts: []TextPart{{Static: true, Value: "\n  "}}},
		{IsFor: true, ForItem: "x", ForList: "Items"},
		{IsText: true, TextParts: []TextPart{{Static: true, Value: "\n"}}},
	}

	type comp struct {
		Items []string
	}
	c := &comp{Items: nil}
	ctx := &ResolveContext{
		State: reflect.ValueOf(c),
		Vars:  make(map[string]any),
	}

	nodes := ResolveTree(templates, ctx)
	// Two whitespace text nodes should be merged into one
	if len(nodes) != 1 {
		t.Fatalf("expected 1 merged text node, got %d nodes", len(nodes))
	}
	tn, ok := nodes[0].(*TextNode)
	if !ok {
		t.Fatal("expected a TextNode")
	}
	if tn.Text != "\n  \n" {
		t.Errorf("expected merged text '\\n  \\n', got %q", tn.Text)
	}
}

func TestMergeAdjacentText_DropsEmptyText(t *testing.T) {
	nodes := []Node{
		&TextNode{Text: "a"},
		&TextNode{Text: ""},
		&ElementNode{Tag: "div"},
		&TextNode{Text: ""},
		&TextNode{Text: "b"},
	}
	merged := MergeAdjacentText(nodes)
	// Empty text nodes should be dropped; "a" stands alone, "b" stands alone
	if len(merged) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(merged))
	}
	if merged[0].(*TextNode).Text != "a" {
		t.Errorf("first = %q, want 'a'", merged[0].(*TextNode).Text)
	}
	if merged[2].(*TextNode).Text != "b" {
		t.Errorf("last = %q, want 'b'", merged[2].(*TextNode).Text)
	}
}

func TestResolveElementNode_GTextEmpty(t *testing.T) {
	// When g-text resolves to "", the element should have no children
	// because the browser creates no DOM text node for empty innerHTML.
	tmpl := &TemplateNode{
		Tag:        "div",
		Directives: []Directive{{Type: "text", Expr: "Name"}},
	}
	type comp struct {
		Name string
	}
	c := &comp{Name: ""}
	ctx := &ResolveContext{State: reflect.ValueOf(c), Vars: make(map[string]any)}
	nodes := ResolveTemplateNode(tmpl, ctx)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 element, got %d", len(nodes))
	}
	el := nodes[0].(*ElementNode)
	if len(el.Children) != 0 {
		t.Errorf("expected 0 children for empty g-text, got %d", len(el.Children))
	}
}

// ---------------------------------------------------------------------------
// Section 1: Directive resolution tests
// ---------------------------------------------------------------------------

type testDirectiveState struct {
	Name      string
	Email     string
	Width     string
	Transform string
	Done      bool
	Visible   bool
	Hidden    bool
	Count     int
	ChartData map[string]int
}

func (s *testDirectiveState) ComputedName() string {
	return "computed_" + s.Name
}

func (s *testDirectiveState) Add(a, b int) int {
	return a + b
}

func TestResolve_GHide(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "div",
		Directives: []Directive{{Type: "hide", Expr: "Hidden"}},
	}

	t.Run("truthy hides", func(t *testing.T) {
		state := &testDirectiveState{Hidden: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Styles == nil || el.Facts.Styles["display"] != "none" {
			t.Error("expected display:none when Hidden=true")
		}
	})

	t.Run("falsy does not hide", func(t *testing.T) {
		state := &testDirectiveState{Hidden: false}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Styles != nil && el.Facts.Styles["display"] == "none" {
			t.Error("expected no display:none when Hidden=false")
		}
	})
}

func TestResolve_GValue(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "input",
		Directives: []Directive{{Type: "value", Expr: "Name"}},
	}

	t.Run("resolves to Props value", func(t *testing.T) {
		state := &testDirectiveState{Name: "Alice"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props["value"] != "Alice" {
			t.Errorf("expected value='Alice', got %v", el.Facts.Props["value"])
		}
	})

	t.Run("does not leak into Attrs", func(t *testing.T) {
		state := &testDirectiveState{Name: "Alice"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Attrs != nil {
			if _, ok := el.Facts.Attrs["value"]; ok {
				t.Error("g-value leaked into Attrs")
			}
		}
	})
}

func TestResolve_GChecked(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "input",
		Directives: []Directive{{Type: "checked", Expr: "Done"}},
	}

	t.Run("truthy sets checked true", func(t *testing.T) {
		state := &testDirectiveState{Done: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props["checked"] != true {
			t.Errorf("expected checked=true, got %v", el.Facts.Props["checked"])
		}
	})

	t.Run("falsy sets checked false", func(t *testing.T) {
		state := &testDirectiveState{Done: false}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props["checked"] != false {
			t.Errorf("expected checked=false, got %v", el.Facts.Props["checked"])
		}
	})

	t.Run("value is bool not string", func(t *testing.T) {
		state := &testDirectiveState{Done: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if _, ok := el.Facts.Props["checked"].(bool); !ok {
			t.Errorf("expected checked to be bool, got %T", el.Facts.Props["checked"])
		}
	})

	t.Run("does not leak into Attrs", func(t *testing.T) {
		state := &testDirectiveState{Done: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Attrs != nil {
			if _, ok := el.Facts.Attrs["checked"]; ok {
				t.Error("g-checked leaked into Attrs")
			}
		}
	})
}

func TestResolve_GBind(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "input",
		Directives: []Directive{{Type: "bind", Expr: "Email"}},
	}

	t.Run("resolves to Props value as string", func(t *testing.T) {
		state := &testDirectiveState{Email: "a@b.com"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props["value"] != "a@b.com" {
			t.Errorf("expected value='a@b.com', got %v", el.Facts.Props["value"])
		}
	})

	t.Run("does not set checked", func(t *testing.T) {
		state := &testDirectiveState{Email: "a@b.com"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if _, ok := el.Facts.Props["checked"]; ok {
			t.Error("g-bind should not set checked")
		}
	})
}

func TestResolve_GStyle(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "div",
		Directives: []Directive{{Type: "style", Name: "width", Expr: "Width"}},
	}

	t.Run("resolves to Styles", func(t *testing.T) {
		state := &testDirectiveState{Width: "200px"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Styles["width"] != "200px" {
			t.Errorf("expected width='200px', got %q", el.Facts.Styles["width"])
		}
	})

	t.Run("does not leak into Props or Attrs", func(t *testing.T) {
		state := &testDirectiveState{Width: "200px"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props != nil {
			if _, ok := el.Facts.Props["width"]; ok {
				t.Error("g-style leaked into Props")
			}
		}
		if el.Facts.Attrs != nil {
			if _, ok := el.Facts.Attrs["width"]; ok {
				t.Error("g-style leaked into Attrs")
			}
		}
	})
}

func TestResolve_GAttr(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "div",
		Directives: []Directive{{Type: "attr", Name: "transform", Expr: "Transform"}},
	}

	t.Run("resolves to Attrs", func(t *testing.T) {
		state := &testDirectiveState{Transform: "rotate(45)"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Attrs["transform"] != "rotate(45)" {
			t.Errorf("expected transform='rotate(45)', got %q", el.Facts.Attrs["transform"])
		}
	})

	t.Run("does not leak into Props or Styles", func(t *testing.T) {
		state := &testDirectiveState{Transform: "rotate(45)"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props != nil {
			if _, ok := el.Facts.Props["transform"]; ok {
				t.Error("g-attr leaked into Props")
			}
		}
		if el.Facts.Styles != nil {
			if _, ok := el.Facts.Styles["transform"]; ok {
				t.Error("g-attr leaked into Styles")
			}
		}
	})
}

func TestResolve_GKeydown(t *testing.T) {
	t.Run("no key filter", func(t *testing.T) {
		tmpl := &TemplateNode{
			Tag:        "input",
			Directives: []Directive{{Type: "keydown", Expr: "Save"}},
		}
		state := &testDirectiveState{}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		ev, ok := el.Facts.Events["keydown"]
		if !ok {
			t.Fatal("expected keydown event")
		}
		if ev.Handler != "Save" {
			t.Errorf("expected handler 'Save', got %q", ev.Handler)
		}
		if ev.Options.Key != "" {
			t.Errorf("expected no key filter, got %q", ev.Options.Key)
		}
	})

	t.Run("with key filter", func(t *testing.T) {
		tmpl := &TemplateNode{
			Tag:        "input",
			Directives: []Directive{{Type: "keydown", Name: "Enter", Expr: "Save"}},
		}
		state := &testDirectiveState{}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		ev, ok := el.Facts.Events["keydown:Enter"]
		if !ok {
			t.Fatal("expected keydown:Enter event")
		}
		if ev.Handler != "Save" {
			t.Errorf("expected handler 'Save', got %q", ev.Handler)
		}
		if ev.Options.Key != "Enter" {
			t.Errorf("expected key filter 'Enter', got %q", ev.Options.Key)
		}
		// Should NOT have a plain keydown entry
		if _, ok := el.Facts.Events["keydown"]; ok {
			t.Error("expected no plain 'keydown' event, only 'keydown:Enter'")
		}
	})

	t.Run("multi-handler semicolon", func(t *testing.T) {
		// Parsed from HTML: g-keydown="Enter:Save;Escape:Cancel"
		// extractAttrsAndDirectives splits into two directives
		tmpl := &TemplateNode{
			Tag: "input",
			Directives: []Directive{
				{Type: "keydown", Name: "Enter", Expr: "Save"},
				{Type: "keydown", Name: "Escape", Expr: "Cancel"},
			},
		}
		state := &testDirectiveState{}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)

		ev1, ok1 := el.Facts.Events["keydown:Enter"]
		if !ok1 || ev1.Handler != "Save" {
			t.Errorf("expected keydown:Enter → Save, got %+v", el.Facts.Events)
		}
		ev2, ok2 := el.Facts.Events["keydown:Escape"]
		if !ok2 || ev2.Handler != "Cancel" {
			t.Errorf("expected keydown:Escape → Cancel, got %+v", el.Facts.Events)
		}
		if len(el.Facts.Events) != 2 {
			t.Errorf("expected exactly 2 events, got %d", len(el.Facts.Events))
		}
	})
}

func TestResolve_MouseEvents(t *testing.T) {
	eventTypes := []string{"mousedown", "mousemove", "mouseup", "wheel", "drop"}

	for _, evType := range eventTypes {
		t.Run(evType, func(t *testing.T) {
			tmpl := &TemplateNode{
				Tag:        "div",
				Directives: []Directive{{Type: evType, Expr: "Handle"}},
			}
			state := &testDirectiveState{}
			ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
			nodes := ResolveTemplateNode(tmpl, ctx)
			el := nodes[0].(*ElementNode)

			ev, ok := el.Facts.Events[evType]
			if !ok {
				t.Fatalf("expected %s event", evType)
			}
			if ev.Handler != "Handle" {
				t.Errorf("expected handler 'Handle', got %q", ev.Handler)
			}
			// Should not create other event types
			if len(el.Facts.Events) != 1 {
				t.Errorf("expected 1 event, got %d: %v", len(el.Facts.Events), el.Facts.Events)
			}
		})
	}
}

func TestResolve_GDraggable(t *testing.T) {
	t.Run("no group", func(t *testing.T) {
		tmpl := &TemplateNode{
			Tag:        "div",
			Directives: []Directive{{Type: "draggable", Expr: "Name"}},
		}
		state := &testDirectiveState{Name: "item-1"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props["draggable"] != true {
			t.Error("expected draggable=true in Props")
		}
		if el.Facts.Attrs["data-drag-value"] != "item-1" {
			t.Errorf("expected data-drag-value='item-1', got %q", el.Facts.Attrs["data-drag-value"])
		}
	})

	t.Run("with group", func(t *testing.T) {
		tmpl := &TemplateNode{
			Tag:        "div",
			Directives: []Directive{{Type: "draggable", Name: "cards", Expr: "Name"}},
		}
		state := &testDirectiveState{Name: "card-1"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if el.Facts.Props["draggable"] != true {
			t.Error("expected draggable=true")
		}
		if el.Facts.Attrs["data-drag-value"] != "card-1" {
			t.Errorf("expected data-drag-value='card-1', got %q", el.Facts.Attrs["data-drag-value"])
		}
	})

	t.Run("no event handler from draggable alone", func(t *testing.T) {
		tmpl := &TemplateNode{
			Tag:        "div",
			Directives: []Directive{{Type: "draggable", Expr: "Name"}},
		}
		state := &testDirectiveState{Name: "x"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		el := nodes[0].(*ElementNode)
		if len(el.Facts.Events) != 0 {
			t.Errorf("expected no events from draggable alone, got %v", el.Facts.Events)
		}
	})
}

func TestResolve_GDropzone(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "div",
		Directives: []Directive{{Type: "dropzone", Expr: "HandleDrop"}},
	}
	state := &testDirectiveState{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := ResolveTemplateNode(tmpl, ctx)
	el := nodes[0].(*ElementNode)

	ev, ok := el.Facts.Events["drop"]
	if !ok {
		t.Fatal("expected drop event from g-dropzone")
	}
	if ev.Handler != "HandleDrop" {
		t.Errorf("expected handler 'HandleDrop', got %q", ev.Handler)
	}
	// Should NOT set draggable
	if el.Facts.Props != nil {
		if _, ok := el.Facts.Props["draggable"]; ok {
			t.Error("g-dropzone should not set draggable prop")
		}
	}
}

func TestResolve_GPlugin(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "div",
		IsPlugin:   true,
		PluginName: "chart",
		PluginExpr: "ChartData",
	}

	t.Run("produces PluginNode", func(t *testing.T) {
		state := &testDirectiveState{ChartData: map[string]int{"a": 1}}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		if len(nodes) != 1 {
			t.Fatalf("expected 1 node, got %d", len(nodes))
		}
		pn, ok := nodes[0].(*PluginNode)
		if !ok {
			t.Fatalf("expected PluginNode, got %T", nodes[0])
		}
		if pn.Name != "chart" {
			t.Errorf("expected Name='chart', got %q", pn.Name)
		}
		if pn.Tag != "div" {
			t.Errorf("expected Tag='div', got %q", pn.Tag)
		}
		if pn.Data == nil {
			t.Error("expected Data to be non-nil")
		}
	})

	t.Run("non-plugin not parsed as PluginNode", func(t *testing.T) {
		plainTmpl := &TemplateNode{Tag: "div"}
		state := &testDirectiveState{}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(plainTmpl, ctx)
		if _, ok := nodes[0].(*PluginNode); ok {
			t.Error("plain element should not be a PluginNode")
		}
	})
}

func TestResolve_GIfNegation(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:        "div",
		Directives: []Directive{{Type: "if", Expr: "!Done"}},
		Children:   []*TemplateNode{{IsText: true, TextParts: []TextPart{{Static: true, Value: "visible"}}}},
	}

	t.Run("Done=true negated to false, element absent", func(t *testing.T) {
		state := &testDirectiveState{Done: true}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		if len(nodes) != 0 {
			t.Errorf("expected 0 nodes when !Done with Done=true, got %d", len(nodes))
		}
	})

	t.Run("Done=false negated to true, element present", func(t *testing.T) {
		state := &testDirectiveState{Done: false}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		if len(nodes) != 1 {
			t.Fatalf("expected 1 node when !Done with Done=false, got %d", len(nodes))
		}
	})
}

func TestResolve_ComponentNode(t *testing.T) {
	tmpl := &TemplateNode{
		Tag:          "todo-item",
		IsComponent:  true,
		ComponentTag: "todo-item",
		PropExprs:    map[string]string{"text": "Name", "count": "Count"},
	}

	t.Run("props resolve from expressions", func(t *testing.T) {
		state := &testDirectiveState{Name: "hello", Count: 42}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		if len(nodes) != 1 {
			t.Fatalf("expected 1 node, got %d", len(nodes))
		}
		cn, ok := nodes[0].(*ComponentNode)
		if !ok {
			t.Fatalf("expected ComponentNode, got %T", nodes[0])
		}
		if cn.Tag != "todo-item" {
			t.Errorf("expected tag 'todo-item', got %q", cn.Tag)
		}
		if cn.Props["text"] != "hello" {
			t.Errorf("expected text='hello', got %v", cn.Props["text"])
		}
		if cn.Props["count"] != 42 {
			t.Errorf("expected count=42, got %v", cn.Props["count"])
		}
	})

	t.Run("SubTree is nil at resolve time", func(t *testing.T) {
		state := &testDirectiveState{Name: "x"}
		ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
		nodes := ResolveTemplateNode(tmpl, ctx)
		cn := nodes[0].(*ComponentNode)
		if cn.SubTree != nil {
			t.Error("expected SubTree to be nil at resolve time")
		}
	})
}

// ---------------------------------------------------------------------------
// Section 2: Expression engine tests
// ---------------------------------------------------------------------------

func TestResolveExpr_BooleanLiterals(t *testing.T) {
	state := &testDirectiveState{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}

	if ResolveExpr("true", ctx) != true {
		t.Error("expected 'true' → true")
	}
	if ResolveExpr("false", ctx) != false {
		t.Error("expected 'false' → false")
	}
	// "True" is not a bool literal — resolved as field (returns nil)
	if val := ResolveExpr("True", ctx); val == true {
		t.Errorf("'True' should not be a bool literal, got %v", val)
	}
}

func TestResolveExpr_Negation(t *testing.T) {
	state := &testDirectiveState{Done: true}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}

	val := ResolveExpr("!Done", ctx)
	if val != false {
		t.Errorf("expected !Done (true) → false, got %v", val)
	}

	state2 := &testDirectiveState{Done: false}
	ctx2 := &ResolveContext{State: reflect.ValueOf(state2), Vars: make(map[string]any)}
	val2 := ResolveExpr("!Done", ctx2)
	if val2 != true {
		t.Errorf("expected !Done (false) → true, got %v", val2)
	}

	// !MissingField — missing resolves to nil, !nil should be true
	val3 := ResolveExpr("!MissingField", ctx)
	if val3 != true {
		t.Errorf("expected !MissingField → true, got %v", val3)
	}
}

type testNested struct {
	Inner testInner
}

type testInner struct {
	Value string
}

func TestResolveExpr_DottedFieldPath(t *testing.T) {
	state := &testNested{Inner: testInner{Value: "deep"}}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}

	val := ResolveExpr("Inner.Value", ctx)
	if val != "deep" {
		t.Errorf("expected 'deep', got %v", val)
	}

	// Missing nested field
	val2 := ResolveExpr("Inner.Missing", ctx)
	if val2 != nil {
		t.Errorf("expected nil for missing nested field, got %v", val2)
	}

	// Missing root
	val3 := ResolveExpr("Missing.Field", ctx)
	if val3 != nil {
		t.Errorf("expected nil for missing root, got %v", val3)
	}
}

func TestResolveExpr_ZeroArgMethod(t *testing.T) {
	state := &testDirectiveState{Name: "test"}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}

	val := ResolveExpr("ComputedName", ctx)
	if val != "computed_test" {
		t.Errorf("expected 'computed_test', got %v", val)
	}

	// Non-existent method returns nil
	val2 := ResolveExpr("NoSuchMethod", ctx)
	if val2 != nil {
		t.Errorf("expected nil for missing method, got %v", val2)
	}
}

func TestResolveExpr_MethodWithArgs(t *testing.T) {
	state := &testDirectiveState{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}

	// Add(3, 4) should work via ParseMethodCall + callMethodWithArgs
	// But Add takes int args and ResolveExpr for "3" returns nil (not a field).
	// So we need loop vars or fields. Let's use Vars.
	ctx.Vars["a"] = 3
	ctx.Vars["b"] = 4
	val := ResolveExpr("Add(a, b)", ctx)
	if val != 7 {
		t.Errorf("expected Add(3,4)=7, got %v", val)
	}
}

func TestParseMethodCall_Cases(t *testing.T) {
	tests := []struct {
		input      string
		wantMethod string
		wantArgs   []string
	}{
		{"Save", "Save", nil},
		{"Save()", "Save", nil},
		{"Toggle(i)", "Toggle", []string{"i"}},
		{"Remove(i, todo.ID)", "Remove", []string{"i", "todo.ID"}},
	}

	for _, tt := range tests {
		method, args := ParseMethodCall(tt.input)
		if method != tt.wantMethod {
			t.Errorf("ParseMethodCall(%q): method=%q, want %q", tt.input, method, tt.wantMethod)
		}
		if len(args) != len(tt.wantArgs) {
			t.Errorf("ParseMethodCall(%q): args=%v, want %v", tt.input, args, tt.wantArgs)
			continue
		}
		for i := range args {
			if args[i] != tt.wantArgs[i] {
				t.Errorf("ParseMethodCall(%q): args[%d]=%q, want %q", tt.input, i, args[i], tt.wantArgs[i])
			}
		}
	}
}

func TestIsTruthy(t *testing.T) {
	truthy := []struct {
		name string
		val  any
	}{
		{"true", true},
		{"int 1", 1},
		{"int64 1", int64(1)},
		{"float64 1.0", 1.0},
		{"non-empty string", "x"},
		{"non-empty slice", []int{1}},
		{"non-empty map", map[string]int{"a": 1}},
		{"struct", struct{}{}},
	}
	for _, tt := range truthy {
		if !IsTruthy(tt.val) {
			t.Errorf("IsTruthy(%s) = false, want true", tt.name)
		}
	}

	falsy := []struct {
		name string
		val  any
	}{
		{"nil", nil},
		{"false", false},
		{"int 0", 0},
		{"int64 0", int64(0)},
		{"float64 0.0", 0.0},
		{"empty string", ""},
		{"empty slice", []int{}},
		{"empty map", map[string]int{}},
	}
	for _, tt := range falsy {
		if IsTruthy(tt.val) {
			t.Errorf("IsTruthy(%s) = true, want false", tt.name)
		}
	}
}

func TestCopyVars(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		cp := CopyVars(nil)
		if cp == nil {
			t.Fatal("expected non-nil map for nil input")
		}
		if len(cp) != 0 {
			t.Errorf("expected empty map, got %v", cp)
		}
	})

	t.Run("populated input", func(t *testing.T) {
		orig := map[string]any{"a": 1, "b": "two"}
		cp := CopyVars(orig)
		if cp["a"] != 1 || cp["b"] != "two" {
			t.Errorf("copy doesn't match original: %v", cp)
		}
	})

	t.Run("independence", func(t *testing.T) {
		orig := map[string]any{"a": 1}
		cp := CopyVars(orig)
		cp["a"] = 99
		cp["new"] = "added"
		if orig["a"] != 1 {
			t.Error("modifying copy changed original")
		}
		if _, ok := orig["new"]; ok {
			t.Error("adding to copy affected original")
		}
	})
}

func TestDeepCopyJSON(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if DeepCopyJSON(nil) != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("deep copy map", func(t *testing.T) {
		orig := map[string]any{"a": float64(1), "nested": map[string]any{"b": float64(2)}}
		cp := DeepCopyJSON(orig)
		cpMap, ok := cp.(map[string]any)
		if !ok {
			t.Fatalf("expected map, got %T", cp)
		}
		if cpMap["a"] != float64(1) {
			t.Errorf("expected a=1, got %v", cpMap["a"])
		}

		// Modify copy, original should be untouched
		cpMap["a"] = float64(99)
		if orig["a"] != float64(1) {
			t.Error("modifying copy changed original")
		}
	})

	t.Run("non-serializable fallback", func(t *testing.T) {
		ch := make(chan int)
		result := DeepCopyJSON(ch)
		// Should return original since chan can't be JSON-marshaled
		if result != ch {
			t.Error("expected original returned for non-serializable")
		}
	})
}

func TestIDCounter(t *testing.T) {
	c := &IDCounter{}
	id1 := c.Next()
	id2 := c.Next()
	id3 := c.Next()

	if id1 != 1 {
		t.Errorf("expected first ID=1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("expected second ID=2, got %d", id2)
	}
	if id3 != 3 {
		t.Errorf("expected third ID=3, got %d", id3)
	}
	// Never returns 0
	if id1 == 0 || id2 == 0 || id3 == 0 {
		t.Error("IDCounter should never return 0")
	}
	// Never repeats
	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Error("IDCounter should never repeat")
	}
}

type testPtrNested struct {
	Inner *testInner
}

func TestResolveStructField(t *testing.T) {
	t.Run("simple field", func(t *testing.T) {
		state := &testDirectiveState{Name: "Alice"}
		val := resolveStructField(reflect.ValueOf(state), "Name")
		if val != "Alice" {
			t.Errorf("expected 'Alice', got %v", val)
		}
	})

	t.Run("dotted path", func(t *testing.T) {
		state := &testNested{Inner: testInner{Value: "deep"}}
		val := resolveStructField(reflect.ValueOf(state), "Inner.Value")
		if val != "deep" {
			t.Errorf("expected 'deep', got %v", val)
		}
	})

	t.Run("nil pointer", func(t *testing.T) {
		state := &testPtrNested{Inner: nil}
		val := resolveStructField(reflect.ValueOf(state), "Inner.Value")
		if val != nil {
			t.Errorf("expected nil for nil pointer, got %v", val)
		}
	})

	t.Run("missing field", func(t *testing.T) {
		state := &testDirectiveState{}
		val := resolveStructField(reflect.ValueOf(state), "NoSuchField")
		if val != nil {
			t.Errorf("expected nil for missing field, got %v", val)
		}
	})

	t.Run("non-struct intermediate", func(t *testing.T) {
		state := &testDirectiveState{Name: "Alice"}
		val := resolveStructField(reflect.ValueOf(state), "Name.Sub")
		if val != nil {
			t.Errorf("expected nil for non-struct intermediate, got %v", val)
		}
	})
}

// ---------------------------------------------------------------------------
// Section 3: Parsing tests
// ---------------------------------------------------------------------------

func TestParse_SVGNamespace(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<svg><rect width="100"></rect><circle r="5"></circle></svg>
		<div>not svg</div>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	svg := findTemplateTag(nodes, "svg")
	if svg == nil {
		t.Fatal("expected svg element")
	}
	if svg.Namespace != "http://www.w3.org/2000/svg" {
		t.Errorf("expected SVG namespace on svg, got %q", svg.Namespace)
	}

	rect := findTemplateTag(svg.Children, "rect")
	if rect == nil {
		t.Fatal("expected rect child")
	}
	if rect.Namespace != "http://www.w3.org/2000/svg" {
		t.Errorf("expected SVG namespace on rect, got %q", rect.Namespace)
	}

	// div outside svg should NOT have SVG namespace
	div := findTemplateTag(nodes, "div")
	if div == nil {
		t.Fatal("expected div element")
	}
	if div.Namespace != "" {
		t.Errorf("expected empty namespace on div, got %q", div.Namespace)
	}
}

func TestResolveFacts_ClassAttrToClassName(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div class="foo bar" id="main" style="color:red">text</div>
	</body></html>`

	templates, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	state := &testDirectiveState{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := ResolveTree(templates, ctx)
	div := findElement(nodes, "div")
	if div == nil {
		t.Fatal("expected div")
	}

	// class → className in Props
	if div.Facts.Props["className"] != "foo bar" {
		t.Errorf("expected className='foo bar', got %v", div.Facts.Props["className"])
	}
	// id → Props
	if div.Facts.Props["id"] != "main" {
		t.Errorf("expected id='main', got %v", div.Facts.Props["id"])
	}
	// style → Props
	if div.Facts.Props["style"] != "color:red" {
		t.Errorf("expected style='color:red', got %v", div.Facts.Props["style"])
	}

	// class, id, style should NOT be in Attrs
	if div.Facts.Attrs != nil {
		for _, key := range []string{"class", "id", "style"} {
			if _, ok := div.Facts.Attrs[key]; ok {
				t.Errorf("%q should not be in Attrs", key)
			}
		}
	}
}

func TestParse_CommentIgnored(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div>before</div>
		<!-- this is a comment -->
		<span>after</span>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should have div and span, no comment node
	div := findTemplateTag(nodes, "div")
	span := findTemplateTag(nodes, "span")
	if div == nil || span == nil {
		t.Fatal("expected div and span elements")
	}

	// No node should have comment-like content
	for _, n := range nodes {
		if n.IsText && strings.Contains(n.TextParts[0].Value, "this is a comment") {
			t.Error("comment content should not appear in parsed nodes")
		}
	}
}

func TestParse_KeydownMultiHandler(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<input g-keydown="Enter:Save;Escape:Cancel"/>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	input := findTemplateTag(nodes, "input")
	if input == nil {
		t.Fatal("expected input element")
	}

	// Should have 2 keydown directives
	keydownCount := 0
	for _, d := range input.Directives {
		if d.Type == "keydown" {
			keydownCount++
		}
	}
	if keydownCount != 2 {
		t.Errorf("expected 2 keydown directives, got %d", keydownCount)
	}

	// Verify specific entries
	found := map[string]string{}
	for _, d := range input.Directives {
		if d.Type == "keydown" {
			found[d.Name] = d.Expr
		}
	}
	if found["Enter"] != "Save" {
		t.Errorf("expected Enter:Save, got Enter:%q", found["Enter"])
	}
	if found["Escape"] != "Cancel" {
		t.Errorf("expected Escape:Cancel, got Escape:%q", found["Escape"])
	}
}

func TestParse_PluginDirective(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-plugin:chart="ChartData" class="chart-container">content</div>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	div := findTemplateTag(nodes, "div")
	if div == nil {
		t.Fatal("expected div")
	}
	if !div.IsPlugin {
		t.Error("expected IsPlugin=true")
	}
	if div.PluginName != "chart" {
		t.Errorf("expected PluginName='chart', got %q", div.PluginName)
	}
	if div.PluginExpr != "ChartData" {
		t.Errorf("expected PluginExpr='ChartData', got %q", div.PluginExpr)
	}
}

func TestParse_ComponentWithChildren(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<my-comp :title="Name"><span>child</span></my-comp>
	</body></html>`

	comps := map[string]bool{"my-comp": true}
	nodes, err := ParseTemplate(html, comps)
	if err != nil {
		t.Fatal(err)
	}

	var comp *TemplateNode
	for _, n := range nodes {
		if n.IsComponent {
			comp = n
			break
		}
	}
	if comp == nil {
		t.Fatal("expected component node")
	}
	if comp.ComponentTag != "my-comp" {
		t.Errorf("expected tag 'my-comp', got %q", comp.ComponentTag)
	}
	// Should have children
	if len(comp.Children) == 0 {
		t.Error("expected component to have children")
	}
}

func TestParse_ForWithComponent(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<todo-item g-for="t in Todos" :text="t.Text"></todo-item>
	</body></html>`

	comps := map[string]bool{"todo-item": true}
	nodes, err := ParseTemplate(html, comps)
	if err != nil {
		t.Fatal(err)
	}

	// Should have a g-for node
	var forNode *TemplateNode
	for _, n := range nodes {
		if n.IsFor {
			forNode = n
			break
		}
	}
	if forNode == nil {
		t.Fatal("expected g-for node")
	}
	// The body template should be a component
	if len(forNode.ForBody) != 1 {
		t.Fatalf("expected 1 body template, got %d", len(forNode.ForBody))
	}
	body := forNode.ForBody[0]
	if !body.IsComponent {
		t.Error("expected body template to be a component")
	}
}

func TestParse_ForWithSVG(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<svg><rect g-for="r in Rects" g-attr:width="r.W"></rect></svg>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	svg := findTemplateTag(nodes, "svg")
	if svg == nil {
		t.Fatal("expected svg")
	}

	var forNode *TemplateNode
	for _, c := range svg.Children {
		if c.IsFor {
			forNode = c
			break
		}
	}
	if forNode == nil {
		t.Fatal("expected g-for in svg")
	}
	// Body template should have SVG namespace
	if len(forNode.ForBody) != 1 {
		t.Fatalf("expected 1 body, got %d", len(forNode.ForBody))
	}
	if forNode.ForBody[0].Namespace != "http://www.w3.org/2000/svg" {
		t.Errorf("expected SVG namespace on for body, got %q", forNode.ForBody[0].Namespace)
	}
}

func TestParse_DropzoneDirective(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-dropzone="HandleDrop">drop here</div>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	div := findTemplateTagWithDirective(nodes, "div", "dropzone")
	if div == nil {
		t.Error("expected div with dropzone directive")
	}
}

func TestParse_PropPrefixSkipped(t *testing.T) {
	// :prop attributes should not appear in plain attrs or directives
	html := `<!DOCTYPE html><html><head></head><body>
		<div :title="Name" class="x">text</div>
	</body></html>`
	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}
	div := findTemplateTag(nodes, "div")
	if div == nil {
		t.Fatal("expected div")
	}
	for _, a := range div.Attrs {
		if a.Key == ":title" {
			t.Error(":title should not be in plain attrs")
		}
	}
	for _, d := range div.Directives {
		if d.Expr == "Name" && d.Type != "text" && d.Type != "bind" {
			// :prop is skipped entirely, not converted to directive
		}
	}
}

func TestParse_NoBody(t *testing.T) {
	// Go's html.Parse always synthesizes a <body> node, so findBody never returns nil.
	// Even an input with no explicit <body> tag produces an empty body → empty node list.
	nodes, err := ParseTemplate("<html><head></head></html>", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Body exists but is empty → no child template nodes (only whitespace, if any)
	for _, n := range nodes {
		if !n.IsText {
			t.Errorf("expected only text nodes (whitespace), got element: %s", n.Tag)
		}
	}
}

func TestParse_ForExprNoIn(t *testing.T) {
	// ParseForExpr with no "in" keyword
	item, index, list := ParseForExpr("justAnExpression")
	if item != "" || index != "" || list != "justAnExpression" {
		t.Errorf("expected ('', '', 'justAnExpression'), got (%q, %q, %q)", item, index, list)
	}
}

func TestParse_TextInterpolationUnclosed(t *testing.T) {
	// Unclosed {{ should be treated as static text
	parts := ParseTextInterpolations("hello {{unclosed")
	if len(parts) != 1 || !parts[0].Static {
		t.Errorf("expected 1 static part for unclosed interpolation, got %+v", parts)
	}
}

func TestParse_TextInterpolationEmpty(t *testing.T) {
	parts := ParseTextInterpolations("")
	if len(parts) != 1 || parts[0].Value != "" {
		t.Errorf("expected 1 empty static part, got %+v", parts)
	}
}

type testCallState struct {
	Name string
}

func (s *testCallState) NoReturn() {
	// method with no return value
}

func (s *testCallState) TwoArgs(a, b int) int {
	return a + b
}

func TestResolveExpr_MethodNoReturn(t *testing.T) {
	state := &testCallState{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	// Zero-arg method with no return → callMethod returns nil (wrong sig)
	val := ResolveExpr("NoReturn", ctx)
	if val != nil {
		t.Errorf("expected nil for method with no return, got %v", val)
	}
}

func TestResolve_NextIDWithoutCounter(t *testing.T) {
	// When IDs is nil, nextID returns 0
	tmpl := &TemplateNode{Tag: "div"}
	state := &testDirectiveState{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any), IDs: nil}
	nodes := ResolveTemplateNode(tmpl, ctx)
	el := nodes[0].(*ElementNode)
	if el.ID != 0 {
		t.Errorf("expected ID=0 without counter, got %d", el.ID)
	}
}

func TestResolve_NextIDWithCounter(t *testing.T) {
	tmpl := &TemplateNode{Tag: "div"}
	state := &testDirectiveState{}
	ids := &IDCounter{}
	ctx := &ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any), IDs: ids}
	nodes := ResolveTemplateNode(tmpl, ctx)
	el := nodes[0].(*ElementNode)
	if el.ID != 1 {
		t.Errorf("expected ID=1 with counter, got %d", el.ID)
	}
}

func TestParse_DraggableDirective(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
		<div g-draggable="ItemID">drag me</div>
		<div g-draggable:cards="ItemID">drag card</div>
	</body></html>`

	nodes, err := ParseTemplate(html, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Find divs with draggable directive
	var noGroup, withGroup *TemplateNode
	for _, n := range nodes {
		for _, d := range n.Directives {
			if d.Type == "draggable" && d.Name == "" {
				noGroup = n
			}
			if d.Type == "draggable" && d.Name == "cards" {
				withGroup = n
			}
		}
		// Search children too
		for _, c := range n.Children {
			for _, d := range c.Directives {
				if d.Type == "draggable" && d.Name == "" {
					noGroup = c
				}
				if d.Type == "draggable" && d.Name == "cards" {
					withGroup = c
				}
			}
		}
	}

	if noGroup == nil {
		t.Error("expected draggable directive without group")
	}
	if withGroup == nil {
		t.Error("expected draggable directive with group 'cards'")
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func findTemplateTag(nodes []*TemplateNode, tag string) *TemplateNode {
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

func findTemplateTagWithDirective(nodes []*TemplateNode, tag, dirType string) *TemplateNode {
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

func findElement(nodes []Node, tag string) *ElementNode {
	for _, n := range nodes {
		switch n := n.(type) {
		case *ElementNode:
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

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestParse_ShowClickDropDirectives(t *testing.T) {
	// Exercises extractAttrsAndDirectives for g-show, g-click, g-drop
	tmplHTML := `<div g-show="Visible" g-click="Save" g-drop="HandleDrop"></div>`
	nodes, err := ParseTemplate(tmplHTML, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	div := findTemplateByTag(nodes, "div")
	if div == nil {
		t.Fatal("no div found")
	}
	var hasShow, hasClick, hasDrop bool
	for _, d := range div.Directives {
		switch d.Type {
		case "show":
			hasShow = true
		case "click":
			hasClick = true
		case "drop":
			hasDrop = true
		}
	}
	if !hasShow {
		t.Error("missing show directive")
	}
	if !hasClick {
		t.Error("missing click directive")
	}
	if !hasDrop {
		t.Error("missing drop directive")
	}
}

func TestParse_AllMouseDirectives(t *testing.T) {
	// Exercises extractAttrsAndDirectives for all mouse events + wheel
	tmplHTML := `<div g-mousedown="Down" g-mousemove="Move" g-mouseup="Up" g-wheel="Scroll"></div>`
	nodes, err := ParseTemplate(tmplHTML, nil)
	if err != nil {
		t.Fatal(err)
	}
	div := findTemplateByTag(nodes, "div")
	if div == nil {
		t.Fatal("no div found")
	}
	types := map[string]bool{}
	for _, d := range div.Directives {
		types[d.Type] = true
	}
	for _, expected := range []string{"mousedown", "mousemove", "mouseup", "wheel"} {
		if !types[expected] {
			t.Errorf("missing %s directive", expected)
		}
	}
}

func TestParse_StyleAndIdAttrs(t *testing.T) {
	// Exercises resolveFacts for style and id as props (not attrs)
	tmplHTML := `<div style="color:red" id="main"></div>`
	nodes, err := ParseTemplate(tmplHTML, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := struct{}{}
	counter := &IDCounter{}
	ctx := &ResolveContext{State: reflect.ValueOf(&state), IDs: counter}
	tree := ResolveTree(nodes, ctx)
	el := tree[0].(*ElementNode)
	if el.Facts.Props["style"] != "color:red" {
		t.Errorf("expected style prop 'color:red', got %v", el.Facts.Props["style"])
	}
	if el.Facts.Props["id"] != "main" {
		t.Errorf("expected id prop 'main', got %v", el.Facts.Props["id"])
	}
	// Verify they don't leak into attrs
	if el.Facts.Attrs != nil {
		if _, ok := el.Facts.Attrs["style"]; ok {
			t.Error("style should not be in Attrs")
		}
		if _, ok := el.Facts.Attrs["id"]; ok {
			t.Error("id should not be in Attrs")
		}
	}
}

// findTemplateByTag searches template nodes for an element with the given tag.
func findTemplateByTag(nodes []*TemplateNode, tag string) *TemplateNode {
	for _, n := range nodes {
		if n.Tag == tag {
			return n
		}
		if found := findTemplateByTag(n.Children, tag); found != nil {
			return found
		}
	}
	return nil
}

func TestResolveExpr_CallMethodWithNilArg(t *testing.T) {
	// Exercises callMethodWithArgs nil arg path (line 832-833)
	type state struct{}
	s := &state{}
	ctx := &ResolveContext{State: reflect.ValueOf(s)}
	// Call a method that doesn't exist with args → returns nil
	result := ResolveExpr("NonExistent(x)", ctx)
	if result != nil {
		t.Errorf("expected nil for non-existent method, got %v", result)
	}
}

func TestResolveExpr_CallMethodWithActualNilArg(t *testing.T) {
	// Exercises callMethodWithArgs where a resolved arg is nil → reflect.Zero path
	s := &testCallState{Name: "test"}
	ctx := &ResolveContext{
		State: reflect.ValueOf(s),
		Vars:  map[string]any{"a": nil, "b": nil},
		IDs:   &IDCounter{},
	}
	// TwoArgs(a, b) where a,b are nil → callMethodWithArgs uses reflect.Zero → int(0)
	result := ResolveExpr("TwoArgs(a, b)", ctx)
	if result != 0 {
		t.Errorf("expected 0 (zero-value int args), got %v", result)
	}
}

func TestParse_ForSVGElement(t *testing.T) {
	// Exercises parseForTemplate SVG namespace path (line 179-181)
	tmplHTML := `<svg><rect g-for="r in Rects" width="10"></rect></svg>`
	nodes, err := ParseTemplate(tmplHTML, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Find the svg node, then the for node inside it
	svg := findTemplateByTag(nodes, "svg")
	if svg == nil {
		t.Fatal("no svg found")
	}
	// The rect with g-for should be parsed as a ForNode
	var forNode *TemplateNode
	for _, c := range svg.Children {
		if c.IsFor {
			forNode = c
			break
		}
	}
	if forNode == nil {
		t.Fatal("no for node found inside svg")
	}
	// The for body item template should have SVG namespace
	body := forNode.ForBody[0]
	if body.Namespace != "http://www.w3.org/2000/svg" {
		t.Errorf("expected SVG namespace on for body, got %q", body.Namespace)
	}
}

func TestResolveStructField_NilPtrInPath(t *testing.T) {
	type Inner struct {
		Value string
	}
	type Outer struct {
		Inner *Inner
	}
	s := &Outer{Inner: nil}
	ctx := &ResolveContext{State: reflect.ValueOf(s)}
	result := ResolveExpr("Inner.Value", ctx)
	if result != nil {
		t.Errorf("expected nil for nil pointer in path, got %v", result)
	}
}

func TestParse_ExtractPropExprsNone(t *testing.T) {
	// Test component with no :prop attrs → PropExprs is nil
	html := `<my-comp></my-comp>`
	comps := map[string]bool{"my-comp": true}
	nodes, err := ParseTemplate(html, comps)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	comp := nodes[0]
	if comp.PropExprs != nil {
		t.Errorf("expected nil PropExprs when no :props, got %v", comp.PropExprs)
	}
}

func TestParse_ForComponentNoChildren(t *testing.T) {
	// Tests parseForTemplate with a component tag that has no child nodes
	html := `<my-item g-for="item in Items" :text="item.Text"></my-item>`
	comps := map[string]bool{"my-item": true}
	nodes, err := ParseTemplate(html, comps)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	forNode := nodes[0]
	if !forNode.IsFor {
		t.Fatal("expected for node")
	}
	body := forNode.ForBody[0]
	if !body.IsComponent {
		t.Error("expected component in for body")
	}
	if body.PropExprs == nil || body.PropExprs["text"] != "item.Text" {
		t.Error("expected :text prop expr")
	}
}

func findNodeText(nodes []Node, text string) bool {
	for _, n := range nodes {
		switch n := n.(type) {
		case *TextNode:
			if strings.Contains(n.Text, text) {
				return true
			}
		case *ElementNode:
			if findNodeText(n.Children, text) {
				return true
			}
		case *KeyedElementNode:
			for _, kc := range n.Children {
				if findNodeText([]Node{kc.Node}, text) {
					return true
				}
			}
		}
	}
	return false
}
