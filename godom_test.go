package godom

import (
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/middleware"
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

type childApp struct {
	Component
	Value string
}

var testHTML = `<!DOCTYPE html><html><head><title>Test</title></head><body>
	<span g-text="Name">placeholder</span>
	<button g-click="Increment">+</button>
</body></html>`

var childHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Value">placeholder</span></body></html>`

func makeTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte(testHTML)},
		"child/index.html": &fstest.MapFile{Data: []byte(childHTML)},
	}
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
	if e.compIndex == nil {
		t.Error("expected non-nil compIndex map")
	}
	if e.registered == nil {
		t.Error("expected non-nil registered map")
	}
	if len(e.comps) != 0 {
		t.Error("expected empty comps before Register")
	}
}

func TestNewEngine_ReadsEnvVars(t *testing.T) {
	t.Setenv("GODOM_PORT", "9090")
	t.Setenv("GODOM_HOST", "myhost")
	t.Setenv("GODOM_NO_AUTH", "1")
	t.Setenv("GODOM_TOKEN", "secret123")
	t.Setenv("GODOM_NO_BROWSER", "true")
	t.Setenv("GODOM_QUIET", "1")

	e := NewEngine()

	if e.Port != 9090 {
		t.Errorf("Port = %d, want 9090", e.Port)
	}
	if e.Host != "myhost" {
		t.Errorf("Host = %q, want \"myhost\"", e.Host)
	}
	if !e.NoAuth {
		t.Error("NoAuth should be true")
	}
	if e.FixedAuthToken != "secret123" {
		t.Errorf("FixedAuthToken = %q, want \"secret123\"", e.FixedAuthToken)
	}
	if !e.NoBrowser {
		t.Error("NoBrowser should be true")
	}
	if !e.Quiet {
		t.Error("Quiet should be true")
	}
}

func TestNewEngine_CodeOverridesEnv(t *testing.T) {
	t.Setenv("GODOM_PORT", "9090")
	t.Setenv("GODOM_HOST", "envhost")

	e := NewEngine()
	// Code overrides after NewEngine
	e.Port = 8080
	e.Host = "codehost"

	if e.Port != 8080 {
		t.Errorf("Port = %d, want 8080 (code should win)", e.Port)
	}
	if e.Host != "codehost" {
		t.Errorf("Host = %q, want \"codehost\" (code should win)", e.Host)
	}
}

