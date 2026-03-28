package godom

import (
	_ "embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"reflect"
	"strconv"
	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/env"
	"github.com/anupshinde/godom/internal/server"
	"github.com/anupshinde/godom/internal/template"
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
	Port       int    // 0 = random available port
	Host       string // default "localhost"; set to "0.0.0.0" for network access
	NoAuth     bool   // disable token auth (default false = auth enabled)
	Token      string // fixed auth token; empty = generate random token
	NoBrowser  bool   // don't open browser on start
	Quiet      bool   // suppress startup output
	NoGodomEnv bool   // skip reading GODOM_* environment variables for configuration
	comps      []*server.MountedComponent // mounted components
	plugins    map[string][]string        // plugin name → JS scripts
	staticFS   fs.FS                      // embedded UI filesystem for static assets
	compIndex  map[interface{}]int        // comp pointer → index in comps slice
	registered map[string]*registration   // instance name → registration (from Register)
	uiFS       fs.FS                      // shared UI filesystem set via SetUI
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
func (c Component) MarkRefresh(fields ...string) {
	if c.ci == nil {
		return
	}
	c.ci.Mu.Lock()
	c.ci.MarkedFields = append(c.ci.MarkedFields, fields...)
	c.ci.Mu.Unlock()
}

// Refresh pushes updates to all connected browsers.
// If fields were marked via MarkRefresh(), only those bound nodes are patched.
// Otherwise, a full refresh is sent.
//
// Do not call Refresh inside methods invoked by browser events (e.g. g-click).
// The framework automatically refreshes after every method call, so calling
// Refresh there would result in a redundant double invocation.
// Use Refresh only from background goroutines (timers, tickers, async work).
func (c Component) Refresh() {
	if c.ci == nil {
		return
	}
	if c.ci.RefreshFn != nil {
		c.ci.RefreshFn()
	}
}

// NewEngine creates a new godom Engine.
func NewEngine() *Engine {
	return &Engine{
		plugins:    make(map[string][]string),
		compIndex:  make(map[interface{}]int),
		registered: make(map[string]*registration),
	}
}

// SetUI sets the shared UI filesystem for templates. When set, Register()
// uses this filesystem instead of requiring one per call.
func (a *Engine) SetUI(fsys fs.FS) {
	a.uiFS = fsys
}

// RegisterPlugin registers a named plugin with one or more JS scripts.
func (a *Engine) RegisterPlugin(name string, scripts ...string) {
	a.plugins[name] = scripts
}

// Mount registers the root component with an embedded filesystem and HTML template.
// The entryPath is the path to the index.html file within fsys (e.g. "ui/index.html").
//
// For single-component apps, call Mount once. For multi-component apps, Mount the
// root component and use Register() for child components — they are auto-wired to
// parent templates based on <g-slot> tags.
func (a *Engine) Mount(comp interface{}, entryPath string) {
	if a.uiFS == nil {
		log.Fatal("godom: call SetUI() before Mount()")
	}
	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: Mount requires a pointer to a struct")
	}
	if !embedsComponent(v.Elem().Type()) {
		log.Fatal("godom: mounted struct must embed godom.Component")
	}

	a.mountInternal(comp, a.uiFS, entryPath)
}

// mountInternal is the shared mount logic used by both Mount and Register.
func (a *Engine) mountInternal(comp interface{}, fsys fs.FS, entryPath string) {
	v := reflect.ValueOf(comp)
	t := v.Elem().Type()

	// Derive static FS from the first mounted component's entry path.
	if a.staticFS == nil {
		dir := path.Dir(entryPath)
		if dir == "." {
			a.staticFS = fsys
		} else {
			sub, err := fs.Sub(fsys, dir)
			if err != nil {
				log.Fatalf("godom: invalid entry path %q: %v", entryPath, err)
			}
			a.staticFS = sub
		}
	}

	indexHTML, err := fs.ReadFile(fsys, entryPath)
	if err != nil {
		log.Fatalf("godom: failed to read %s: %v", entryPath, err)
	}

	composed, err := template.ExpandComponents(string(indexHTML), fsys, path.Dir(entryPath))
	if err != nil {
		log.Fatalf("godom: failed to expand components: %v", err)
	}

	ci := &component.Info{
		Value:    v,
		Typ:      t,
		HTMLBody: composed,
	}

	if err := template.ValidateDirectives(composed, ci); err != nil {
		log.Fatalf("godom: %v", err)
	}

	templates, err := vdom.ParseTemplate(composed)
	if err != nil {
		log.Fatalf("godom: failed to parse templates: %v", err)
	}
	ci.VDOMTemplates = templates

	ci.Value.Elem().FieldByName("Component").Set(reflect.ValueOf(Component{ci: ci}))

	idx := len(a.comps)
	a.comps = append(a.comps, &server.MountedComponent{Info: ci})
	a.compIndex[comp] = idx
}

