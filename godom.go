package godom

import (
	_ "embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"os"
	"path"
	"reflect"

	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/env"
	"github.com/anupshinde/godom/internal/middleware"
	"github.com/anupshinde/godom/internal/server"
	"github.com/anupshinde/godom/internal/utils"
	"github.com/anupshinde/godom/internal/vdom"
)

//go:embed internal/bridge/bridge.js
var bridgeJS string

//go:embed internal/proto/protobuf.min.js
var protobufMinJS string

//go:embed internal/proto/protocol.js
var protocolJS string

// Engine is the godom runtime. It registers components and plugins,
// mounts the root component, and starts the server.
type Engine struct {
	Port           int    // 0 = random available port
	Host           string // default "localhost"; set to "0.0.0.0" for network access
	NoAuth         bool   // disable token auth (default false = auth enabled)
	FixedAuthToken string // fixed auth token; empty = generate random token
	NoBrowser      bool   // don't open browser on start
	Quiet          bool   // suppress startup output
	DisableExecJS  bool   // disable ExecJS — server won't send, bridge won't execute

	comps      []*component.Info        // mounted components
	plugins    map[string][]string      // plugin name → JS scripts
	compIndex  map[interface{}]int      // comp pointer → index in comps slice
	registered map[string]*registration // instance name → registration (from Register)
	componentFS fs.FS                   // filesystem for component templates, set via SetFS
	userMux    *http.ServeMux           // custom mux from SetMux()
	muxOpts    *MuxOptions              // custom paths for /ws and /godom.js
	authFn     middleware.AuthFunc      // auth check; nil = no auth
	wsPath     string                   // resolved WebSocket path (from muxOpts or default)
	scriptPath string                   // resolved script path (from muxOpts or default)
}

// MuxOptions configures custom paths for godom's handlers when using SetMux.
type MuxOptions struct {
	WSPath     string // WebSocket endpoint path (default "/ws")
	ScriptPath string // godom.js script path (default "/godom.js")
}

// registration holds metadata for a component registered via Register().
type registration struct {
	name      string // instance name (e.g. "counter1")
	comp      interface{}
	entryPath string
}

// Component is embedded in user structs to make them godom components.
type Component struct {
	ci *component.Info
}

// MarkRefresh marks fields for surgical refresh. The actual refresh happens
// when Refresh() is called (either by the user or automatically by the
// framework after a method call). Multiple calls accumulate.
func (c *Component) MarkRefresh(fields ...string) {
	if c.ci == nil {
		return
	}
	c.ci.AddMarkedFields(fields...)
}

// ExecJS sends a JavaScript expression to all connected browsers for execution.
// The callback fires once per connected browser with the JSON-encoded result
// and an error string (empty on success).
//
// Example:
//
//	c.ExecJS("location.pathname", func(result json.RawMessage, err string) {
//	    var path string
//	    json.Unmarshal(result, &path)
//	})
func (c *Component) ExecJS(expr string, cb func(result []byte, err string)) {
	if c.ci == nil {
		return
	}
	c.ci.ExecJS(expr, cb)
}

// Refresh pushes updates to all connected browsers.
// If fields were marked via MarkRefresh(), only those bound nodes are patched.
// Otherwise, a full refresh is sent.
//
// Do not call Refresh inside methods invoked by browser events (e.g. g-click).
// The framework automatically refreshes after every method call, so calling
// Refresh there would result in a redundant double invocation.
// Use Refresh only from background goroutines (timers, tickers, async work).
func (c *Component) Refresh() {
	if c.ci == nil {
		return
	}
	if c.ci.EventCh != nil {
		c.ci.EventCh <- component.Event{Kind: component.RefreshKind}
	}
}

// NewEngine creates a new godom Engine.
func NewEngine() *Engine {
	return &Engine{
		Port:           env.Port(),
		Host:           env.Host(),
		NoAuth:         env.NoAuth(),
		FixedAuthToken: env.Token(),
		NoBrowser:      env.NoBrowser(),
		Quiet:          env.Quiet(),
		plugins:        make(map[string][]string),
		compIndex:      make(map[interface{}]int),
		registered:     make(map[string]*registration),
	}
}

