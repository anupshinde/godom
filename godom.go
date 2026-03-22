package godom

import (
	_ "embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"reflect"

	"github.com/anupshinde/godom/internal/component"
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
	comp    *component.Info
	plugins map[string][]string // plugin name → JS scripts
	staticFS   fs.FS                     // embedded UI filesystem for static assets
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
		plugins: make(map[string][]string),
	}
}

// RegisterPlugin registers a named plugin with one or more JS scripts.
func (a *Engine) RegisterPlugin(name string, scripts ...string) {
	a.plugins[name] = scripts
}

// Mount registers a component struct with an embedded filesystem containing HTML templates.
// The entryPath is the path to the index.html file within fsys (e.g. "ui/index.html").
func (a *Engine) Mount(comp interface{}, fsys fs.FS, entryPath string) {
	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: Mount requires a pointer to a struct")
	}

	t := v.Elem().Type()

	if !embedsComponent(t) {
		log.Fatal("godom: mounted struct must embed godom.Component")
	}

	// Derive the static FS root from the entry path's directory
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

	indexHTML, err := fs.ReadFile(fsys, entryPath)
	if err != nil {
		log.Fatalf("godom: failed to read %s: %v", entryPath, err)
	}

	composed, err := template.ExpandComponents(string(indexHTML), a.staticFS)
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

	a.comp = ci
}

// Start starts the HTTP server, opens the default browser, and blocks forever.
func (a *Engine) Start() error {
	if a.comp == nil {
		return fmt.Errorf("godom: no component mounted, call Mount() before Start()")
	}

	// Parse CLI flags using a separate FlagSet to avoid conflicts
	fs := flag.NewFlagSet("godom", flag.ContinueOnError)
	flagPort := fs.Int("port", 0, "port to listen on (0 = random)")
	flagHost := fs.String("host", "localhost", "host to bind to")
	flagNoAuth := fs.Bool("no-auth", false, "disable token authentication")
	flagToken := fs.String("token", "", "fixed auth token (default: random)")
	flagNoBrowser := fs.Bool("no-browser", false, "don't open browser on start")
	flagQuiet := fs.Bool("quiet", false, "suppress startup output")
	_ = fs.Parse(os.Args[1:])

	if a.Port == 0 && *flagPort != 0 {
		a.Port = *flagPort
	}
	if a.Host == "" && *flagHost != "localhost" {
		a.Host = *flagHost
	}
	if !a.NoAuth && *flagNoAuth {
		a.NoAuth = true
	}
	if a.Token == "" && *flagToken != "" {
		a.Token = *flagToken
	}
	if !a.NoBrowser && *flagNoBrowser {
		a.NoBrowser = true
	}
	if !a.Quiet && *flagQuiet {
		a.Quiet = true
	}

	return server.Run(server.Config{
		Comp:          a.comp,
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
	})
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
