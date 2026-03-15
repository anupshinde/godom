package godom

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

//go:embed bridge.js
var bridgeJS string

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
	Port       int // 0 = random available port
	comp       *componentInfo
	components map[string]*componentReg // tag name → registered component
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
	}
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

// Start starts the HTTP server, opens the default browser, and blocks forever.
func (a *App) Start() error {
	if a.comp == nil {
		return fmt.Errorf("godom: no component mounted, call Mount() before Start()")
	}

	ci := a.comp
	pool := &connPool{}

	// Wire up Refresh: allow Go code to push state to all browsers.
	ci.refreshFn = func() {
		ci.mu.Lock()
		allFields := allExportedFieldNames(ci.typ)
		updateMsg := computeUpdateMessage(ci.pb, ci, allFields)
		ci.mu.Unlock()
		if updateMsg != nil {
			pool.broadcast(updateMsg)
		}
	}

	mux := http.NewServeMux()

	// Inject bridge script before </body>
	pageHTML := strings.Replace(ci.htmlBody, "</body>",
		"<script>"+bridgeJS+"</script>\n</body>", 1)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
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

		// Read messages from this connection
		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				pool.remove(wc)
				conn.Close()
				return
			}

			switch msg.Type {
			case "call":
				handleCall(ci, msg, pool)
			case "bind":
				handleBind(ci, msg, pool)
			}
		}
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", a.Port))
	if err != nil {
		return fmt.Errorf("godom: failed to listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("godom running at %s\n", url)

	openBrowser(url)

	return http.Serve(ln, mux)
}

// --- WebSocket helpers ---

// wsMessage is the structure of messages received from the bridge.
type wsMessage struct {
	Type   string            `json:"type"`
	Method string            `json:"method,omitempty"`
	Args   []json.RawMessage `json:"args,omitempty"`
	Field  string            `json:"field,omitempty"`
	Value  json.RawMessage   `json:"value,omitempty"`
	Scope  string            `json:"scope,omitempty"` // "forGID:idx" for child components
}

// wsConn wraps a WebSocket connection with a write mutex.
// gorilla/websocket does not allow concurrent writes, so each
// connection needs its own lock.
type wsConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func (wc *wsConn) writeJSON(msg interface{}) error {
	wc.wmu.Lock()
	defer wc.wmu.Unlock()
	return wc.conn.WriteJSON(msg)
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

func (p *connPool) broadcast(msg interface{}) {
	p.mu.RLock()
	snapshot := make([]*wsConn, len(p.conns))
	copy(snapshot, p.conns)
	p.mu.RUnlock()

	for _, wc := range snapshot {
		wc.writeJSON(msg)
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
	return wc.writeJSON(initMsg)
}

// handleCall processes a method call message from the bridge.
func handleCall(ci *componentInfo, msg wsMessage, pool *connPool) {
	target := ci
	if msg.Scope != "" {
		if child := resolveScope(ci, msg.Scope); child != nil {
			target = child
		} else {
			log.Printf("godom: unknown scope %q", msg.Scope)
			return
		}
	}

	ci.mu.Lock()
	oldRootState := ci.snapshotState()

	if err := target.callMethod(msg.Method, msg.Args); err != nil {
		log.Printf("godom: %v", err)
		ci.mu.Unlock()
		return
	}

	newRootState := ci.snapshotState()
	changed := ci.changedFields(oldRootState, newRootState)

	// For scoped calls where parent state didn't change,
	// re-render the child to pick up child state changes.
	if msg.Scope != "" && len(changed) == 0 {
		updateMsg := computeChildUpdateMessage(ci.pb, ci, msg.Scope)
		ci.mu.Unlock()
		if updateMsg != nil {
			pool.broadcast(updateMsg)
		}
		return
	}

	updateMsg := computeUpdateMessage(ci.pb, ci, changed)
	ci.mu.Unlock()
	if updateMsg != nil {
		pool.broadcast(updateMsg)
	}
}

// handleBind processes a two-way binding update from the bridge.
func handleBind(ci *componentInfo, msg wsMessage, pool *connPool) {
	target := ci
	if msg.Scope != "" {
		if child := resolveScope(ci, msg.Scope); child != nil {
			target = child
		} else {
			log.Printf("godom: unknown scope %q", msg.Scope)
			return
		}
	}

	ci.mu.Lock()
	oldState := ci.snapshotState()

	if err := target.setField(msg.Field, msg.Value); err != nil {
		log.Printf("godom: bind error: %v", err)
	}

	newState := ci.snapshotState()
	changed := ci.changedFields(oldState, newState)
	ci.mu.Unlock()

	if len(changed) > 0 {
		ci.mu.Lock()
		updateMsg := computeUpdateMessage(ci.pb, ci, changed)
		ci.mu.Unlock()
		if updateMsg != nil {
			pool.broadcast(updateMsg)
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
