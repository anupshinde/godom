package godom

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/anupshinde/godom/internal/island"
	"github.com/anupshinde/godom/internal/middleware"
)

// --- test app structs ---

type testApp struct {
	Island
	Name  string
	Count int
}

func (a *testApp) Increment() {
	a.Count++
}

// noIslandApp does NOT embed godom.Island
type noIslandApp struct {
	Name string
}

type childApp struct {
	Island
	Value string
}

var testHTML = `<!DOCTYPE html><html><head><title>Test</title></head><body>
	<span g-text="Name">placeholder</span>
	<button g-click="Increment">+</button>
</body></html>`

var childHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Value">placeholder</span></body></html>`

func makeTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte(testHTML)},
		"child/index.html":  &fstest.MapFile{Data: []byte(childHTML)},
		"child/style.css":   &fstest.MapFile{Data: []byte("body { color: red; }")},
		"assets/readme.txt": &fstest.MapFile{Data: []byte("static asset")},
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
	if e.islIndex == nil {
		t.Error("expected non-nil compIndex map")
	}
	if e.names == nil {
		t.Error("expected non-nil names map")
	}
	if len(e.islands) != 0 {
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

// --- Use (PluginFunc) ---

func TestUse_Single(t *testing.T) {
	e := NewEngine()
	p := PluginFunc(func(eng *Engine) {
		eng.RegisterPlugin("myplugin", "plugin.js")
	})
	e.Use(p)

	scripts, ok := e.plugins["myplugin"]
	if !ok {
		t.Fatal("expected plugin 'myplugin' to be registered via Use")
	}
	if len(scripts) != 1 || scripts[0] != "plugin.js" {
		t.Errorf("unexpected scripts: %v", scripts)
	}
}

func TestUse_Multiple(t *testing.T) {
	e := NewEngine()
	p1 := PluginFunc(func(eng *Engine) {
		eng.RegisterPlugin("alpha", "a.js")
	})
	p2 := PluginFunc(func(eng *Engine) {
		eng.RegisterPlugin("beta", "b.js")
	})
	e.Use(p1, p2)

	if _, ok := e.plugins["alpha"]; !ok {
		t.Error("expected plugin 'alpha' to be registered")
	}
	if _, ok := e.plugins["beta"]; !ok {
		t.Error("expected plugin 'beta' to be registered")
	}
}

// --- embedsIsland ---

func TestEmbedsComponent_True(t *testing.T) {
	typ := reflect.TypeOf(testApp{})
	if !embedsIsland(typ) {
		t.Error("expected testApp to embed Island")
	}
}

func TestEmbedsComponent_False(t *testing.T) {
	typ := reflect.TypeOf(noIslandApp{})
	if embedsIsland(typ) {
		t.Error("expected noIslandApp to NOT embed Island")
	}
}

func TestEmbedsComponent_EmptyStruct(t *testing.T) {
	type empty struct{}
	typ := reflect.TypeOf(empty{})
	if embedsIsland(typ) {
		t.Error("expected empty struct to NOT embed Island")
	}
}

// --- Island.MarkRefresh ---

func TestMarkRefresh_NilCI(t *testing.T) {
	c := Island{ci: nil}
	// Should not panic
	c.MarkRefresh("Name")
}

func TestMarkRefresh_Accumulates(t *testing.T) {
	ci := &island.Info{}
	c := Island{ci: ci}

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

// --- Island.Refresh ---

func TestRefresh_NilCI(t *testing.T) {
	c := Island{ci: nil}
	// Should not panic
	c.Refresh()
}

func TestRefresh_NilEventCh(t *testing.T) {
	c := Island{ci: &island.Info{}}
	// EventCh is nil — should not panic
	c.Refresh()
}

func TestRefresh_SendsToEventChannel(t *testing.T) {
	ch := make(chan island.Event, 1)
	c := Island{ci: &island.Info{EventCh: ch}}

	c.Refresh()

	select {
	case evt := <-ch:
		if evt.Kind != island.RefreshKind {
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

	if e.sharedFS == nil {
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
	app := &testApp{Name: "test"}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

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
	app.TargetName = "main"
	app.Template = "index.html"
	e.SetFS(makeTestFS())
	e.Register(app)

	if len(e.islands) != 1 {
		t.Fatal("expected one component after Register")
	}
	ci := e.islands[0]
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
	app.TargetName = "main"
	app.Template = "index.html"
	e.SetFS(makeTestFS())
	e.Register(app)

	if app.Island.ci == nil {
		t.Fatal("expected Island.ci to be wired after Register")
	}
	if app.Island.ci != e.islands[0] {
		t.Error("expected Island.ci to point to the same Info as Engine.islands[0]")
	}
}

func TestRegister_SetsHTMLBodyFromTemplate(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "Alice"}
	app.TargetName = "main"
	app.Template = "index.html"
	e.SetFS(makeTestFS())
	e.Register(app)

	ci := e.islands[0]
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
	app.TargetName = "main"
	app.Template = "index.html"
	e.SetFS(makeTestFS())
	e.Register(app)

	ci := e.islands[0]
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
	app.TargetName = "main"
	app.Template = "index.html"
	e.SetFS(makeTestFS())
	e.Register(app)

	// After Register, Refresh should not panic (ci is wired, but RefreshFn is nil until Run)
	app.Refresh()
}

func TestRegister_DocumentBody(t *testing.T) {
	e := NewEngine()
	app := &testApp{Name: "root"}
	app.TargetName = "document.body"
	app.Template = "index.html"
	e.SetFS(makeTestFS())
	e.Register(app)

	if len(e.islands) != 1 {
		t.Fatal("expected one component")
	}

	// SlotName is set directly by Register.
	if e.islands[0].SlotName != "document.body" {
		t.Errorf("expected SlotName 'document.body', got %q", e.islands[0].SlotName)
	}
}

func TestRegister_NonPointer(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_NONPTR") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		e.Register(testApp{})
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
		e.Register(&n)
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
		e.Register(&noIslandApp{})
		return
	}
	out := runSubprocess(t, "TestRegister_NoEmbed", "TEST_FATAL_REGISTER_NOEMBED")
	if !strings.Contains(out, "must embed godom.Island") {
		t.Errorf("expected error about embedding Island, got: %s", out)
	}
}

func TestRegister_EmptyName(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_EMPTY") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		app := &testApp{}
		app.Template = "index.html"
		e.Register(app)
		return
	}
	out := runSubprocess(t, "TestRegister_EmptyName", "TEST_FATAL_REGISTER_EMPTY")
	if !strings.Contains(out, "Island.TargetName to be set") {
		t.Errorf("expected TargetName error, got: %s", out)
	}
}

func TestRegister_InvalidIdentifier(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_BADID") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		app := &testApp{}
		app.TargetName = "123invalid"
		app.Template = "index.html"
		e.Register(app)
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
		app := &testApp{}
		app.TargetName = "main"
		app.Template = "index.html"
		e.Register(app)
		return
	}
	out := runSubprocess(t, "TestRegister_NoFS", "TEST_FATAL_REGISTER_NOFS")
	if !strings.Contains(out, "has no filesystem") {
		t.Errorf("expected no-filesystem error, got: %s", out)
	}
}

func TestRegister_BadEntryPath(t *testing.T) {
	if os.Getenv("TEST_FATAL_REGISTER_BADPATH") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		app := &testApp{}
		app.TargetName = "main"
		app.Template = "nonexistent.html"
		e.Register(app)
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
		app1 := &testApp{}
		app1.TargetName = "main"
		app1.Template = "index.html"
		app2 := &testApp{}
		app2.TargetName = "main"
		app2.Template = "index.html"
		e.Register(app1)
		e.Register(app2)
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
		app := &testApp{}
		app.TargetName = "main"
		app.Template = "index.html"
		e.Register(app)
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
		app.TargetName = "first"
		app.Template = "index.html"
		e.Register(app)
		app.TargetName = "second"
		e.Register(app) // same pointer
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

// --- Register: SlotName ---

func TestRegister_SetsSlotName(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())

	child := &childApp{Value: "child"}
	child.TargetName = "sidebar"
	child.Template = "child/index.html"
	e.Register(child)

	if e.islands[0].SlotName != "sidebar" {
		t.Errorf("expected SlotName='sidebar', got %q", e.islands[0].SlotName)
	}
}

func TestRegister_SetsSlotNameMultiple(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())

	child1 := &childApp{Value: "c1"}
	child1.TargetName = "sidebar"
	child1.Template = "child/index.html"

	child2 := &childApp{Value: "c2"}
	child2.TargetName = "footer"
	child2.Template = "child/index.html"

	e.Register(child1, child2)

	if e.islands[0].SlotName != "sidebar" {
		t.Errorf("sidebar: expected SlotName='sidebar', got %q", e.islands[0].SlotName)
	}
	if e.islands[1].SlotName != "footer" {
		t.Errorf("footer: expected SlotName='footer', got %q", e.islands[1].SlotName)
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
	if !strings.Contains(err.Error(), "no islands registered") {
		t.Errorf("expected 'no islands registered' error, got: %v", err)
	}
}

func TestRun_WithMuxOptions(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

	mux := http.NewServeMux()
	e.SetMux(mux, &MuxOptions{WSPath: "/app/ws", ScriptPath: "/app/godom.js"})

	err := e.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if got := e.WebSocketPath(); got != "/app/ws" {
		t.Fatalf("WebSocketPath() = %q, want %q", got, "/app/ws")
	}
	if got := e.GodomScriptPath(); got != "/app/godom.js" {
		t.Fatalf("GodomScriptPath() = %q, want %q", got, "/app/godom.js")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app/godom.js", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /app/godom.js = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "/app/ws") {
		t.Fatalf("godom.js should embed custom ws path %q, body was %q", "/app/ws", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/godom.js", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /godom.js = %d, want 404 when custom script path is configured", rec.Code)
	}
}

func TestRun_DisableExecJS(t *testing.T) {
	e := NewEngine()
	e.DisableExecJS = true
	e.SetFS(makeTestFS())
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

	mux := http.NewServeMux()
	e.SetMux(mux, nil)

	err := e.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// The registered component's ci should have ExecJSDisabled set
	if !e.islands[0].ExecJSDisabled {
		t.Error("expected ExecJSDisabled to be set on component after Run with DisableExecJS")
	}
}

func TestRun_SetsUpAuth(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

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
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

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
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

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

func TestAuthMiddleware_WithAuthRejectsUnauthorized(t *testing.T) {
	e := NewEngine()
	e.SetAuth(func(w http.ResponseWriter, r *http.Request) bool { return false })

	wrapped := e.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("wrapped handler should not be called when auth rejects")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("AuthMiddleware status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Unauthorized") {
		t.Fatalf("AuthMiddleware body = %q, want Unauthorized message", rec.Body.String())
	}
}

// --- QuickServe ---

func TestQuickServe_RegistersDocumentBody(t *testing.T) {
	// QuickServe calls Register with TargetName="document.body" and sets up mux.
	// We can't fully test QuickServe (it blocks on ListenAndServe), but we can
	// verify the register path works for document.body.
	e := NewEngine()
	e.SetFS(makeTestFS())
	app := &testApp{Name: "root"}
	app.TargetName = "document.body"
	app.Template = "index.html"
	e.Register(app)

	if len(e.islands) != 1 {
		t.Fatal("expected one component")
	}
	if e.islands[0].SlotName != "document.body" {
		t.Errorf("expected SlotName='document.body', got %q", e.islands[0].SlotName)
	}
}

func TestQuickServe_BuildsMuxAndInjectsScriptBeforeListenError(t *testing.T) {
	e := NewEngine()
	e.Host = "127.0.0.1:1" // invalid host format once the port is appended
	e.NoBrowser = true
	e.NoAuth = true
	e.SetFS(makeTestFS())

	app := &testApp{Name: "root"}
	app.Template = "index.html"

	err := e.QuickServe(app)
	if err == nil {
		t.Fatal("expected QuickServe to return a listen error for invalid host")
	}
	if !strings.Contains(err.Error(), "failed to listen") {
		t.Fatalf("QuickServe error = %v, want wrapped listen error", err)
	}
	if app.TargetName != "document.body" {
		t.Fatalf("QuickServe should set TargetName to document.body, got %q", app.TargetName)
	}
	if e.Mux() == nil {
		t.Fatal("QuickServe should set a mux before ListenAndServe runs")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	e.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<script src="/godom.js"></script>`) {
		t.Fatalf("root page should include injected godom.js script, got %q", body)
	}
	if !strings.Contains(body, `g-text="Name"`) {
		t.Fatalf("root page should contain the component template, got %q", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/godom.js", nil)
	e.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /godom.js = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "/ws") {
		t.Fatalf("godom.js should reference the default ws path, got %q", body)
	}
}

func TestQuickServe_SubdirTemplateServesStaticFilesBeforeListenError(t *testing.T) {
	e := NewEngine()
	e.Host = "127.0.0.1:1" // invalid host format once the port is appended
	e.NoBrowser = true
	e.NoAuth = true
	e.SetFS(makeTestFS())

	app := &childApp{Value: "child"}
	app.Template = "child/index.html"

	err := e.QuickServe(app)
	if err == nil {
		t.Fatal("expected QuickServe to return a listen error for invalid host")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	e.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /style.css = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "color: red") {
		t.Fatalf("static file should come from the template directory, got %q", body)
	}
	if strings.Contains(body, "static asset") {
		t.Fatalf("static file should not come from the root template, got %q", body)
	}
}

func TestListenAndServe_InvalidHostReturnsWrappedError(t *testing.T) {
	e := NewEngine()
	e.Host = "127.0.0.1:1" // invalid host format once the port is appended
	e.NoBrowser = true
	e.SetMux(http.NewServeMux(), nil)

	err := e.ListenAndServe()
	if err == nil {
		t.Fatal("expected ListenAndServe to fail for invalid host")
	}
	if !strings.Contains(err.Error(), "failed to listen") {
		t.Fatalf("ListenAndServe error = %v, want wrapped listen error", err)
	}
}

// --- Cleanup ---

var simpleHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Name">placeholder</span></body></html>`

type cleanableApp struct {
	Island
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
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

	// Wire up event channel (normally done by Run/server)
	e.islands[0].EventCh = make(chan island.Event, 1)

	e.Cleanup()

	if !app.cleaned {
		t.Error("expected Cleanup() to be called on the component")
	}
}

func TestCleanup_ClosesEventChannels(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

	ch := make(chan island.Event, 1)
	e.islands[0].EventCh = ch

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
	app := &testApp{}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

	e.islands[0].EventCh = make(chan island.Event, 1)

	// Should not panic — testApp has no Cleanup method
	e.Cleanup()
}

// --- ExecJS ---

func TestExecJS_NilCI(t *testing.T) {
	c := Island{ci: nil}
	// Should not panic
	c.ExecJS("test", func(result []byte, err string) {
		t.Error("callback should not be called when ci is nil")
	})
}

func TestExecJS_DelegatesToInfo(t *testing.T) {
	ci := &island.Info{}
	called := false
	ci.ExecJSFn = func(id int32, expr string) {
		called = true
		if expr != "location.href" {
			t.Errorf("expected expr 'location.href', got %q", expr)
		}
	}

	c := Island{ci: ci}
	c.ExecJS("location.href", func(result []byte, err string) {})

	if !called {
		t.Error("expected ExecJSFn to be called")
	}
}

func TestExecJS_DisabledReturnsError(t *testing.T) {
	ci := &island.Info{ExecJSDisabled: true}
	c := Island{ci: ci}

	var gotErr string
	c.ExecJS("test", func(result []byte, err string) {
		gotErr = err
	})

	if gotErr != "ExecJS is disabled" {
		t.Errorf("expected disabled error, got %q", gotErr)
	}
}

// --- Engine getter methods ---

func TestComponents_ReturnsRegisteredComps(t *testing.T) {
	e := NewEngine()
	e.SetFS(makeTestFS())

	app := &testApp{Name: "Alice"}
	app.TargetName = "main"
	app.Template = "index.html"
	e.Register(app)

	comps := e.Islands()
	if len(comps) != 1 {
		t.Fatalf("expected 1 component, got %d", len(comps))
	}
	if comps[0].Typ.Name() != "testApp" {
		t.Errorf("expected Typ.Name()='testApp', got %q", comps[0].Typ.Name())
	}
}

func TestComponents_EmptyBeforeRegister(t *testing.T) {
	e := NewEngine()
	if len(e.Islands()) != 0 {
		t.Errorf("expected 0 components before register, got %d", len(e.Islands()))
	}
}

func TestPluginScripts_ReturnsRegisteredPlugins(t *testing.T) {
	e := NewEngine()
	e.RegisterPlugin("chartjs", "chart.min.js")

	plugins := e.PluginScripts()
	scripts, ok := plugins["chartjs"]
	if !ok {
		t.Fatal("expected 'chartjs' in plugin scripts")
	}
	if len(scripts) != 1 || scripts[0] != "chart.min.js" {
		t.Errorf("expected ['chart.min.js'], got %v", scripts)
	}
}

func TestPluginScripts_EmptyByDefault(t *testing.T) {
	e := NewEngine()
	if len(e.PluginScripts()) != 0 {
		t.Errorf("expected empty plugin scripts, got %d", len(e.PluginScripts()))
	}
}

func TestEmbeddedJS_ReturnsNonEmpty(t *testing.T) {
	e := NewEngine()
	bridge, protobuf, protocol := e.EmbeddedJS()
	if bridge == "" {
		t.Error("expected non-empty bridge JS")
	}
	if protobuf == "" {
		t.Error("expected non-empty protobuf JS")
	}
	if protocol == "" {
		t.Error("expected non-empty protocol JS")
	}
}

func TestMux_NilBeforeSetMux(t *testing.T) {
	e := NewEngine()
	if e.Mux() != nil {
		t.Error("expected nil mux before SetMux")
	}
}

func TestMux_ReturnsSetMux(t *testing.T) {
	e := NewEngine()
	mux := http.NewServeMux()
	e.SetMux(mux, nil)
	if e.Mux() != mux {
		t.Error("expected Mux() to return the mux passed to SetMux()")
	}
}

func TestWebSocketPath_EmptyBeforeRun(t *testing.T) {
	e := NewEngine()
	if e.WebSocketPath() != "" {
		t.Errorf("expected empty ws path before Run, got %q", e.WebSocketPath())
	}
}

func TestGodomScriptPath_EmptyBeforeRun(t *testing.T) {
	e := NewEngine()
	if e.GodomScriptPath() != "" {
		t.Errorf("expected empty script path before Run, got %q", e.GodomScriptPath())
	}
}

func TestAuth_NilByDefault(t *testing.T) {
	e := NewEngine()
	if e.Auth() != nil {
		t.Error("expected nil auth by default")
	}
}

func TestAuth_ReturnsSetAuth(t *testing.T) {
	e := NewEngine()
	fn := func(w http.ResponseWriter, r *http.Request) bool { return true }
	e.SetAuth(fn)
	if e.Auth() == nil {
		t.Error("expected non-nil auth after SetAuth")
	}
}

func TestExecJSDisabled_DefaultFalse(t *testing.T) {
	e := NewEngine()
	if e.ExecJSDisabled() {
		t.Error("expected ExecJSDisabled=false by default")
	}
}

func TestExecJSDisabled_ReflectsFlag(t *testing.T) {
	e := NewEngine()
	e.DisableExecJS = true
	if !e.ExecJSDisabled() {
		t.Error("expected ExecJSDisabled=true after setting flag")
	}
}

func TestGetDisconnectHTML_DefaultNonEmpty(t *testing.T) {
	e := NewEngine()
	html := e.GetDisconnectHTML()
	if html == "" {
		t.Error("expected non-empty default disconnect HTML")
	}
}

func TestGetDisconnectHTML_CustomOverridesDefault(t *testing.T) {
	e := NewEngine()
	custom := "<div>custom disconnect</div>"
	e.DisconnectHTML = custom
	if e.GetDisconnectHTML() != custom {
		t.Errorf("expected custom HTML %q, got %q", custom, e.GetDisconnectHTML())
	}
}

func TestGetDisconnectBadgeHTML_DefaultNonEmpty(t *testing.T) {
	e := NewEngine()
	html := e.GetDisconnectBadgeHTML()
	if html == "" {
		t.Error("expected non-empty default disconnect badge HTML")
	}
}

func TestGetDisconnectBadgeHTML_CustomOverridesDefault(t *testing.T) {
	e := NewEngine()
	custom := "<span>offline</span>"
	e.DisconnectBadgeHTML = custom
	if e.GetDisconnectBadgeHTML() != custom {
		t.Errorf("expected custom badge HTML %q, got %q", custom, e.GetDisconnectBadgeHTML())
	}
}

func TestGetFaviconSVG_NonEmpty(t *testing.T) {
	e := NewEngine()
	svg := e.GetFaviconSVG()
	if svg == "" {
		t.Error("expected non-empty favicon SVG")
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

// =============================================================================
// Phase B — AssetsFS, TemplateHTML, RegisterPartial, UsePartials
// =============================================================================

func TestRegister_PerIslandAssetsFS(t *testing.T) {
	// Island carries its own FS — no SetFS on the engine.
	e := NewEngine()
	islFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div g-text="Name">x</div>`)},
	}
	app := &testApp{Name: "hello"}
	app.TargetName = "solo"
	app.Template = "index.html"
	app.AssetsFS = islFS

	e.Register(app)

	if len(e.islands) != 1 {
		t.Fatalf("expected 1 island, got %d", len(e.islands))
	}
	if !strings.Contains(e.islands[0].HTMLBody, `g-text="Name"`) {
		t.Error("expected HTMLBody to come from per-island FS")
	}
}

func TestRegister_TemplateHTMLInline(t *testing.T) {
	// Island uses inline HTML — no FS at all.
	e := NewEngine()
	app := &testApp{Name: "hello"}
	app.TargetName = "inline"
	app.TemplateHTML = `<div g-text="Name">x</div>`

	e.Register(app)

	if len(e.islands) != 1 {
		t.Fatalf("expected 1 island, got %d", len(e.islands))
	}
	if !strings.Contains(e.islands[0].HTMLBody, `g-text="Name"`) {
		t.Error("expected HTMLBody to come from inline TemplateHTML")
	}
}

func TestRegister_TemplateHTMLAndTemplate_Rejected(t *testing.T) {
	if os.Getenv("TEST_FATAL_BOTH") == "1" {
		e := NewEngine()
		e.SetFS(makeTestFS())
		app := &testApp{}
		app.TargetName = "both"
		app.Template = "index.html"
		app.TemplateHTML = "<div></div>"
		e.Register(app)
		return
	}
	out := runSubprocess(t, "TestRegister_TemplateHTMLAndTemplate_Rejected", "TEST_FATAL_BOTH")
	if !strings.Contains(out, "mutually exclusive") {
		t.Errorf("expected mutual-exclusion error, got: %s", out)
	}
}

func TestRegister_NeitherTemplate_Rejected(t *testing.T) {
	if os.Getenv("TEST_FATAL_NEITHER") == "1" {
		e := NewEngine()
		app := &testApp{}
		app.TargetName = "neither"
		// no Template, no TemplateHTML
		e.Register(app)
		return
	}
	out := runSubprocess(t, "TestRegister_NeitherTemplate_Rejected", "TEST_FATAL_NEITHER")
	if !strings.Contains(out, "Template") && !strings.Contains(out, "TemplateHTML") {
		t.Errorf("expected template-required error, got: %s", out)
	}
}

func TestRegisterPartial_ResolvedInTemplate(t *testing.T) {
	// A shared partial registered by name; an island references it.
	e := NewEngine()
	e.RegisterPartial("my-badge", `<span class="badge" g-text="Name">?</span>`)

	app := &testApp{Name: "ok"}
	app.TargetName = "app"
	app.TemplateHTML = `<div><my-badge></my-badge></div>`

	e.Register(app)

	if !strings.Contains(e.islands[0].HTMLBody, `class="badge"`) {
		t.Errorf("expected partial expansion, got: %s", e.islands[0].HTMLBody)
	}
}

func TestUsePartials_BulkRegister(t *testing.T) {
	partialsFS := fstest.MapFS{
		"partials/my-badge.html": &fstest.MapFile{Data: []byte(`<span class="badge">badge</span>`)},
		"partials/my-card.html":  &fstest.MapFile{Data: []byte(`<div class="card">card</div>`)},
	}
	e := NewEngine()
	e.UsePartials(partialsFS, "partials")

	if e.partials["my-badge"] == "" {
		t.Error("expected my-badge to be registered")
	}
	if e.partials["my-card"] == "" {
		t.Error("expected my-card to be registered")
	}
	if !strings.Contains(e.partials["my-badge"], `class="badge"`) {
		t.Errorf("expected my-badge content, got: %s", e.partials["my-badge"])
	}
}

func TestRegisterPartial_LocalFSBeatsRegistry(t *testing.T) {
	// An island with a local sibling my-tag.html should win over a registered partial of the same name.
	e := NewEngine()
	e.RegisterPartial("my-tag", `<span>registry</span>`)

	islFS := fstest.MapFS{
		"index.html":  &fstest.MapFile{Data: []byte(`<div><my-tag></my-tag></div>`)},
		"my-tag.html": &fstest.MapFile{Data: []byte(`<span>local</span>`)},
	}
	app := &testApp{}
	app.TargetName = "app"
	app.Template = "index.html"
	app.AssetsFS = islFS

	e.Register(app)

	if !strings.Contains(e.islands[0].HTMLBody, "local") || strings.Contains(e.islands[0].HTMLBody, "registry") {
		t.Errorf("expected local FS to win over registry, got: %s", e.islands[0].HTMLBody)
	}
}

func TestTemplateHTML_UsesPartialRegistry(t *testing.T) {
	// Inline-HTML islands have no FS; partial resolution must use the registry.
	e := NewEngine()
	e.RegisterPartial("my-tag", `<span>from-registry</span>`)

	app := &testApp{}
	app.TargetName = "inline"
	app.TemplateHTML = `<div><my-tag></my-tag></div>`

	e.Register(app)

	if !strings.Contains(e.islands[0].HTMLBody, "from-registry") {
		t.Errorf("expected registry lookup for inline island, got: %s", e.islands[0].HTMLBody)
	}
}

func TestRegisterPartial_LazyInitOnBareEngine(t *testing.T) {
	// Using &Engine{} directly (skipping NewEngine) leaves partials == nil.
	// RegisterPartial must lazily initialize the map.
	e := &Engine{}
	e.RegisterPartial("x", "<span>ok</span>")
	if e.partials == nil {
		t.Fatal("partials map not lazily initialized")
	}
	if e.partials["x"] != "<span>ok</span>" {
		t.Errorf("expected partial stored, got: %v", e.partials)
	}
}

func TestUsePartials_SkipsSubdirsAndNonHTML(t *testing.T) {
	fsys := fstest.MapFS{
		"partials/my-badge.html":     &fstest.MapFile{Data: []byte(`<span class="badge">x</span>`)},
		"partials/README.md":         &fstest.MapFile{Data: []byte(`not a partial`)},
		"partials/notes.txt":         &fstest.MapFile{Data: []byte(`not a partial`)},
		"partials/sub/nested.html":   &fstest.MapFile{Data: []byte(`<span>nested</span>`)},
	}
	e := NewEngine()
	e.UsePartials(fsys, "partials")

	// Only my-badge.html should register; README.md and notes.txt are skipped
	// by suffix, and sub/ is a directory (ReadDir sees "sub", IsDir returns true).
	if _, ok := e.partials["my-badge"]; !ok {
		t.Error("expected my-badge to register")
	}
	if _, ok := e.partials["README"]; ok {
		t.Error("README.md should not have registered (non-html suffix)")
	}
	if _, ok := e.partials["notes"]; ok {
		t.Error("notes.txt should not have registered (non-html suffix)")
	}
	if _, ok := e.partials["sub"]; ok {
		t.Error("sub/ directory should not have registered (IsDir)")
	}
	if len(e.partials) != 1 {
		t.Errorf("expected exactly 1 partial registered, got %d: %v", len(e.partials), e.partials)
	}
}

func TestUsePartials_NilFS_Rejected(t *testing.T) {
	if os.Getenv("TEST_FATAL_USEPARTIALS_NIL") == "1" {
		e := NewEngine()
		e.UsePartials(nil, "partials")
		return
	}
	out := runSubprocess(t, "TestUsePartials_NilFS_Rejected", "TEST_FATAL_USEPARTIALS_NIL")
	if !strings.Contains(out, "non-nil fs.FS") {
		t.Errorf("expected nil-fs error, got: %s", out)
	}
}

func TestUsePartials_MissingDir_Rejected(t *testing.T) {
	if os.Getenv("TEST_FATAL_USEPARTIALS_NODIR") == "1" {
		e := NewEngine()
		e.UsePartials(fstest.MapFS{}, "does-not-exist")
		return
	}
	out := runSubprocess(t, "TestUsePartials_MissingDir_Rejected", "TEST_FATAL_USEPARTIALS_NODIR")
	if !strings.Contains(out, "failed to read") {
		t.Errorf("expected ReadDir-failed error, got: %s", out)
	}
}

func TestRegister_PerIslandAssetsFS_OverridesSetFS(t *testing.T) {
	// Engine has SetFS, but a particular island has AssetsFS — the island's
	// AssetsFS must win for that island's template resolution.
	engineFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div g-text="Name">engine</div>`)},
	}
	islandFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div g-text="Name">island</div>`)},
	}
	e := NewEngine()
	e.SetFS(engineFS)

	app := &testApp{Name: "hi"}
	app.TargetName = "app"
	app.Template = "index.html"
	app.AssetsFS = islandFS

	e.Register(app)

	if !strings.Contains(e.islands[0].HTMLBody, ">island<") {
		t.Errorf("expected AssetsFS to win over SetFS, got: %s", e.islands[0].HTMLBody)
	}
	if strings.Contains(e.islands[0].HTMLBody, ">engine<") {
		t.Error("engine FS should not have been read when AssetsFS is set")
	}
}

func TestRegister_TemplateHTMLIgnoresEngineSetFS(t *testing.T) {
	// When TemplateHTML is set, the engine's SetFS should not be consulted —
	// even if SetFS points at a filesystem, the inline HTML wins.
	e := NewEngine()
	e.SetFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div>engine</div>`)},
	})

	app := &testApp{Name: "hi"}
	app.TargetName = "inline"
	app.TemplateHTML = `<div g-text="Name">inline</div>`

	e.Register(app)

	if !strings.Contains(e.islands[0].HTMLBody, `g-text="Name"`) {
		t.Errorf("expected inline TemplateHTML used, got: %s", e.islands[0].HTMLBody)
	}
	if strings.Contains(e.islands[0].HTMLBody, "engine") {
		t.Error("engine FS should not have been read when TemplateHTML is set")
	}
}

func TestRegister_AssetsFS_ReadFileFailure(t *testing.T) {
	if os.Getenv("TEST_FATAL_BAD_ASSETSFS_PATH") == "1" {
		e := NewEngine()
		app := &testApp{}
		app.TargetName = "bad"
		app.Template = "missing.html"
		app.AssetsFS = fstest.MapFS{} // empty — no missing.html
		e.Register(app)
		return
	}
	out := runSubprocess(t, "TestRegister_AssetsFS_ReadFileFailure", "TEST_FATAL_BAD_ASSETSFS_PATH")
	if !strings.Contains(out, "failed to read") {
		t.Errorf("expected read-failure error, got: %s", out)
	}
}
