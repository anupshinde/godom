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
	if e.plugins == nil {
		t.Error("expected non-nil plugins map")
	}
	if len(e.comps) != 0 {
		t.Error("expected empty comps before Mount")
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

// --- Mount ---

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

func TestMount_Valid(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetUI(makeTestFS())
	e.Mount(app, "index.html")

	if len(e.comps) != 1 {
		t.Fatal("expected one component after Mount")
	}
	ci := e.comps[0].Info
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

func TestMount_NestedEntryPath(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetUI(makeTestFSNested())
	e.Mount(app, "ui/index.html")

	if len(e.comps) != 1 {
		t.Fatal("expected one component after Mount")
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
	e.SetUI(makeTestFS())
	e.Mount(app, "index.html")

	// After Mount, app.Component.ci should be wired to the same Info as e.comps[0].Info
	if app.Component.ci == nil {
		t.Fatal("expected Component.ci to be wired after Mount")
	}
	if app.Component.ci != e.comps[0].Info {
		t.Error("expected Component.ci to point to the same Info as Engine.comps[0].Info")
	}
}

func TestMount_SetsHTMLBodyFromTemplate(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetUI(makeTestFS())
	e.Mount(app, "index.html")

	ci := e.comps[0].Info
	if !strings.Contains(ci.HTMLBody, "g-text") {
		t.Error("expected HTMLBody to contain template directives")
	}
	if !strings.Contains(ci.HTMLBody, "g-click") {
		t.Error("expected HTMLBody to contain event directive")
	}
}

func TestMount_NonPointer(t *testing.T) {
	if os.Getenv("TEST_FATAL_MOUNT_NONPTR") == "1" {
		e := NewEngine()
		e.SetUI(makeTestFS())
		e.Mount(testApp{}, "index.html")
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
		e.SetUI(makeTestFS())
		e.Mount(&n, "index.html")
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
		e.SetUI(makeTestFS())
		e.Mount(&noComponentApp{}, "index.html")
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
		e.SetUI(makeTestFS())
		e.Mount(&testApp{}, "nonexistent.html")
		return
	}
	out := runSubprocess(t, "TestMount_BadEntryPath", "TEST_FATAL_MOUNT_BADPATH")
	if !strings.Contains(out, "failed to read") {
		t.Errorf("expected error about failed to read, got: %s", out)
	}
}

// --- Start ---

func TestMount_SetsValueAndType(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Bob", Count: 42}
	e.SetUI(makeTestFS())
	e.Mount(app, "index.html")

	ci := e.comps[0].Info
	// ci.Value should point to the same app instance
	if ci.Value.Pointer() != reflect.ValueOf(app).Pointer() {
		t.Error("expected ci.Value to point to the original app")
	}
	if ci.Typ != reflect.TypeOf(testApp{}) {
		t.Errorf("expected ci.Typ = testApp, got %v", ci.Typ)
	}
}

func TestMount_RefreshWorksAfterMount(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetUI(makeTestFS())
	e.Mount(app, "index.html")

	// After Mount, Refresh should not panic (ci is wired, but RefreshFn is nil until Start)
	app.Refresh()
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
		e.SetUI(badFS)
		e.Mount(&testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestMount_InvalidDirective", "TEST_FATAL_MOUNT_BADDIR")
	if !strings.Contains(out, "NonExistentMethod") {
		t.Errorf("expected error mentioning NonExistentMethod, got: %s", out)
	}
}

// --- AddToSlot ---

type childApp struct {
	Component
	Value string
}

var childHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Value">placeholder</span></body></html>`

func makeChildFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(childHTML)},
	}
}

func TestAddToSlot_Valid(t *testing.T) {
	e := NewEngine()
	parent := &testApp{Name: "parent"}
	child := &childApp{Value: "child"}

	e.SetUI(makeTestFS())
	e.Mount(parent, "index.html")
	e.Mount(child, "child/index.html")
	e.AddToSlot(parent, "sidebar", child)

	if len(e.comps) != 2 {
		t.Fatalf("expected 2 comps, got %d", len(e.comps))
	}
	if e.comps[1].ParentIdx != 0 {
		t.Errorf("expected child ParentIdx=0, got %d", e.comps[1].ParentIdx)
	}
	if e.comps[1].SlotName != "sidebar" {
		t.Errorf("expected SlotName='sidebar', got %q", e.comps[1].SlotName)
	}
}

func TestAddToSlot_UnmountedParent(t *testing.T) {
	if os.Getenv("TEST_FATAL_ADDSLOT_PARENT") == "1" {
		e := NewEngine()
		child := &childApp{Value: "child"}
		e.SetUI(makeTestFS())
		e.Mount(child, "child/index.html")
		e.AddToSlot(&testApp{}, "sidebar", child) // parent not mounted
		return
	}
	out := runSubprocess(t, "TestAddToSlot_UnmountedParent", "TEST_FATAL_ADDSLOT_PARENT")
	if !strings.Contains(out, "unmounted parent") {
		t.Errorf("expected 'unmounted parent' error, got: %s", out)
	}
}

func TestAddToSlot_UnmountedChild(t *testing.T) {
	if os.Getenv("TEST_FATAL_ADDSLOT_CHILD") == "1" {
		e := NewEngine()
		parent := &testApp{Name: "parent"}
		e.SetUI(makeTestFS())
		e.Mount(parent, "index.html")
		e.AddToSlot(parent, "sidebar", &childApp{}) // child not mounted
		return
	}
	out := runSubprocess(t, "TestAddToSlot_UnmountedChild", "TEST_FATAL_ADDSLOT_CHILD")
	if !strings.Contains(out, "unmounted child") {
		t.Errorf("expected 'unmounted child' error, got: %s", out)
	}
}

func TestAddToSlot_DuplicateSlot(t *testing.T) {
	if os.Getenv("TEST_FATAL_ADDSLOT_DUP") == "1" {
		e := NewEngine()
		parent := &testApp{Name: "parent"}
		child1 := &childApp{Value: "c1"}
		child2 := &childApp{Value: "c2"}

		e.SetUI(makeTestFS())
		e.Mount(parent, "index.html")
		e.Mount(child1, "child/index.html")
		e.Mount(child2, "child/index.html")

		e.AddToSlot(parent, "sidebar", child1)
		e.AddToSlot(parent, "sidebar", child2) // duplicate slot
		return
	}
	out := runSubprocess(t, "TestAddToSlot_DuplicateSlot", "TEST_FATAL_ADDSLOT_DUP")
	if !strings.Contains(out, "already has a component") {
		t.Errorf("expected 'already has a component' error, got: %s", out)
	}
}

func TestMount_MultipleComponents_StaticFSFromFirst(t *testing.T) {
	e := NewEngine()
	parent := &testApp{Name: "parent"}
	child := &childApp{Value: "child"}

	e.SetUI(makeTestFSNested())
	e.Mount(parent, "ui/index.html")
	e.Mount(child, "child/index.html")

	// staticFS should be derived from the first Mount call only
	if e.staticFS == nil {
		t.Fatal("expected staticFS to be set")
	}
	// It should be the "ui/" subdirectory from the first mount
	data, err := fs.ReadFile(e.staticFS, "style.css")
	if err != nil {
		t.Errorf("expected staticFS from first mount, got error: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

// TODO: Mount no longer accepts an FS parameter (uses SetUI instead),
// so the premise of this test (second Mount's FS not overwriting staticFS) is invalid.
// Revisit if per-component FS support is re-added.
func TestMount_SecondMountDoesNotOverwriteStaticFS(t *testing.T) {
	t.Skip("Mount no longer takes an FS argument; test premise is invalid")
}

func TestAddToSlot_SameSlotDifferentParents(t *testing.T) {
	// Two different parents can each have a child in the same slot name — no conflict.
	e := NewEngine()
	parent1 := &testApp{Name: "p1"}
	parent2 := &testApp{Name: "p2"}
	child1 := &childApp{Value: "c1"}
	child2 := &childApp{Value: "c2"}

	e.SetUI(makeTestFS())
	e.Mount(parent1, "index.html")
	e.Mount(parent2, "index.html")
	e.Mount(child1, "child/index.html")
	e.Mount(child2, "child/index.html")

	e.AddToSlot(parent1, "sidebar", child1)
	e.AddToSlot(parent2, "sidebar", child2) // same slot name, different parent — should be fine

	if e.comps[2].ParentIdx != 0 {
		t.Errorf("expected child1 parent=0, got %d", e.comps[2].ParentIdx)
	}
	if e.comps[3].ParentIdx != 1 {
		t.Errorf("expected child2 parent=1, got %d", e.comps[3].ParentIdx)
	}
}

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

