package godom

import (
	_ "embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"reflect"
	"strings"

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
	comp       *component.Info
	components map[string]*component.Reg // tag name → registered component
	plugins    map[string][]string       // plugin name → JS scripts
	staticFS   fs.FS                     // embedded UI filesystem for static assets
}

// Component is embedded in user structs to make them godom components.
type Component struct {
	ci *component.Info
}

// Refresh pushes updates to all connected browsers.
// With field names: surgical update — only the bound nodes for those fields are patched.
// Without arguments: full refresh — re-sends the entire tree.
func (c Component) Refresh(fields ...string) {
	if c.ci == nil {
		return
	}
	if c.ci.RefreshFn != nil {
		c.ci.RefreshFn(fields...)
	}
}

// NewEngine creates a new godom Engine.
func NewEngine() *Engine {
	return &Engine{
		components: make(map[string]*component.Reg),
		plugins:    make(map[string][]string),
	}
}

// RegisterPlugin registers a named plugin with one or more JS scripts.
func (a *Engine) RegisterPlugin(name string, scripts ...string) {
	a.plugins[name] = scripts
}

// RegisterComponent registers a stateful component struct for a custom element tag.
// The tag must contain a hyphen (e.g., "todo-item").
// The comp argument must be a pointer to a struct that embeds godom.Component.
func (a *Engine) RegisterComponent(tag string, comp interface{}) {
	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatalf("godom: Component(%q) requires a pointer to a struct", tag)
	}

	t := v.Elem().Type()

	if !strings.Contains(tag, "-") {
		log.Fatalf("godom: Component tag %q must contain a hyphen", tag)
	}

	if !embedsComponent(t) {
		log.Fatalf("godom: Component(%q) struct must embed godom.Component", tag)
	}

	a.components[tag] = &component.Reg{
		Typ:   t,
		Proto: v,
	}
}

// Mount registers a component struct with an embedded filesystem containing HTML templates.
func (a *Engine) Mount(comp interface{}, fsys fs.FS) {
	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: Mount requires a pointer to a struct")
	}

	t := v.Elem().Type()

	if !embedsComponent(t) {
		log.Fatal("godom: mounted struct must embed godom.Component")
	}

	root, err := template.FindIndexHTML(fsys)
	if err != nil {
		log.Fatalf("godom: %v", err)
	}

	a.staticFS = root

	indexHTML, err := fs.ReadFile(root, "index.html")
	if err != nil {
		log.Fatalf("godom: failed to read index.html: %v", err)
	}

	composed, err := template.ExpandComponents(string(indexHTML), root, a.components)
	if err != nil {
		log.Fatalf("godom: failed to expand components: %v", err)
	}

	ci := &component.Info{
		Value:    v,
		Typ:      t,
		HTMLBody: composed,
		Children: make(map[string][]*component.Info),
		Registry: a.components,
	}

	if err := template.ValidateDirectives(composed, ci); err != nil {
		log.Fatalf("godom: %v", err)
	}

	componentTags := make(map[string]bool)
	for tag := range a.components {
		componentTags[tag] = true
	}
	templates, err := vdom.ParseTemplate(composed, componentTags)
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
