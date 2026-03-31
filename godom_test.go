package godom

import (
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/anupshinde/godom/internal/component"
)

// --- test app structs ---

type testApp struct {
	Component
	Name  string
	Count int
}

func (a *testApp) Increment() {
	a.Count++
}

// noComponentApp does NOT embed godom.Component
type noComponentApp struct {
	Name string
}

// --- NewEngine ---

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("expected non-nil Engine")
	}
	if e.plugins == nil {
		t.Error("expected non-nil plugins map")
	}
	if len(e.comps) != 0 {
		t.Error("expected empty comps before Register")
	}
}

// --- RegisterPlugin ---

func TestRegisterPlugin(t *testing.T) {
	e := NewEngine()
	e.RegisterPlugin("chart", "console.log('chart')")

	scripts, ok := e.plugins["chart"]
	if !ok {
		t.Fatal("expected plugin 'chart' to be registered")
	}
	if len(scripts) != 1 || scripts[0] != "console.log('chart')" {
		t.Errorf("unexpected scripts: %v", scripts)
	}
}

func TestRegisterPlugin_MultipleScripts(t *testing.T) {
	e := NewEngine()
	e.RegisterPlugin("editor", "script1.js", "script2.js")

	scripts := e.plugins["editor"]
	if len(scripts) != 2 {
		t.Fatalf("expected 2 scripts, got %d", len(scripts))
	}
	if scripts[0] != "script1.js" || scripts[1] != "script2.js" {
		t.Errorf("expected [script1.js script2.js], got %v", scripts)
	}
}

func TestRegisterPlugin_Overwrite(t *testing.T) {
	e := NewEngine()
	e.RegisterPlugin("chart", "old.js")
	e.RegisterPlugin("chart", "new.js")

	scripts := e.plugins["chart"]
	if len(scripts) != 1 || scripts[0] != "new.js" {
		t.Errorf("expected overwritten script 'new.js', got %v", scripts)
	}
}

// --- embedsComponent ---

func TestEmbedsComponent_True(t *testing.T) {
	typ := reflect.TypeOf(testApp{})
	if !embedsComponent(typ) {
		t.Error("expected testApp to embed Component")
	}
}

func TestEmbedsComponent_False(t *testing.T) {
	typ := reflect.TypeOf(noComponentApp{})
	if embedsComponent(typ) {
		t.Error("expected noComponentApp to NOT embed Component")
	}
}

func TestEmbedsComponent_EmptyStruct(t *testing.T) {
	type empty struct{}
	typ := reflect.TypeOf(empty{})
	if embedsComponent(typ) {
		t.Error("expected empty struct to NOT embed Component")
	}
}

// --- Component.Refresh ---

func TestRefresh_NilCI(t *testing.T) {
	c := Component{ci: nil}
	// Should not panic
	c.Refresh()
}

func TestRefresh_NilRefreshFn(t *testing.T) {
	// ci exists but RefreshFn is nil
	c := Component{ci: &component.Info{}}
	// Should not panic — just no-op because RefreshFn is nil
	c.Refresh()
}

func TestRefresh_SendsToEventChannel(t *testing.T) {
	ch := make(chan component.Event, 1)
	c := Component{ci: &component.Info{
		EventCh: ch,
	}}

	c.Refresh()

	select {
	case evt := <-ch:
		if evt.Kind != component.RefreshKind {
			t.Errorf("expected RefreshKind, got %v", evt.Kind)
		}
	default:
		t.Error("expected event on channel, got none")
	}
}

func TestRefresh_MarkedFieldsPassedThrough(t *testing.T) {
	ci := &component.Info{}
	c := Component{ci: ci}

	c.MarkRefresh("Name", "Count")

	fields := ci.DrainMarkedFields()
	if len(fields) != 2 || fields[0] != "Name" || fields[1] != "Count" {
		t.Errorf("expected MarkedFields [Name Count], got %v", fields)
	}
}

func TestMarkRefresh_NilCI(t *testing.T) {
	c := Component{ci: nil}
	// Should not panic
	c.MarkRefresh("Name")
}

func TestMarkRefresh_Accumulates(t *testing.T) {
	ci := &component.Info{}
	c := Component{ci: ci}

	c.MarkRefresh("Name")
	c.MarkRefresh("Count")

	fields := ci.DrainMarkedFields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 accumulated fields, got %d", len(fields))
	}
	if fields[0] != "Name" || fields[1] != "Count" {
		t.Errorf("expected [Name Count], got %v", fields)
	}

	// After drain, should be empty
	fields = ci.DrainMarkedFields()
	if len(fields) != 0 {
		t.Errorf("expected empty after drain, got %v", fields)
	}
}

