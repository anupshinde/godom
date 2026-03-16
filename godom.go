package godom

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

//go:embed bridge.js
var bridgeJS string

//go:embed protobuf.min.js
var protobufMinJS string

//go:embed protocol.js
var protocolJS string

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// componentReg holds the registration info for a stateful component.
type componentReg struct {
	typ   reflect.Type  // the struct type (not pointer)
	proto reflect.Value // pointer to the prototype instance
}

// App is the main godom application.
type App struct {
	Port       int    // 0 = random available port
	Host       string // default "localhost"; set to "0.0.0.0" for network access
	NoAuth     bool   // disable token auth (default false = auth enabled)
	Token      string // fixed auth token; empty = generate random token
	NoBrowser  bool   // don't open browser on start
	Quiet      bool   // suppress startup output
	comp       *componentInfo
	components map[string]*componentReg // tag name → registered component
	plugins    map[string][]string       // plugin name → JS scripts
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

// New creates a new godom App.
func New() *App {
	return &App{
		components: make(map[string]*componentReg),
		plugins:    make(map[string][]string),
	}
}

// Plugin registers a named plugin with one or more JS scripts.
// Scripts are injected in order. The last script should call
// godom.register(name, {init, update}) to handle plugin commands.
func (a *App) Plugin(name string, scripts ...string) {
	a.plugins[name] = scripts
}

// Component registers a stateful component struct for a custom element tag.
// The tag must contain a hyphen (e.g., "todo-item").
// The comp argument must be a pointer to a struct that embeds godom.Component.
func (a *App) Component(tag string, comp interface{}) {
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

	a.components[tag] = &componentReg{
		typ:   t,
		proto: v,
	}
}

// Mount registers a component struct with an embedded filesystem containing HTML templates.
func (a *App) Mount(comp interface{}, fsys fs.FS) {
	v := reflect.ValueOf(comp)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		log.Fatal("godom: Mount requires a pointer to a struct")
	}

	t := v.Elem().Type()

	if !embedsComponent(t) {
		log.Fatal("godom: mounted struct must embed godom.Component")
	}

	// Find index.html in the filesystem
	root, err := findIndexHTML(fsys)
	if err != nil {
		log.Fatalf("godom: %v", err)
	}

	indexHTML, err := fs.ReadFile(root, "index.html")
	if err != nil {
		log.Fatalf("godom: failed to read index.html: %v", err)
	}

	// Expand custom element tags into their component HTML
	composed, err := expandComponents(string(indexHTML), root, a.components)
	if err != nil {
		log.Fatalf("godom: failed to expand components: %v", err)
	}

	ci := &componentInfo{
		value:    v,
		typ:      t,
		htmlBody: composed,
		children: make(map[string][]*componentInfo),
		registry: a.components,
	}

	// Validate all directives against the component struct (fail-fast).
	if err := validateDirectives(composed, ci); err != nil {
		log.Fatalf("godom: %v", err)
	}

	// Parse HTML: assign data-gid, extract bindings, replace g-for with anchors.
	pb, err := parsePageHTML(composed)
	if err != nil {
		log.Fatalf("godom: %v", err)
	}
	ci.pb = pb
	ci.htmlBody = pb.HTML // use the gid-annotated HTML

	// Set ci on the embedded Component so Refresh()/Emit() work
	// even if a goroutine starts before Start() is called.
	ci.value.Elem().FieldByName("Component").Set(reflect.ValueOf(Component{ci: ci}))

	a.comp = ci
}

// generateToken returns a cryptographically random hex token.
func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("godom: failed to generate auth token: %v", err)
	}
	return hex.EncodeToString(b)
}

