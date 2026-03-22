package godom

import (
	"io/fs"
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
	if e.components == nil {
		t.Error("expected non-nil components map")
	}
	if e.plugins == nil {
		t.Error("expected non-nil plugins map")
	}
	if e.comp != nil {
		t.Error("expected nil comp before Mount")
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

func TestRefresh_DelegatesToRefreshFn(t *testing.T) {
	called := false
	c := Component{ci: &component.Info{
		RefreshFn: func() {
			called = true
		},
	}}

	c.Refresh()

	if !called {
		t.Error("expected RefreshFn to be called")
	}
}

func TestRefresh_MarkedFieldsPassedThrough(t *testing.T) {
	ci := &component.Info{}
	c := Component{ci: ci}

	c.MarkRefresh("Name", "Count")

	if len(ci.MarkedFields) != 2 || ci.MarkedFields[0] != "Name" || ci.MarkedFields[1] != "Count" {
		t.Errorf("expected MarkedFields [Name Count], got %v", ci.MarkedFields)
	}
}

// --- RegisterComponent ---

func TestRegisterComponent_Valid(t *testing.T) {
	e := NewEngine()
	app := &testApp{}
	e.RegisterComponent("my-app", app)

	reg, ok := e.components["my-app"]
	if !ok {
		t.Fatal("expected 'my-app' to be registered")
	}
	if reg.Typ != reflect.TypeOf(testApp{}) {
		t.Errorf("expected type testApp, got %v", reg.Typ)
	}
	// Proto must point to the original prototype value
	if reg.Proto.Pointer() != reflect.ValueOf(app).Pointer() {
		t.Error("expected Proto to point to the original app instance")
	}
}

func TestRegisterComponent_NonPointer(t *testing.T) {
	if os.Getenv("TEST_FATAL_NONPOINTER") == "1" {
		e := NewEngine()
		e.RegisterComponent("my-app", testApp{}) // not a pointer → log.Fatal
		return
	}
	out := runSubprocess(t, "TestRegisterComponent_NonPointer", "TEST_FATAL_NONPOINTER")
	if !strings.Contains(out, "requires a pointer to a struct") {
		t.Errorf("expected error about pointer to struct, got: %s", out)
	}
}

func TestRegisterComponent_PointerToNonStruct(t *testing.T) {
	if os.Getenv("TEST_FATAL_NONSTRUCT") == "1" {
		e := NewEngine()
		n := 42
		e.RegisterComponent("my-app", &n) // pointer to int, not struct → log.Fatal
		return
	}
	out := runSubprocess(t, "TestRegisterComponent_PointerToNonStruct", "TEST_FATAL_NONSTRUCT")
	if !strings.Contains(out, "requires a pointer to a struct") {
		t.Errorf("expected error about pointer to struct, got: %s", out)
	}
}

func TestRegisterComponent_NoHyphen(t *testing.T) {
	if os.Getenv("TEST_FATAL_NOHYPHEN") == "1" {
		e := NewEngine()
		e.RegisterComponent("myapp", &testApp{}) // no hyphen → log.Fatal
		return
	}
	out := runSubprocess(t, "TestRegisterComponent_NoHyphen", "TEST_FATAL_NOHYPHEN")
	if !strings.Contains(out, "must contain a hyphen") {
		t.Errorf("expected error about hyphen, got: %s", out)
	}
}

func TestRegisterComponent_NoEmbed(t *testing.T) {
	if os.Getenv("TEST_FATAL_NOEMBED") == "1" {
		e := NewEngine()
		e.RegisterComponent("my-app", &noComponentApp{}) // no Component embed → log.Fatal
		return
	}
	out := runSubprocess(t, "TestRegisterComponent_NoEmbed", "TEST_FATAL_NOEMBED")
	if !strings.Contains(out, "must embed godom.Component") {
		t.Errorf("expected error about embedding Component, got: %s", out)
	}
}

// --- Mount ---

var testHTML = `<!DOCTYPE html><html><head><title>Test</title></head><body>
	<span g-text="Name">placeholder</span>
	<button g-click="Increment">+</button>
</body></html>`

func makeTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(testHTML)},
	}
}

func makeTestFSNested() fstest.MapFS {
	return fstest.MapFS{
		"ui/index.html": &fstest.MapFile{Data: []byte(testHTML)},
		"ui/style.css":  &fstest.MapFile{Data: []byte("body{}")},
	}
}

func TestMount_Valid(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.Mount(app, makeTestFS(), "index.html")

	if e.comp == nil {
		t.Fatal("expected comp to be set after Mount")
	}
	if e.comp.HTMLBody == "" {
		t.Error("expected HTMLBody to be set")
	}
	if e.comp.VDOMTemplates == nil {
		t.Fatal("expected VDOMTemplates to be parsed")
	}
	if len(e.comp.VDOMTemplates) == 0 {
		t.Error("expected at least one parsed VDOM template")
	}
	if e.staticFS == nil {
		t.Error("expected staticFS to be set")
	}
}