// --- Register ---

var testHTML = `<!DOCTYPE html><html><head><title>Test</title></head><body>
	<span g-text="Name">placeholder</span>
	<button g-click="Increment">+</button>
</body></html>`

func makeTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte(testHTML)},
		"child/index.html": &fstest.MapFile{Data: []byte(childHTML)},
	}
}

func makeTestFSNested() fstest.MapFS {
	return fstest.MapFS{
		"ui/index.html":    &fstest.MapFile{Data: []byte(testHTML)},
		"ui/style.css":     &fstest.MapFile{Data: []byte("body{}")},
		"child/index.html": &fstest.MapFile{Data: []byte(childHTML)},
	}
}

func TestRegister_Valid(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	if len(e.comps) != 1 {
		t.Fatal("expected one component after Register")
	}
	ci := e.comps[0]
	if ci.HTMLBody == "" {
		t.Error("expected HTMLBody to be set")
	}
	if ci.VDOMTemplates == nil {
		t.Fatal("expected VDOMTemplates to be parsed")
	}
	if len(ci.VDOMTemplates) == 0 {
		t.Error("expected at least one parsed VDOM template")
	}
	if e.staticFS == nil {
		t.Error("expected staticFS to be set")
	}
}

func TestRegister_NestedEntryPath(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFSNested())
	e.Register("main", app, "ui/index.html")

	if len(e.comps) != 1 {
		t.Fatal("expected one component after Register")
	}
	// staticFS should be the "ui/" subdirectory — verify by reading a file from it
	if e.staticFS == nil {
		t.Fatal("expected staticFS to be set")
	}
	data, err := fs.ReadFile(e.staticFS, "style.css")
	if err != nil {
		t.Errorf("expected staticFS to contain style.css (sub-dir of ui/), got error: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("expected style.css content 'body{}', got %q", string(data))
	}
}

func TestRegister_WiresComponentField(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	// After Register, app.Component.ci should be wired to the same Info as e.comps[0]
	if app.Component.ci == nil {
		t.Fatal("expected Component.ci to be wired after Register")
	}
	if app.Component.ci != e.comps[0] {
		t.Error("expected Component.ci to point to the same Info as Engine.comps[0]")
	}
}

func TestRegister_SetsHTMLBodyFromTemplate(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	ci := e.comps[0]
	if !strings.Contains(ci.HTMLBody, "g-text") {
		t.Error("expected HTMLBody to contain template directives")
	}
	if !strings.Contains(ci.HTMLBody, "g-click") {
		t.Error("expected HTMLBody to contain event directive")
	}
}

func TestRegister_NonPointer(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_NONPTR") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register("main", testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_NonPointer", "TEST_FATAL_REGISTER_NONPTR")
	if !strings.Contains(out, "requires a pointer to a struct") {
		t.Errorf("expected error about pointer to struct, got: %s", out)
	}
}

func TestRegister_PointerToNonStruct(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_NONSTRUCT") == "1" {
		e := NewEngine()
		n := 42
		e.SetFS(makeTestFS())
		e.Register("main", &n, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_PointerToNonStruct", "TEST_FATAL_REGISTER_NONSTRUCT")
	if !strings.Contains(out, "requires a pointer to a struct") {
		t.Errorf("expected error about pointer to struct, got: %s", out)
	}
}

func TestRegister_NoEmbed(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_NOEMBED") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register("main", &noComponentApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_NoEmbed", "TEST_FATAL_REGISTER_NOEMBED")
	if !strings.Contains(out, "must embed godom.Component") {
		t.Errorf("expected error about embedding Component, got: %s", out)
	}
}

func TestRegister_BadEntryPath(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_BADPATH") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register("main", &testApp{}, "nonexistent.html")
		return
	}
	out := runSubprocess(t, "TestRegister_BadEntryPath", "TEST_FATAL_REGISTER_BADPATH")
	if !strings.Contains(out, "failed to read") {
		t.Errorf("expected error about failed to read, got: %s", out)
	}
}