// Register registers a named component with a template. The name is used in
// <g-slot type="component:Type" instance="name"> tags in parent templates.
//
// Register uses the filesystem set via SetUI() or the one from the first Mount() call.
// The entryPath is relative to that filesystem (e.g. "ui/counter/index.html").
func (a *Engine) Register(name string, comp interface{}, entryPath string) {
	if name == "" {
		log.Fatal("godom: Register requires a non-empty name")
	}
	if !vdom.IsValidIdentifier(name) {
		log.Fatalf("godom: Register name %q must be a valid identifier (letters, digits, underscores; cannot start with a digit)", name)
	}

	if _, exists := a.registered[name]; exists {
		log.Fatalf("godom: component %q already registered", name)
	}

	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: Register requires a pointer to a struct")
	}
	if !embedsComponent(v.Elem().Type()) {
		log.Fatal("godom: registered struct must embed godom.Component")
	}

	if a.uiFS == nil {
		log.Fatal("godom: call SetUI() or Mount() before Register()")
	}

	a.registered[name] = &registration{
		name:      name,
		comp:      comp,
		entryPath: entryPath,
	}

	// Mount the component internally
	a.mountInternal(comp, a.uiFS, entryPath)
}

// AddToSlot registers a child component to render into a named <g-slot> in the
// parent component's template. Both parent and child must already be mounted.
// Start starts the HTTP server, opens the default browser, and blocks forever.
// If GODOM_VALIDATE_ONLY=1 is set, Start() returns immediately after Mount() validation
// succeeds — useful for CI and pre-commit checks.
func (a *Engine) Start() error {
	if len(a.comps) == 0 {
		return fmt.Errorf("godom: no component mounted, call Mount() before Start()")
	}

	// Auto-wire registered components to their parents based on g-slot tags.
	if len(a.registered) > 0 {
		a.autoWireComponents()
	}

	// Ensure root component (SlotName="") is first — its DOM must exist
	// before child components can find their g-component target elements.
	for i, mc := range a.comps {
		if mc.SlotName == "" && i > 0 {
			a.comps[0], a.comps[i] = a.comps[i], a.comps[0]
			break
		}
	}

	if env.Bool("GODOM_VALIDATE_ONLY") {
		if !a.Quiet {
			fmt.Println("godom: validation passed")
		}
		os.Exit(0)
	}

	a.applyEnv()

	cfg := server.Config{
		Comps:         a.comps,
		Plugins:       a.plugins,
		StaticFS:      a.staticFS,
		Port:          a.Port,
		Host:          a.Host,
		NoAuth:        a.NoAuth,
		Token:         a.Token,
		NoBrowser:     a.NoBrowser,
		Quiet:         a.Quiet,
		BridgeJS:      bridgeJS,
		ProtobufMinJS: protobufMinJS,
		ProtocolJS:    protocolJS,
	}

	return server.Run(cfg)
}

// applyEnv reads GODOM_* environment variables for fields not set in code.
// Skipped entirely when NoGodomEnv is true.
func (a *Engine) applyEnv() {
	if a.NoGodomEnv {
		return
	}
	if a.Port == 0 {
		if v, err := strconv.Atoi(os.Getenv("GODOM_PORT")); err == nil && v != 0 {
			a.Port = v
		}
	}
	if a.Host == "" {
		if v := os.Getenv("GODOM_HOST"); v != "" {
			a.Host = v
		}
	}
	if !a.NoAuth {
		a.NoAuth = env.Bool("GODOM_NO_AUTH")
	}
	if a.Token == "" {
		if v := os.Getenv("GODOM_TOKEN"); v != "" {
			a.Token = v
		}
	}
	if !a.NoBrowser {
		a.NoBrowser = env.Bool("GODOM_NO_BROWSER")
	}
	if !a.Quiet {
		a.Quiet = env.Bool("GODOM_QUIET")
	}
}

// autoWireComponents sets SlotName on each registered component's MountedComponent.
// The SlotName is the registered instance name — the bridge uses it to find
// target elements with matching g-component attributes in the DOM.
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
