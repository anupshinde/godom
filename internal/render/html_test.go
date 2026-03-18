package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/anupshinde/godom/internal/vdom"
)

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

	gid := &GIDCounter{}
	html := RenderToHTML(nodes, gid)

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

	gid := &GIDCounter{}
	html := RenderToHTML(nodes, gid)

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

	gid := &GIDCounter{}
	html := RenderToHTML(nodes, gid)

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

	type testCounter struct {
		Count int
		Name  string
	}
	state := &testCounter{Count: 99, Name: "Alice"}
	ctx := &vdom.ResolveContext{State: reflect.ValueOf(state), Vars: make(map[string]any)}
	nodes := vdom.ResolveTree(templates, ctx)

	gid := &GIDCounter{}
	output := RenderToHTML(nodes, gid)

	if !strings.Contains(output, "99") {
		t.Errorf("expected '99' in output, got %q", output)
	}
	if !strings.Contains(output, `value="Alice"`) {
		t.Errorf("expected value='Alice' in output, got %q", output)
	}
}