func TestRegister_SetsValueAndType(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Bob", Count: 42}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	ci := e.comps[0]
	// ci.Value should point to the same app instance
	if ci.Value.Pointer() != reflect.ValueOf(app).Pointer() {
		t.Error("expected ci.Value to point to the original app")
	}
	if ci.Typ != reflect.TypeOf(testApp{}) {
		t.Errorf("expected ci.Typ = testApp, got %v", ci.Typ)
	}
}

func TestRegister_RefreshWorksAfterRegister(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	// After Register, Refresh should not panic (ci is wired, but RefreshFn is nil until Start)
	app.Refresh()
}

func TestRegister_InvalidDirective(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_BADDIR") == "1" {
		e := NewEngine()
		// g-click references a method that doesn't exist on the struct
		badHTML := `<!DOCTYPE html><html><head></head><body>
			<button g-click="NonExistentMethod">click</button>
		</body></html>`
		badFS := fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(badHTML)},
		}
		e.SetFS(badFS)
		e.Register("main", &testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_InvalidDirective", "TEST_FATAL_REGISTER_BADDIR")
	if !strings.Contains(out, "NonExistentMethod") {
		t.Errorf("expected error mentioning NonExistentMethod, got: %s", out)
	}
}

func TestRegister_EmptyName(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_EMPTY") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register("", &testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_EmptyName", "TEST_FATAL_REGISTER_EMPTY")
	if !strings.Contains(out, "non-empty name") {
		t.Errorf("expected non-empty name error, got: %s", out)
	}
}

func TestRegister_InvalidIdentifier(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_BADID") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register("123invalid", &testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_InvalidIdentifier", "TEST_FATAL_REGISTER_BADID")
	if !strings.Contains(out, "valid identifier") {
		t.Errorf("expected valid identifier error, got: %s", out)
	}
}

func TestRegister_NoFS(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_NOFS") == "1" {
		e := NewEngine()
		// Don't call SetFS
		e.Register("main", &testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_NoFS", "TEST_FATAL_REGISTER_NOFS")
	if !strings.Contains(out, "call SetFS() before Register()") {
		t.Errorf("expected SetFS error, got: %s", out)
	}
}

func TestRegister_DuplicateNameFatals(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_DUP") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register("main", &testApp{}, "index.html")
		e.Register("main", &testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_DuplicateNameFatals", "TEST_FATAL_REGISTER_DUP")
	if !strings.Contains(out, "already registered") {
		t.Errorf("expected 'already registered' error, got: %s", out)
	}
}

type childApp struct {
	Component
	Value string
}

var childHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Value">placeholder</span></body></html>`

func TestRegister_MultipleComponents_StaticFSFromFirst(t *testing.T) {
	e := NewEngine()
	parent := &testApp{Name: "parent"}
	child := &childApp{Value: "child"}

	e.SetFS(makeTestFSNested())
	e.Register("parent", parent, "ui/index.html")
	e.Register("child", child, "child/index.html")

	// staticFS should be derived from the first Register call only
	if e.staticFS == nil {
		t.Fatal("expected staticFS to be set")
	}
	// It should be the "ui/" subdirectory from the first registered component
	data, err := fs.ReadFile(e.staticFS, "style.css")
	if err != nil {
		t.Errorf("expected staticFS from first register, got error: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

// --- Auto-wiring tests ---

func TestAutoWire_SetsSlotName(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())

	child := &childApp{Value: "child"}
	e.Register("sidebar", child, "child/index.html")

	e.autoWireComponents()

	if e.comps[0].SlotName != "sidebar" {
		t.Errorf("expected SlotName='sidebar', got %q", e.comps[0].SlotName)
	}
}

func TestAutoWire_MultipleChildrenWiredCorrectly(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())

	child1 := &childApp{Value: "c1"}
	e.Register("sidebar", child1, "child/index.html")

	child2 := &childApp{Value: "c2"}
	e.Register("footer", child2, "child/index.html")

	e.autoWireComponents()

	if e.comps[0].SlotName != "sidebar" {
		t.Errorf("sidebar: expected SlotName='sidebar', got %q", e.comps[0].SlotName)
	}
	if e.comps[1].SlotName != "footer" {
		t.Errorf("footer: expected SlotName='footer', got %q", e.comps[1].SlotName)
	}
}

// --- Route ---

var layoutHTML = `<!DOCTYPE html><html><head><title>{{.Title}}</title></head><body>{{block "content" .}}{{end}}</body></html>`
var pageHTML = `{{define "content"}}<h1>{{.Title}}</h1><div g-component="widget"></div>{{end}}`

func makeRouteTestFS() fstest.MapFS {
	return fstest.MapFS{
		"layout.html":      &fstest.MapFile{Data: []byte(layoutHTML)},
		"page.html":        &fstest.MapFile{Data: []byte(pageHTML)},
		"child/index.html": &fstest.MapFile{Data: []byte(childHTML)},
	}
}

type pageData struct {
	Title string
}

func TestRoute_Valid(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeRouteTestFS())

	e.Route("/", &pageData{Title: "Home"}, "layout.html", "page.html")

	if len(e.routes) != 1 {
		t.Fatal("expected one route")
	}
	r := e.routes[0]
	if r.pattern != "/" {
		t.Errorf("expected pattern '/', got %q", r.pattern)
	}
	// Template data should be rendered into the HTML
	if !strings.Contains(r.templateHTML, "<title>Home</title>") {
		t.Error("expected rendered title in HTML")
	}
	if !strings.Contains(r.templateHTML, "<h1>Home</h1>") {
		t.Error("expected rendered h1 in HTML")
	}
	if !strings.Contains(r.templateHTML, `g-component="widget"`) {
		t.Error("expected g-component attribute in HTML")
	}
}

func TestRoute_NilData(t *testing.T) {
	e := NewEngine()
	simpleHTML := `<!DOCTYPE html><html><head></head><body><p>Static</p></body></html>`
	e.SetFS(fstest.MapFS{
		"simple.html": &fstest.MapFile{Data: []byte(simpleHTML)},
	})

	e.Route("/about", nil, "simple.html")

	if len(e.routes) != 1 {
		t.Fatal("expected one route")
	}
	if !strings.Contains(e.routes[0].templateHTML, "Static") {
		t.Error("expected static content in HTML")
	}
}

func TestRoute_MultipleRoutes(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeRouteTestFS())

	e.Route("/", &pageData{Title: "Home"}, "layout.html", "page.html")
	e.Route("/settings", &pageData{Title: "Settings"}, "layout.html", "page.html")

	if len(e.routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(e.routes))
	}
	if !strings.Contains(e.routes[0].templateHTML, "Home") {
		t.Error("first route should contain Home")
	}
	if !strings.Contains(e.routes[1].templateHTML, "Settings") {
		t.Error("second route should contain Settings")
	}
}