// SetFS sets the shared UI filesystem for templates. When set, Register()
// uses this filesystem instead of requiring one per call.
func (a *Engine) SetFS(fsys fs.FS) {
	a.componentFS = fsys
}

// SetMux sets the HTTP mux. godom registers its handlers (/ws, /godom.js) on it.
// Must be called before Run().
func (a *Engine) SetMux(mux *http.ServeMux, opts *MuxOptions) {
	a.userMux = mux
	a.muxOpts = opts
}

// SetAuth sets a custom auth function. When set, godom uses it to protect
// /ws and (via AuthMiddleware/ListenAndServe) all routes. If not set and
// NoAuth is false, godom uses built-in token auth.
func (a *Engine) SetAuth(fn middleware.AuthFunc) {
	a.authFn = fn
}

// --- EngineConfig interface methods (used by internal/server) ---

func (a *Engine) Components() []*component.Info         { return a.comps }
func (a *Engine) PluginScripts() map[string][]string    { return a.plugins }
func (a *Engine) EmbeddedJS() (string, string, string)  { return bridgeJS, protobufMinJS, protocolJS }
func (a *Engine) Mux() *http.ServeMux                   { return a.userMux }
func (a *Engine) WebSocketPath() string                  { return a.wsPath }
func (a *Engine) GodomScriptPath() string                { return a.scriptPath }
func (a *Engine) Auth() middleware.AuthFunc              { return a.authFn }
func (a *Engine) ExecJSDisabled() bool                   { return a.DisableExecJS }

// RegisterPlugin registers a named plugin with one or more JS scripts.
func (a *Engine) RegisterPlugin(name string, scripts ...string) {
	a.plugins[name] = scripts
}

// Register registers a named component with a template. The name is used in
// g-component="name" attributes on elements in parent templates.
//
// Register uses the filesystem set via SetFS().
// The entryPath is relative to that filesystem (e.g. "ui/counter/index.html").
func (a *Engine) Register(name string, comp interface{}, entryPath string) {
	if name == "" {
		log.Fatal("godom: Register requires a non-empty name")
	}
	if name != "document.body" && !vdom.IsValidIdentifier(name) {
		log.Fatalf("godom: Register name %q must be a valid identifier (letters, digits, underscores; cannot start with a digit)", name)
	}

	if _, exists := a.registered[name]; exists {
		log.Fatalf("godom: component %q already registered", name)
	}

	// Each component instance can only be registered once because it holds
	// a single VDOM tree, bindings, and event channel. Registering the same
	// pointer twice would create two component.Info entries sharing one struct,
	// causing tree/binding conflicts. Use shared state via embedded pointers
	// instead (see examples/shared-state).
	if _, exists := a.compIndex[comp]; exists {
		// Find the existing registration name for this pointer
		var existingName string
		for regName, reg := range a.registered {
			if reg.comp == comp {
				existingName = regName
				break
			}
		}
		log.Fatalf("godom: Register %q failed — same instance already registered as %q", name, existingName)
	}

	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: Register requires a pointer to a struct")
	}
	if !embedsComponent(v.Elem().Type()) {
		log.Fatal("godom: registered struct must embed godom.Component")
	}

	if a.componentFS == nil {
		log.Fatal("godom: call SetFS() before Register()")
	}

	a.registered[name] = &registration{
		name:      name,
		comp:      comp,
		entryPath: entryPath,
	}

	ci := server.BuildComponentInfo(comp, a.componentFS, entryPath)
	ci.Value.Elem().FieldByName("Component").Set(reflect.ValueOf(Component{ci: ci}))
	a.comps = append(a.comps, ci)
	a.compIndex[comp] = len(a.comps) - 1
}