func TestNewEngine_InvalidPortIgnored(t *testing.T) {
	t.Setenv("GODOM_PORT", "notanumber")

	e := NewEngine()

	if e.Port != 0 {
		t.Errorf("Port = %d, want 0 (invalid port should be ignored)", e.Port)
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

// --- Component.MarkRefresh ---

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
	if len(fields) != 2 || fields[0] != "Name" || fields[1] != "Count" {
		t.Errorf("expected [Name Count], got %v", fields)
	}

	// After drain, should be empty
	fields = ci.DrainMarkedFields()
	if len(fields) != 0 {
		t.Errorf("expected empty after drain, got %v", fields)
	}
}

// --- Component.Refresh ---

func TestRefresh_NilCI(t *testing.T) {
	c := Component{ci: nil}
	// Should not panic
	c.Refresh()
}

func TestRefresh_NilEventCh(t *testing.T) {
	c := Component{ci: &component.Info{}}
	// EventCh is nil — should not panic
	c.Refresh()
}

func TestRefresh_SendsToEventChannel(t *testing.T) {
	ch := make(chan component.Event, 1)
	c := Component{ci: &component.Info{EventCh: ch}}

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

// --- SetFS ---

func TestSetFS(t *testing.T) {
	e := NewEngine()
	fsys := makeTestFS()
	e.SetFS(fsys)

	if e.componentFS == nil {
		t.Error("expected componentFS to be set")
	}
}

// --- SetMux ---

func TestSetMux(t *testing.T) {
	e := NewEngine()
	mux := http.NewServeMux()
	e.SetMux(mux, nil)

	if e.userMux != mux {
		t.Error("expected userMux to be set")
	}
	if e.muxOpts != nil {
		t.Error("expected muxOpts to be nil")
	}
}

func TestSetMux_WithOptions(t *testing.T) {
	e := NewEngine()
	mux := http.NewServeMux()
	opts := &MuxOptions{WSPath: "/app/ws", ScriptPath: "/app/godom.js"}
	e.SetMux(mux, opts)

	if e.userMux != mux {
		t.Error("expected userMux to be set")
	}
	if e.muxOpts != opts {
		t.Error("expected muxOpts to be set")
	}
	if e.muxOpts.WSPath != "/app/ws" {
		t.Errorf("expected WSPath '/app/ws', got %q", e.muxOpts.WSPath)
	}
	if e.muxOpts.ScriptPath != "/app/godom.js" {
		t.Errorf("expected ScriptPath '/app/godom.js', got %q", e.muxOpts.ScriptPath)
	}
}

// --- SetAuth ---

func TestSetAuth(t *testing.T) {
	e := NewEngine()
	e.SetAuth(func(w http.ResponseWriter, r *http.Request) bool {
		return true
	})

	if e.authFn == nil {
		t.Fatal("expected authFn to be set")
	}
}

func TestSetAuth_OverridesTokenAuth(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	// Set custom auth before Run
	customCalled := false
	e.SetAuth(func(w http.ResponseWriter, r *http.Request) bool {
		customCalled = true
		return true
	})

	mux := http.NewServeMux()
	e.SetMux(mux, nil)
	e.Run()

	// authFn should be the custom one, not token auth
	if e.authFn == nil {
		t.Fatal("expected authFn to be set")
	}
	// Call it to verify it's our custom function
	e.authFn(nil, nil)
	if !customCalled {
		t.Error("expected custom auth function to be called, not token auth")
	}
}

// --- Register ---

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
	if ci.VDOMTemplates == nil || len(ci.VDOMTemplates) == 0 {
		t.Error("expected VDOMTemplates to be parsed")
	}
}

func TestRegister_WiresComponentField(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

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

func TestRegister_SetsValueAndType(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Bob", Count: 42}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	ci := e.comps[0]
	if ci.Value.Pointer() != reflect.ValueOf(app).Pointer() {
		t.Error("expected ci.Value to point to the original app")
	}
	if ci.Typ != reflect.TypeOf(testApp{}) {
		t.Errorf("expected ci.Typ = testApp, got %v", ci.Typ)
	}
}

func TestRegister_RefreshDoesNotPanic(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	e.SetFS(makeTestFS())
	e.Register("main", app, "index.html")

	// After Register, Refresh should not panic (ci is wired, but RefreshFn is nil until Run)
	app.Refresh()
}

func TestRegister_DocumentBody(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "root"}
	e.SetFS(makeTestFS())
	e.Register("document.body", app, "index.html")

	if len(e.comps) != 1 {
		t.Fatal("expected one component")
	}

	// SlotName is set by autoWireComponents (called during Run), not Register.
	e.autoWireComponents()

	if e.comps[0].SlotName != "document.body" {
		t.Errorf("expected SlotName 'document.body', got %q", e.comps[0].SlotName)
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
		e.Register("main", &testApp{}, "index.html")
		return
	}
	out := runSubprocess(t, "TestRegister_NoFS", "TEST_FATAL_REGISTER_NOFS")
	if !strings.Contains(out, "call SetFS() before Register()") {
		t.Errorf("expected SetFS error, got: %s", out)
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

func TestRegister_InvalidDirective(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_BADDIR") == "1" {
		e := NewEngine()
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

func TestRegister_DuplicateInstanceFatals(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_DUPINST") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		app := &testApp{}
		e.Register("first", app, "index.html")
		e.Register("second", app, "index.html") // same pointer
		return
	}
	out := runSubprocess(t, "TestRegister_DuplicateInstanceFatals", "TEST_FATAL_REGISTER_DUPINST")
	if !strings.Contains(out, "same instance already registered") {
		t.Errorf("expected duplicate instance error, got: %s", out)
	}
	if !strings.Contains(out, `"first"`) {
		t.Errorf("expected error to mention existing name 'first', got: %s", out)
	}
}

// --- Auto-wiring ---

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

func TestAutoWire_MultipleChildren(t *testing.T) {
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

// --- Run ---

func TestRun_NoComponents(t *testing.T) {
	e := NewEngine()
	mux := http.NewServeMux()
	e.SetMux(mux, nil)

	err := e.Run()
	if err == nil {
		t.Fatal("expected error when calling Run without Register")
	}
	if !strings.Contains(err.Error(), "no components registered") {
		t.Errorf("expected 'no components registered' error, got: %v", err)
	}
}

func TestRun_WithMuxOptions(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	mux := http.NewServeMux()
	e.SetMux(mux, &MuxOptions{WSPath: "/app/ws", ScriptPath: "/app/godom.js"})

	err := e.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify handlers were registered on custom paths
	// (mux doesn't expose handlers, but we can check it doesn't panic)
}

func TestRun_DisableExecJS(t *testing.T) {
	e := NewEngine()
	e.DisableExecJS = true
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	mux := http.NewServeMux()
	e.SetMux(mux, nil)

	err := e.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// The registered component's ci should have ExecJSDisabled set
	if !e.comps[0].ExecJSDisabled {
		t.Error("expected ExecJSDisabled to be set on component after Run with DisableExecJS")
	}
}

func TestRun_SetsUpAuth(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	mux := http.NewServeMux()
	e.SetMux(mux, nil)

	err := e.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// With NoAuth=false (default), Run should set up token auth
	if e.authFn == nil {
		t.Error("expected authFn to be set after Run with auth enabled")
	}
}

func TestRun_NoAuthSkipsAuth(t *testing.T) {
	e := NewEngine()
	e.NoAuth = true
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	mux := http.NewServeMux()
	e.SetMux(mux, nil)

	err := e.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// With NoAuth=true, authFn should remain nil
	if e.authFn != nil {
		t.Error("expected authFn to be nil when NoAuth is true")
	}
}

func TestRun_CustomAuthPreserved(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	customCalled := false
	e.SetAuth(func(w http.ResponseWriter, r *http.Request) bool {
		customCalled = true
		return true
	})

	mux := http.NewServeMux()
	e.SetMux(mux, nil)
	e.Run()

	// Custom auth should not be overwritten by Run
	e.authFn(nil, nil)
	if !customCalled {
		t.Error("expected custom auth to be preserved after Run")
	}
}

// --- AuthMiddleware ---

func TestAuthMiddleware_NoAuth(t *testing.T) {
	e := NewEngine()
	e.NoAuth = true
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	wrapped := e.AuthMiddleware(handler)

	// With no authFn, should return the handler unwrapped
	if reflect.ValueOf(wrapped).Pointer() != reflect.ValueOf(handler).Pointer() {
		t.Error("expected AuthMiddleware to return handler unwrapped when no authFn")
	}
}

// --- QuickServe ---

func TestQuickServe_RegistersDocumentBody(t *testing.T) {
	// QuickServe calls Register("document.body", ...) and sets up mux.
	// We can't fully test QuickServe (it blocks on ListenAndServe), but we can
	// verify the register + auto-wire path works for document.body.
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("document.body", &testApp{Name: "root"}, "index.html")
	e.autoWireComponents()

	if len(e.comps) != 1 {
		t.Fatal("expected one component")
	}
	if e.comps[0].SlotName != "document.body" {
		t.Errorf("expected SlotName='document.body', got %q", e.comps[0].SlotName)
	}
}

// --- Cleanup ---

var simpleHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Name">placeholder</span></body></html>`

type cleanableApp struct {
	Component
	Name    string
	cleaned bool
}

func (a *cleanableApp) Cleanup() {
	a.cleaned = true
}

func TestCleanup_CallsComponentCleanup(t *testing.T) {
	e := NewEngine()
	e.SetFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(simpleHTML)},
	})
	app := &cleanableApp{Name: "test"}
	e.Register("main", app, "index.html")

	// Wire up event channel (normally done by Run/server)
	e.comps[0].EventCh = make(chan component.Event, 1)

	e.Cleanup()

	if !app.cleaned {
		t.Error("expected Cleanup() to be called on the component")
	}
}

func TestCleanup_ClosesEventChannels(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	ch := make(chan component.Event, 1)
	e.comps[0].EventCh = ch

	e.Cleanup()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected event channel to be closed after Cleanup")
	}
}