func TestRoute_SetsStaticFS(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFSNested())

	e.Route("/", nil, "ui/index.html")

	if e.staticFS == nil {
		t.Fatal("expected staticFS to be set from Route")
	}
	data, err := fs.ReadFile(e.staticFS, "style.css")
	if err != nil {
		t.Errorf("expected staticFS to contain style.css, got error: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestRoute_NoFS(t *testing.T) {
	if os.Getenv("TEST_FATAL_ROUTE_NOFS") == "1" {
		e := NewEngine()
		e.Route("/", nil, "index.html")
		return
	}
	out := runSubprocess(t, "TestRoute_NoFS", "TEST_FATAL_ROUTE_NOFS")
	if !strings.Contains(out, "call SetFS() before Route()") {
		t.Errorf("expected SetFS error, got: %s", out)
	}
}

func TestRoute_NoTemplateFiles(t *testing.T) {
	if os.Getenv("TEST_FATAL_ROUTE_NOTMPL") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Route("/", nil)
		return
	}
	out := runSubprocess(t, "TestRoute_NoTemplateFiles", "TEST_FATAL_ROUTE_NOTMPL")
	if !strings.Contains(out, "requires at least one template file") {
		t.Errorf("expected template file error, got: %s", out)
	}
}

func TestRoute_LayoutComposition(t *testing.T) {
	// Verify that {{block}}/{{define}} actually composes layout + page content.
	e := NewEngine()
	e.SetFS(makeRouteTestFS())

	e.Route("/", &pageData{Title: "Dashboard"}, "layout.html", "page.html")

	html := e.routes[0].templateHTML
	// Layout structure should be present
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE from layout")
	}
	if !strings.Contains(html, "<title>Dashboard</title>") {
		t.Error("expected title rendered from data in layout head")
	}
	// Page content block should be inlined into the layout body
	if !strings.Contains(html, "<h1>Dashboard</h1>") {
		t.Error("expected page content block rendered inside layout")
	}
	// The body should contain both layout structure and page content
	if !strings.Contains(html, "</body>") {
		t.Error("expected closing body tag from layout")
	}
}

