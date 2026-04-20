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

	"github.com/anupshinde/godom/internal/island"
	"github.com/anupshinde/godom/internal/env"
	"github.com/anupshinde/godom/internal/middleware"
	"github.com/anupshinde/godom/internal/server"
	"github.com/anupshinde/godom/internal/template"
	"github.com/anupshinde/godom/internal/utils"
	"github.com/anupshinde/godom/internal/vdom"
)

//go:embed internal/bridge/bridge.js
var bridgeJS string

//go:embed internal/proto/protobuf.min.js
var protobufMinJS string

//go:embed internal/proto/protocol.js
var protocolJS string

//go:embed internal/bridge/disconnect.html
var defaultDisconnectHTML string

//go:embed internal/bridge/disconnect-badge.html
var defaultDisconnectBadgeHTML string

//go:embed internal/bridge/favicon.svg
var defaultFaviconSVG string

// Engine is the godom runtime. It registers islands and plugins,
// mounts the root island, and starts the server.
type Engine struct {
	Port           int    // 0 = random available port
	Host           string // default "localhost"; set to "0.0.0.0" for network access
	NoAuth         bool   // disable token auth (default false = auth enabled)
	FixedAuthToken string // fixed auth token; empty = generate random token
	NoBrowser      bool   // don't open browser on start
	Quiet          bool   // suppress startup output
	DisableExecJS  bool   // disable ExecJS — server won't send, bridge won't execute
	DisconnectHTML      string // custom disconnect overlay HTML (root mode); empty = default
	DisconnectBadgeHTML string // custom disconnect badge HTML (embedded mode); empty = default

	islands    []*island.Info           // mounted islands
	plugins    map[string][]string      // plugin name → JS scripts
	islIndex   map[interface{}]int      // island pointer → index in islands slice
	names      map[string]bool          // registered target names (for duplicate check)
	sharedFS   fs.FS                    // default UI filesystem, set via SetFS; used when an island has no AssetsFS
	partials   map[string]string        // named shared-partial registry (RegisterPartial / UsePartials)
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

// Island is embedded in user structs to make them godom islands.
// An island is a self-contained, stateful unit: its own goroutine, event
// queue, and VDOM tree. This is islands-architecture terminology — what
// other frameworks call "component" at the page level. See docs/why-islands.md.
//
// Template sources, in order of precedence:
//   - TemplateHTML set: inline HTML is used directly (no filesystem involved).
//   - Template + AssetsFS set: read Template from AssetsFS.
//   - Template + engine's SetFS: read Template from the engine's shared FS.
type Island struct {
	TargetName   string // matches g-island="name" attributes in parent templates
	Template     string // template path (resolved against AssetsFS or engine's SetFS)
	TemplateHTML string // inline HTML; mutually exclusive with Template/AssetsFS
	AssetsFS     fs.FS  // per-island filesystem for Template + sibling partials
	ci           *island.Info
}

// MarkRefresh marks fields for surgical refresh. The actual refresh happens
// when Refresh() is called (either by the user or automatically by the
// framework after a method call). Multiple calls accumulate.
func (c *Island) MarkRefresh(fields ...string) {
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
func (c *Island) ExecJS(expr string, cb func(result []byte, err string)) {
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
func (c *Island) Refresh() {
	if c.ci == nil {
		return
	}
	if c.ci.EventCh != nil {
		c.ci.EventCh <- island.Event{Kind: island.RefreshKind}
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
		islIndex:       make(map[interface{}]int),
		names:          make(map[string]bool),
		partials:       make(map[string]string),
	}
}

// SetFS sets the default UI filesystem for island templates. It is used by
// Register() when an island does not carry its own AssetsFS. Optional — if
// every island brings its own AssetsFS or uses TemplateHTML, SetFS can be
// skipped.
func (a *Engine) SetFS(fsys fs.FS) {
	a.sharedFS = fsys
}

// RegisterPartial registers a shared partial by name. When a template uses
// <my-button>, the engine resolves it by looking in (1) the island's local
// FS at the entry's baseDir, then (2) this registry. Use this for reusable
// HTML fragments shared across islands.
//
// Example:
//
//	eng.RegisterPartial("brand-logo", `<img src="/logo.svg" alt="brand"/>`)
func (a *Engine) RegisterPartial(name, html string) {
	if a.partials == nil {
		a.partials = make(map[string]string)
	}
	a.partials[name] = html
}

// UsePartials bulk-registers partials from a filesystem. It scans baseDir
// inside fsys for *.html files and calls RegisterPartial(basename, content)
// for each. Sugar over RegisterPartial for the embed-a-directory case.
//
// Example:
//
//	//go:embed partials
//	var partials embed.FS
//	eng.UsePartials(partials, "partials")
func (a *Engine) UsePartials(fsys fs.FS, baseDir string) {
	if fsys == nil {
		log.Fatal("godom: UsePartials requires a non-nil fs.FS")
	}
	entries, err := fs.ReadDir(fsys, baseDir)
	if err != nil {
		log.Fatalf("godom: UsePartials failed to read %s: %v", baseDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		base := strings.TrimSuffix(name, ".html")
		b, err := fs.ReadFile(fsys, path.Join(baseDir, name))
		if err != nil {
			log.Fatalf("godom: UsePartials failed to read %s: %v", name, err)
		}
		a.RegisterPartial(base, string(b))
	}
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

func (a *Engine) Islands() []*island.Info               { return a.islands }
func (a *Engine) PluginScripts() map[string][]string    { return a.plugins }
func (a *Engine) EmbeddedJS() (string, string, string)  { return bridgeJS, protobufMinJS, protocolJS }
func (a *Engine) Mux() *http.ServeMux                   { return a.userMux }
func (a *Engine) WebSocketPath() string                  { return a.wsPath }
func (a *Engine) GodomScriptPath() string                { return a.scriptPath }
func (a *Engine) Auth() middleware.AuthFunc              { return a.authFn }
func (a *Engine) ExecJSDisabled() bool                   { return a.DisableExecJS }
func (a *Engine) GetDisconnectHTML() string {
	if a.DisconnectHTML != "" {
		return a.DisconnectHTML
	}
	return defaultDisconnectHTML
}
func (a *Engine) GetDisconnectBadgeHTML() string {
	if a.DisconnectBadgeHTML != "" {
		return a.DisconnectBadgeHTML
	}
	return defaultDisconnectBadgeHTML
}
func (a *Engine) GetFaviconSVG() string { return defaultFaviconSVG }

// PluginFunc sets up a plugin on an Engine.
type PluginFunc func(*Engine)

// Use registers one or more plugins with the engine.
func (a *Engine) Use(plugins ...PluginFunc) {
	for _, p := range plugins {
		p(a)
	}
}

// RegisterPlugin registers a named plugin with one or more JS scripts.
func (a *Engine) RegisterPlugin(name string, scripts ...string) {
	a.plugins[name] = scripts
}

// Register registers one or more islands. Each island must set TargetName and
// one of: TemplateHTML (inline), or Template+AssetsFS, or just Template with
// a shared SetFS() default on the engine.
func (a *Engine) Register(islands ...interface{}) {
	for _, isl := range islands {
		v := reflect.ValueOf(isl)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
			log.Fatal("godom: Register requires a pointer to a struct")
		}
		if !embedsIsland(v.Elem().Type()) {
			log.Fatal("godom: registered struct must embed godom.Island")
		}

		// Read Island fields.
		islField := v.Elem().FieldByName("Island")
		name := islField.FieldByName("TargetName").String()
		entryPath := islField.FieldByName("Template").String()
		inlineHTML := islField.FieldByName("TemplateHTML").String()
		var assetsFS fs.FS
		if f := islField.FieldByName("AssetsFS"); f.IsValid() && !f.IsNil() {
			assetsFS = f.Interface().(fs.FS)
		}

		if name == "" {
			log.Fatal("godom: Register requires Island.TargetName to be set")
		}
		if name != "document.body" && !vdom.IsValidIdentifier(name) {
			log.Fatalf("godom: Island.TargetName %q must be a valid identifier (letters, digits, underscores; cannot start with a digit)", name)
		}

		// Validate template source: exactly one mode.
		if inlineHTML != "" && (entryPath != "" || assetsFS != nil) {
			log.Fatalf("godom: island %q: TemplateHTML is mutually exclusive with Template/AssetsFS", name)
		}
		if inlineHTML == "" && entryPath == "" {
			log.Fatalf("godom: island %q: set either Template (path) or TemplateHTML (inline)", name)
		}

		if a.names[name] {
			log.Fatalf("godom: island %q already registered", name)
		}

		// Each island instance can only be registered once because it holds
		// a single VDOM tree, bindings, and event channel. Registering the same
		// pointer twice would create two island.Info entries sharing one struct,
		// causing tree/binding conflicts. Use shared state via embedded pointers
		// instead (see examples/shared-state).
		if idx, exists := a.islIndex[isl]; exists {
			log.Fatalf("godom: Register %q failed — same instance already registered as %q", name, a.islands[idx].SlotName)
		}

		// Resolve the entry HTML and the sibling-file FS layer.
		entryHTML, layers, err := resolveIslandSource(name, entryPath, inlineHTML, assetsFS, a.sharedFS)
		if err != nil {
			log.Fatal(err)
		}

		ci := server.BuildIslandInfo(isl, entryHTML, layers, a.partials)
		ci.SlotName = name

		// Preserve island-side fields on the embed after Register.
		islField.Set(reflect.ValueOf(Island{
			TargetName:   name,
			Template:     entryPath,
			TemplateHTML: inlineHTML,
			AssetsFS:     assetsFS,
			ci:           ci,
		}))
		a.islands = append(a.islands, ci)
		a.islIndex[isl] = len(a.islands) - 1
		a.names[name] = true
	}
}

// resolveIslandSource picks the template source and builds the FS layers for
// sibling-partial lookup. Returns the entry HTML string and ordered FSLayers.
func resolveIslandSource(name, entryPath, inlineHTML string, assetsFS, sharedFS fs.FS) (string, []template.FSLayer, error) {
	// Inline path: HTML is given directly, no FS needed for entry.
	if inlineHTML != "" {
		// Inline islands have no sibling FS; partial lookup goes straight
		// to the engine registry (threaded through BuildIslandInfo).
		return inlineHTML, nil, nil
	}

	// FS-based: prefer island's AssetsFS, fall back to engine SetFS.
	fsys := assetsFS
	layerLabel := fmt.Sprintf("island %q AssetsFS", name)
	if fsys == nil {
		fsys = sharedFS
		layerLabel = "engine SetFS"
	}
	if fsys == nil {
		return "", nil, fmt.Errorf("godom: island %q has no filesystem — set Island.AssetsFS or call SetFS()", name)
	}
	b, err := fs.ReadFile(fsys, entryPath)
	if err != nil {
		return "", nil, fmt.Errorf("godom: island %q: failed to read %s: %w", name, entryPath, err)
	}
	layers := []template.FSLayer{{FS: fsys, BaseDir: path.Dir(entryPath), Label: layerLabel}}
	return string(b), layers, nil
}

// Run initializes the island lifecycle, registers /ws and /godom.js handlers
// on the mux set via SetMux, and starts event processors.
// If GODOM_VALIDATE_ONLY=1 is set, Run() returns immediately after validation
// succeeds — useful for CI and pre-commit checks.
func (a *Engine) Run() error {
	if len(a.islands) == 0 {
		return fmt.Errorf("godom: no islands registered — call Register() before Run()")
	}

	// Validate: every island must have a SlotName.
	for _, ci := range a.islands {
		if ci.SlotName == "" {
			log.Fatal("godom: every island must have a SlotName — use Register() to name islands")
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

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", a.Host, a.Port))
	if err != nil {
		return fmt.Errorf("godom: failed to listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	displayHost := utils.GetURLHost(a.Host)
	utils.PrintUrlQRAndOpen(displayHost, port, a.NoAuth, a.FixedAuthToken, a.NoBrowser, a.Quiet)

	srv := &http.Server{Handler: handler}
	return srv.Serve(ln)
}

// QuickServe is the convenience path for single-island apps. It registers
// the island as the root (document.body), creates a minimal page, sets up
// the mux, and serves. The island must have Template set before calling.
//
// Example:
//
//	app := &App{Step: 1}
//	app.Template = "ui/index.html"
//	eng := godom.NewEngine()
//	eng.SetFS(ui)
//	log.Fatal(eng.QuickServe(app))
func (a *Engine) QuickServe(isl interface{}) error {
	// Set TargetName to "document.body" — a special name that tells the bridge
	// to render directly into document.body instead of a g-island element.
	v := reflect.ValueOf(isl)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: QuickServe requires a pointer to a struct")
	}
	v.Elem().FieldByName("Island").FieldByName("TargetName").SetString("document.body")

	a.Register(isl)

	templateFile := v.Elem().FieldByName("Island").FieldByName("Template").String()

	// Use the island's HTML as the full page, inject godom.js before </body>.
	idx := a.islIndex[isl]
	pageHTML := strings.Replace(a.islands[idx].HTMLBody, "</body>", "<script src=\"/godom.js\"></script>\n</body>", 1)

	// Serve static files from the template's directory. Only available for
	// FS-based islands with a shared engine FS (QuickServe's common case).
	var staticFS fs.FS
	if a.sharedFS != nil && templateFile != "" {
		dir := path.Dir(templateFile)
		if dir == "." {
			staticFS = a.sharedFS
		} else {
			var err error
			staticFS, err = fs.Sub(a.sharedFS, dir)
			if err != nil {
				return fmt.Errorf("godom: invalid template path %q: %w", templateFile, err)
			}
		}
	}
	var staticHandler http.Handler
	if staticFS != nil {
		staticHandler = http.FileServer(http.FS(staticFS))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if staticHandler != nil {
				staticHandler.ServeHTTP(w, r)
			} else {
				http.NotFound(w, r)
			}
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

// Cleanup closes event channels so island goroutines exit.
// Call this when your server is shutting down.
func (a *Engine) Cleanup() {
	server.Cleanup(a.islands)
}

// embedsIsland checks if a struct type embeds godom.Island.
func embedsIsland(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == reflect.TypeOf(Island{}) {
			return true
		}
	}
	return false
}