func TestMount_NestedEntryPath(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.Mount(app, makeTestFSNested(), "ui/index.html")

	if e.comp == nil {
		t.Fatal("expected comp to be set")
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

func TestMount_WiresComponentField(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.Mount(app, makeTestFS(), "index.html")

	// After Mount, app.Component.ci should be wired to the same Info as e.comp
	if app.Component.ci == nil {
		t.Fatal("expected Component.ci to be wired after Mount")
	}
	if app.Component.ci != e.comp {
		t.Error("expected Component.ci to point to the same Info as Engine.comp")
	}
}

func TestMount_SetsHTMLBodyFromTemplate(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.Mount(app, makeTestFS(), "index.html")

	if !strings.Contains(e.comp.HTMLBody, "g-text") {
		t.Error("expected HTMLBody to contain template directives")
	}
	if !strings.Contains(e.comp.HTMLBody, "g-click") {
		t.Error("expected HTMLBody to contain event directive")
	}
}

func TestMount_NonPointer(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_NONPTR") == "1" {
		e := NewEngine()
		e.Mount(testApp{}, makeTestFS(), "index.html")
		return
	}
	out := runSubprocess(t, "TestMount_NonPointer", "TEST_FATAL_MOUNT_NONPTR")
	if !strings.Contains(out, "requires a pointer to a struct") {
		t.Errorf("expected error about pointer to struct, got: %s", out)
	}
}

func TestMount_PointerToNonStruct(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_NONSTRUCT") == "1" {
		e := NewEngine()
		n := 42
		e.Mount(&n, makeTestFS(), "index.html")
		return
	}
	out := runSubprocess(t, "TestMount_PointerToNonStruct", "TEST_FATAL_MOUNT_NONSTRUCT")
	if !strings.Contains(out, "requires a pointer to a struct") {
		t.Errorf("expected error about pointer to struct, got: %s", out)
	}
}

func TestMount_NoEmbed(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_NOEMBED") == "1" {
		e := NewEngine()
		e.Mount(&noComponentApp{}, makeTestFS(), "index.html")
		return
	}
	out := runSubprocess(t, "TestMount_NoEmbed", "TEST_FATAL_MOUNT_NOEMBED")
	if !strings.Contains(out, "must embed godom.Component") {
		t.Errorf("expected error about embedding Component, got: %s", out)
	}
}

func TestMount_BadEntryPath(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_BADPATH") == "1" {
		e := NewEngine()
		e.Mount(&testApp{}, makeTestFS(), "nonexistent.html")
		return
	}
	out := runSubprocess(t, "TestMount_BadEntryPath", "TEST_FATAL_MOUNT_BADPATH")
	if !strings.Contains(out, "failed to read") {
		t.Errorf("expected error about failed to read, got: %s", out)
	}
}

func TestMount_ComponentExpansionError(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_EXPAND") == "1" {
		e := NewEngine()
		// Register a component but don't provide its HTML file in the FS
		e.RegisterComponent("child-widget", &testApp{})
		htmlWithComponent := `<!DOCTYPE html><html><head></head><body>
			<child-widget></child-widget>
		</body></html>`
		badFS := fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(htmlWithComponent)},
		}
		e.Mount(&testApp{}, badFS, "index.html")
		return
	}
	out := runSubprocess(t, "TestMount_ComponentExpansionError", "TEST_FATAL_MOUNT_EXPAND")
	if !strings.Contains(out, "expand components") && !strings.Contains(out, "child-widget") {
		t.Errorf("expected error about component expansion, got: %s", out)
	}
}

// --- Start ---

func TestMount_SetsValueAndType(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Bob", Count: 42}
	e.Mount(app, makeTestFS(), "index.html")

	// comp.Value should point to the same app instance
	if e.comp.Value.Pointer() != reflect.ValueOf(app).Pointer() {
		t.Error("expected comp.Value to point to the original app")
	}
	if e.comp.Typ != reflect.TypeOf(testApp{}) {
		t.Errorf("expected comp.Typ = testApp, got %v", e.comp.Typ)
	}
}

func TestMount_RefreshWorksAfterMount(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.Mount(app, makeTestFS(), "index.html")

	// After Mount, Refresh should not panic (ci is wired, but RefreshFn is nil until Start)
	app.Refresh()
}

func TestMount_ChildrenMapInitialized(t *testing.T) {
	e := NewEngine()
	app := &testApp{}
	e.Mount(app, makeTestFS(), "index.html")

	if e.comp.Children == nil {
		t.Error("expected Children map to be initialized")
	}
}

func TestMount_RegistryPassedThrough(t *testing.T) {
	e := NewEngine()
	e.RegisterComponent("child-comp", &testApp{})

	app := &testApp{}
	e.Mount(app, makeTestFS(), "index.html")

	if e.comp.Registry == nil {
		t.Fatal("expected Registry to be set")
	}
	if _, ok := e.comp.Registry["child-comp"]; !ok {
		t.Error("expected 'child-comp' in Registry")
	}
}

func TestMount_InvalidDirective(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_BADDIR") == "1" {
		e := NewEngine()
		// g-click references a method that doesn't exist on the struct
		badHTML := `<!DOCTYPE html><html><head></head><body>
			<button g-click="NonExistentMethod">click</button>
		</body></html>`
		badFS := fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(badHTML)},
		}
		e.Mount(&testApp{}, badFS, "index.html")
		return
	}
	out := runSubprocess(t, "TestMount_InvalidDirective", "TEST_FATAL_MOUNT_BADDIR")
	if !strings.Contains(out, "NonExistentMethod") {
		t.Errorf("expected error mentioning NonExistentMethod, got: %s", out)
	}
}

func TestStart_NoMount(t *testing.T) {
	e := NewEngine()
	err := e.Start()
	if err == nil {
		t.Fatal("expected error when calling Start without Mount")
	}
	if !strings.Contains(err.Error(), "no component mounted") {
		t.Errorf("expected 'no component mounted' error, got: %v", err)
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