func TestRoute_StaticFSNotOverriddenBySecondRoute(t *testing.T) {
	// First Route sets staticFS; second Route with a different path should not override it.
	e := NewEngine()
	e.SetFS(fstest.MapFS{
		"ui/page.html":    &fstest.MapFile{Data: []byte(`<html><body>page</body></html>`)},
		"ui/style.css":    &fstest.MapFile{Data: []byte("body{}")},
		"other/page.html": &fstest.MapFile{Data: []byte(`<html><body>other</body></html>`)},
	})

	e.Route("/", nil, "ui/page.html")
	e.Route("/other", nil, "other/page.html")

	// staticFS should still be "ui/" from the first route
	data, err := fs.ReadFile(e.staticFS, "style.css")
	if err != nil {
		t.Errorf("staticFS should be from first route (ui/), got error: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestRoute_InvalidTemplateSyntax(t *testing.T) {
	if os.Getenv("TEST_FATAL_ROUTE_BADSYNTAX") == "1" {
		e := NewEngine()
		e.SetFS(fstest.MapFS{
			"bad.html": &fstest.MapFile{Data: []byte(`<html>{{.Unclosed`)},
		})
		e.Route("/", nil, "bad.html")
		return
	}
	out := runSubprocess(t, "TestRoute_InvalidTemplateSyntax", "TEST_FATAL_ROUTE_BADSYNTAX")
	if !strings.Contains(out, "failed to parse template") {
		t.Errorf("expected parse error, got: %s", out)
	}
}

func TestRouteAndRegister_Together(t *testing.T) {
	// The main use case: routes serve pages, components attach via g-component.
	e := NewEngine()
	e.SetFS(makeRouteTestFS())

	e.Route("/", &pageData{Title: "Home"}, "layout.html", "page.html")

	child := &childApp{Value: "live"}
	e.Register("widget", child, "child/index.html")

	// Routes and components should both be populated
	if len(e.routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(e.routes))
	}
	if len(e.comps) != 1 {
		t.Fatalf("expected 1 component, got %d", len(e.comps))
	}

	// Route HTML should have the g-component target
	if !strings.Contains(e.routes[0].templateHTML, `g-component="widget"`) {
		t.Error("route HTML should contain g-component target for the registered component")
	}

	// Component should be wired
	if child.Component.ci == nil {
		t.Error("expected component ci to be wired after Register")
	}
}

func TestStart_RoutesOnlyNoComponents(t *testing.T) {
	// Routes without any Register — valid configuration (static pages, no live components).
	e := NewEngine()
	simpleHTML := `<!DOCTYPE html><html><body><p>Static page</p></body></html>`
	e.SetFS(fstest.MapFS{
		"page.html": &fstest.MapFile{Data: []byte(simpleHTML)},
	})
	e.Route("/", nil, "page.html")

	// Start should not return an error for the "no routes or components" check.
	// It will try to bind a port and block, so we can't fully test Start(),
	// but we can verify the pre-checks pass by checking the error message isn't
	// the "no routes or components" one.
	// The actual error will come from GODOM_VALIDATE_ONLY.
	t.Setenv("GODOM_VALIDATE_ONLY", "1")

	// This will call os.Exit(0) in a subprocess
	if os.Getenv("TEST_START_ROUTES_ONLY") == "1" {
		e.Start()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStart_RoutesOnlyNoComponents$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_START_ROUTES_ONLY=1", "GODOM_VALIDATE_ONLY=1")
	out, _ := cmd.CombinedOutput()
	outStr := string(out)
	if strings.Contains(outStr, "no routes or components") {
		t.Error("Start should accept routes-only configuration")
	}
}

func TestRoute_RootPatternStaticFS(t *testing.T) {
	// When template is at root level (no subdirectory), staticFS should be the whole FS.
	e := NewEngine()
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<html><body>hi</body></html>`)},
		"style.css":  &fstest.MapFile{Data: []byte("body{}")},
	}
	e.SetFS(fsys)
	e.Route("/", nil, "index.html")

	// staticFS should be the whole FS (dir of "index.html" is ".")
	data, err := fs.ReadFile(e.staticFS, "style.css")
	if err != nil {
		t.Errorf("expected staticFS to be whole FS, got error: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestRoute_BadTemplateFile(t *testing.T) {
	if os.Getenv("TEST_FATAL_ROUTE_BADTMPL") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Route("/", nil, "nonexistent.html")
		return
	}
	out := runSubprocess(t, "TestRoute_BadTemplateFile", "TEST_FATAL_ROUTE_BADTMPL")
	if !strings.Contains(out, "failed to read template") {
		t.Errorf("expected read error, got: %s", out)
	}
}

// --- Start ---

func TestStart_NoRoutesOrComponents(t *testing.T) {
	e := NewEngine()
	err := e.Start()
	if err == nil {
		t.Fatal("expected error when calling Start without Route or Register")
	}
	if !strings.Contains(err.Error(), "no routes or components") {
		t.Errorf("expected 'no routes or components' error, got: %v", err)
	}
}

// --- SetMux ---

func TestSetMux(t *testing.T) {
	e := NewEngine()
	mux := http.NewServeMux()
	e.SetMux(mux)

	if e.userMux != mux {
		t.Error("expected userMux to be set")
	}
}

// --- applyEnv ---

func TestApplyEnv_ReadsEnvVars(t *testing.T) {
	t.Setenv("GODOM_PORT", "9090")
	t.Setenv("GODOM_HOST", "0.0.0.0")
	t.Setenv("GODOM_NO_AUTH", "1")
	t.Setenv("GODOM_TOKEN", "secret123")
	t.Setenv("GODOM_NO_BROWSER", "true")
	t.Setenv("GODOM_QUIET", "1")

	e := NewEngine()
	e.applyEnv()

	if e.Port != 9090 {
		t.Errorf("Port = %d, want 9090", e.Port)
	}
	if e.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want \"0.0.0.0\"", e.Host)
	}
	if !e.NoAuth {
		t.Error("NoAuth should be true")
	}
	if e.Token != "secret123" {
		t.Errorf("Token = %q, want \"secret123\"", e.Token)
	}
	if !e.NoBrowser {
		t.Error("NoBrowser should be true")
	}
	if !e.Quiet {
		t.Error("Quiet should be true")
	}
}

func TestApplyEnv_CodeTakesPriority(t *testing.T) {
	t.Setenv("GODOM_PORT", "9090")
	t.Setenv("GODOM_HOST", "0.0.0.0")
	t.Setenv("GODOM_TOKEN", "envtoken")

	e := NewEngine()
	e.Port = 8080
	e.Host = "localhost"
	e.Token = "codetoken"
	e.applyEnv()

	if e.Port != 8080 {
		t.Errorf("Port = %d, want 8080 (code should win)", e.Port)
	}
	if e.Host != "localhost" {
		t.Errorf("Host = %q, want \"localhost\" (code should win)", e.Host)
	}
	if e.Token != "codetoken" {
		t.Errorf("Token = %q, want \"codetoken\" (code should win)", e.Token)
	}
}

func TestApplyEnv_NoGodomEnvSkipsAll(t *testing.T) {
	t.Setenv("GODOM_PORT", "9090")
	t.Setenv("GODOM_HOST", "0.0.0.0")
	t.Setenv("GODOM_NO_AUTH", "1")
	t.Setenv("GODOM_TOKEN", "secret")
	t.Setenv("GODOM_NO_BROWSER", "1")
	t.Setenv("GODOM_QUIET", "1")

	e := NewEngine()
	e.NoGodomEnv = true
	e.applyEnv()

	if e.Port != 0 {
		t.Errorf("Port = %d, want 0 (env should be skipped)", e.Port)
	}
	if e.Host != "" {
		t.Errorf("Host = %q, want \"\" (env should be skipped)", e.Host)
	}
	if e.NoAuth {
		t.Error("NoAuth should be false (env should be skipped)")
	}
	if e.Token != "" {
		t.Errorf("Token = %q, want \"\" (env should be skipped)", e.Token)
	}
	if e.NoBrowser {
		t.Error("NoBrowser should be false (env should be skipped)")
	}
	if e.Quiet {
		t.Error("Quiet should be false (env should be skipped)")
	}
}

func TestApplyEnv_InvalidPort(t *testing.T) {
	t.Setenv("GODOM_PORT", "notanumber")

	e := NewEngine()
	e.applyEnv()

	if e.Port != 0 {
		t.Errorf("Port = %d, want 0 (invalid port should be ignored)", e.Port)
	}
}

// --- Helpers ---

// runSubprocess re-runs the named test in a subprocess with the given env var set to "1".
// Returns combined stdout+stderr. Fails the test if the subprocess exits cleanly (expected fatal).
func runSubprocess(t *testing.T, testName, envVar string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), envVar+"=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected subprocess to exit with error, but it succeeded")
	}
	return string(out)
}