// Run initializes the component lifecycle, registers /ws and /godom.js handlers
// on the mux set via SetMux, and starts event processors.
// If GODOM_VALIDATE_ONLY=1 is set, Run() returns immediately after validation
// succeeds — useful for CI and pre-commit checks.
func (a *Engine) Run() error {
	if len(a.comps) == 0 {
		return fmt.Errorf("godom: no components registered — call Register() before Run()")
	}

	// Auto-wire registered components to their g-component targets.
	if len(a.registered) > 0 {
		a.autoWireComponents()
	}

	// Validate: every component must have a SlotName.
	for _, ci := range a.comps {
		if ci.SlotName == "" {
			log.Fatal("godom: every component must have a SlotName — use Register() to name components")
		}
	}

	if env.Bool("GODOM_VALIDATE_ONLY") {
		if !a.Quiet {
			fmt.Println("godom: validation passed")
		}
		os.Exit(0)
	}

	// Resolve MuxOptions paths.
	a.wsPath = "/ws"
	a.scriptPath = "/godom.js"
	if a.muxOpts != nil {
		if a.muxOpts.WSPath != "" {
			a.wsPath = a.muxOpts.WSPath
		}
		if a.muxOpts.ScriptPath != "" {
			a.scriptPath = a.muxOpts.ScriptPath
		}
	}

	// Set up auth: user-provided AuthFunc takes priority, then built-in token auth.
	if a.authFn == nil && !a.NoAuth {
		a.FixedAuthToken, a.authFn = middleware.TokenAuth()
	}

	return server.Run(a)
}

// ListenAndServe binds a port using the startup config (Port, Host), wraps
// the mux with auth middleware if enabled, prints the URL, opens the browser,
// and serves. Must be called after Run().
func (a *Engine) ListenAndServe() error {
	handler := middleware.Wrap(a.userMux, a.authFn)

	host := utils.GetURLHost(a.Host)

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, a.Port))
	if err != nil {
		return fmt.Errorf("godom: failed to listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	utils.PrintUrlQRAndOpen(host, port, a.NoAuth, a.FixedAuthToken, a.NoBrowser, a.Quiet)

	srv := &http.Server{Handler: handler}
	return srv.Serve(ln)
}

// QuickServe is the convenience path for single-component apps. It registers
// the component, creates a minimal page, sets up the mux, and serves.
//
// Example:
//
//	eng := godom.NewEngine()
//	eng.SetFS(ui)
//	log.Fatal(eng.QuickServe(&App{Step: 1}, "ui/index.html"))
func (a *Engine) QuickServe(comp interface{}, templateFile string) error {
	// Register as "document.body" — a special name that tells the bridge to render
	// directly into document.body instead of a g-component element.
	a.Register("document.body", comp, templateFile)

	// Use the component's HTML as the full page, inject godom.js before </body>.
	idx := a.compIndex[comp]
	pageHTML := strings.Replace(a.comps[idx].HTMLBody, "</body>", "<script src=\"/godom.js\"></script>\n</body>", 1)

	// Serve static files from the template's directory.
	dir := path.Dir(templateFile)
	var staticFS fs.FS
	if dir == "." {
		staticFS = a.componentFS
	} else {
		var err error
		staticFS, err = fs.Sub(a.componentFS, dir)
		if err != nil {
			return fmt.Errorf("godom: invalid template path %q: %w", templateFile, err)
		}
	}
	staticHandler := http.FileServer(http.FS(staticFS))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			staticHandler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(pageHTML))
	})

	a.SetMux(mux, nil)
	if err := a.Run(); err != nil {
		return err
	}

	return a.ListenAndServe()
}

// AuthMiddleware wraps an http.Handler with the configured auth function.
// If no auth is configured, returns the handler unwrapped.
// Must be called after Run().
func (a *Engine) AuthMiddleware(next http.Handler) http.Handler {
	return middleware.Wrap(next, a.authFn)
}

// Cleanup closes event channels so component goroutines exit.
// Call this when your server is shutting down.
func (a *Engine) Cleanup() {
	server.Cleanup(a.comps)
}

// autoWireComponents sets SlotName on each registered component's Info.
// The bridge uses it to find target elements with matching g-component attributes.
func (a *Engine) autoWireComponents() {
	for name, reg := range a.registered {
		idx, ok := a.compIndex[reg.comp]
		if !ok {
			log.Fatalf("godom: registered component %q not found in mounted components", name)
		}
		a.comps[idx].SlotName = name
	}
}

// embedsComponent checks if a struct type embeds godom.Component.
func embedsComponent(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == reflect.TypeOf(Component{}) {
			return true
		}
	}
	return false
}