func TestCleanup_SkipsComponentWithoutCleanupMethod(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	e.Register("main", &testApp{}, "index.html")

	e.comps[0].EventCh = make(chan component.Event, 1)

	// Should not panic — testApp has no Cleanup method
	e.Cleanup()
}

// --- ExecJS ---

func TestExecJS_NilCI(t *testing.T) {
	c := Component{ci: nil}
	// Should not panic
	c.ExecJS("test", func(result []byte, err string) {
		t.Error("callback should not be called when ci is nil")
	})
}

func TestExecJS_DelegatesToInfo(t *testing.T) {
	ci := &component.Info{}
	called := false
	ci.ExecJSFn = func(id int32, expr string) {
		called = true
		if expr != "location.href" {
			t.Errorf("expected expr 'location.href', got %q", expr)
		}
	}

	c := Component{ci: ci}
	c.ExecJS("location.href", func(result []byte, err string) {})

	if !called {
		t.Error("expected ExecJSFn to be called")
	}
}

func TestExecJS_DisabledReturnsError(t *testing.T) {
	ci := &component.Info{ExecJSDisabled: true}
	c := Component{ci: ci}

	var gotErr string
	c.ExecJS("test", func(result []byte, err string) {
		gotErr = err
	})

	if gotErr != "ExecJS is disabled" {
		t.Errorf("expected disabled error, got %q", gotErr)
	}
}

// --- Helpers ---

// runSubprocess re-runs the named test in a subprocess with the given env var set to "1".
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

// Ensure middleware.AuthFunc is usable (compile check).
var _ middleware.AuthFunc = func(w http.ResponseWriter, r *http.Request) bool { return true }