// checkAuth validates the auth token from a cookie or query parameter.
// If the token is in the query parameter, it sets a cookie for future requests.
func checkAuth(token string, w http.ResponseWriter, r *http.Request) bool {
	// Check cookie first
	if c, err := r.Cookie("godom_token"); err == nil && c.Value == token {
		return true
	}
	// Check query parameter
	if r.URL.Query().Get("token") == token {
		http.SetCookie(w, &http.Cookie{
			Name:     "godom_token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		return true
	}
	return false
}

// Start starts the HTTP server, opens the default browser, and blocks forever.
func (a *App) Start() error {
	if a.comp == nil {
		return fmt.Errorf("godom: no component mounted, call Mount() before Start()")
	}

	// Parse CLI flags using a separate FlagSet to avoid conflicts
	// with the app developer's own flag usage.
	// CLI flags only apply when the developer hasn't explicitly set the value.
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

	ci := a.comp
	pool := &connPool{}
	var token string
	if !a.NoAuth {
		if a.Token != "" {
			token = a.Token
		} else {
			token = generateToken()
		}
	}

	// Wire up Refresh: allow Go code to push state to all browsers.
	ci.refreshFn = func() {
		ci.mu.Lock()
		allFields := allExportedFieldNames(ci.typ)
		sm := computeUpdateMessage(ci.pb, ci, allFields)
		ci.mu.Unlock()
		if sm != nil {
			data, _ := proto.Marshal(sm)
			pool.broadcast(data)
		}
	}

	mux := http.NewServeMux()

	// Build injected scripts: protobuf library, protocol definitions,
	// plugin registration + scripts, then bridge (last).
	var injectedJS string
	injectedJS += "<script>" + protobufMinJS + "</script>\n"
	injectedJS += "<script>" + protocolJS + "</script>\n"
	if len(a.plugins) > 0 {
		injectedJS += "<script>window.godom={_plugins:{},register:function(n,h){this._plugins[n]=h}};</script>\n"
		for _, pluginScripts := range a.plugins {
			for _, js := range pluginScripts {
				injectedJS += "<script>" + js + "</script>\n"
			}
		}
	}
	injectedJS += "<script>" + bridgeJS + "</script>\n"
	pageHTML := strings.Replace(ci.htmlBody, "</body>", injectedJS+"</body>", 1)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !a.NoAuth {
			if !checkAuth(token, w, r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			// Redirect to strip token from URL after cookie is set
			if r.URL.Query().Get("token") != "" {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !a.NoAuth && !checkAuth(token, w, r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("godom: websocket upgrade error: %v", err)
			return
		}

		wc := pool.add(conn)

		if err := handleInit(wc, ci); err != nil {
			log.Printf("godom: failed to compute init: %v", err)
			pool.remove(wc)
			conn.Close()
			return
		}

		// Read binary messages from this connection
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				pool.remove(wc)
				conn.Close()
				return
			}
			if msgType != websocket.BinaryMessage {
				continue
			}

			env := &Envelope{}
			if err := proto.Unmarshal(data, env); err != nil {
				log.Printf("godom: envelope unmarshal error: %v", err)
				continue
			}

			wsMsg := &WSMessage{}
			if err := proto.Unmarshal(env.Msg, wsMsg); err != nil {
				log.Printf("godom: wsmessage unmarshal error: %v", err)
				continue
			}

			switch wsMsg.Type {
			case "call":
				handleCall(ci, wsMsg, env.Args, pool)
			case "bind":
				handleBind(ci, wsMsg, env.Value, pool)
			}
		}
	})

	host := a.Host
	if host == "" {
		host = "localhost"
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, a.Port))
	if err != nil {
		return fmt.Errorf("godom: failed to listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://localhost:%d", port)
	if !a.NoAuth {
		url += "?token=" + token
	}
	if !a.Quiet {
		fmt.Printf("godom running at %s\n", url)
	}

	if !a.NoBrowser {
		openBrowser(url)
	}

	return http.Serve(ln, mux)
}

// --- WebSocket helpers ---

// wsConn wraps a WebSocket connection with a write mutex.
// gorilla/websocket does not allow concurrent writes, so each
// connection needs its own lock.
type wsConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func (wc *wsConn) writeBinary(data []byte) error {
	wc.wmu.Lock()
	defer wc.wmu.Unlock()
	return wc.conn.WriteMessage(websocket.BinaryMessage, data)
}

// connPool manages WebSocket connections for broadcasting.
type connPool struct {
	mu    sync.RWMutex
	conns []*wsConn
}

func (p *connPool) add(conn *websocket.Conn) *wsConn {
	wc := &wsConn{conn: conn}
	p.mu.Lock()
	p.conns = append(p.conns, wc)
	p.mu.Unlock()
	return wc
}

func (p *connPool) remove(wc *wsConn) {
	p.mu.Lock()
	for i, c := range p.conns {
		if c == wc {
			p.conns = append(p.conns[:i], p.conns[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
}

func (p *connPool) broadcast(data []byte) {
	p.mu.RLock()
	snapshot := make([]*wsConn, len(p.conns))
	copy(snapshot, p.conns)
	p.mu.RUnlock()

	for _, wc := range snapshot {
		wc.writeBinary(data)
	}
}

// handleInit sends the initial state to a newly connected client.
func handleInit(wc *wsConn, ci *componentInfo) error {
	ci.mu.Lock()
	initMsg, err := computeInitMessage(ci.pb, ci)
	ci.mu.Unlock()
	if err != nil {
		return err
	}
	data, err := proto.Marshal(initMsg)
	if err != nil {
		return err
	}
	return wc.writeBinary(data)
}

// handleCall processes a method call message from the bridge.
func handleCall(ci *componentInfo, wsMsg *WSMessage, envArgs []float64, pool *connPool) {
	target := ci
	if wsMsg.Scope != "" {
		if child := resolveScope(ci, wsMsg.Scope); child != nil {
			target = child
		} else {
			log.Printf("godom: unknown scope %q", wsMsg.Scope)
			return
		}
	}

	ci.mu.Lock()
	oldRootState := ci.snapshotState()

	// Merge browser-side args (mouse coords, wheel delta) into WSMessage args
	for _, a := range envArgs {
		jsonArg, _ := json.Marshal(a)
		wsMsg.Args = append(wsMsg.Args, jsonArg)
	}

	// Convert [][]byte to []json.RawMessage for callMethod
	jsonArgs := make([]json.RawMessage, len(wsMsg.Args))
	for i, a := range wsMsg.Args {
		jsonArgs[i] = json.RawMessage(a)
	}

	if err := target.callMethod(wsMsg.Method, jsonArgs); err != nil {
		log.Printf("godom: %v", err)
		ci.mu.Unlock()
		return
	}

	newRootState := ci.snapshotState()
	changed := ci.changedFields(oldRootState, newRootState)

	// For scoped calls where parent state didn't change,
	// re-render the child to pick up child state changes.
	if wsMsg.Scope != "" && len(changed) == 0 {
		sm := computeChildUpdateMessage(ci.pb, ci, wsMsg.Scope)
		ci.mu.Unlock()
		if sm != nil {
			data, _ := proto.Marshal(sm)
			pool.broadcast(data)
		}
		return
	}

	sm := computeUpdateMessage(ci.pb, ci, changed)
	ci.mu.Unlock()
	if sm != nil {
		data, _ := proto.Marshal(sm)
		pool.broadcast(data)
	}
}

// handleBind processes a two-way binding update from the bridge.
func handleBind(ci *componentInfo, wsMsg *WSMessage, value []byte, pool *connPool) {
	target := ci
	if wsMsg.Scope != "" {
		if child := resolveScope(ci, wsMsg.Scope); child != nil {
			target = child
		} else {
			log.Printf("godom: unknown scope %q", wsMsg.Scope)
			return
		}
	}

	ci.mu.Lock()
	oldState := ci.snapshotState()

	if err := target.setField(wsMsg.Field, json.RawMessage(value)); err != nil {
		log.Printf("godom: bind error: %v", err)
	}

	newState := ci.snapshotState()
	changed := ci.changedFields(oldState, newState)
	ci.mu.Unlock()

	if len(changed) > 0 {
		ci.mu.Lock()
		sm := computeUpdateMessage(ci.pb, ci, changed)
		ci.mu.Unlock()
		if sm != nil {
			data, _ := proto.Marshal(sm)
			pool.broadcast(data)
		}
	}
}

// resolveScope finds a child componentInfo from a scope string like "g3:0".
func resolveScope(root *componentInfo, scope string) *componentInfo {
	parts := strings.SplitN(scope, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	forGID := parts[0]
	idx := 0
	fmt.Sscanf(parts[1], "%d", &idx)

	children := root.children[forGID]
	if idx < 0 || idx >= len(children) {
		return nil
	}
	return children[idx]
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
