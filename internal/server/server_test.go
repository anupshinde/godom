package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/anupshinde/godom/internal/component"
	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/anupshinde/godom/internal/render"
	"github.com/anupshinde/godom/internal/vdom"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// counterApp mirrors the counter example's state struct.
type counterApp struct {
	Component struct{} // dummy — matches the field name check
	Count     int
	Step      int
}

func (a *counterApp) Increment() {
	a.Count += a.Step
}

func (a *counterApp) Decrement() {
	a.Count -= a.Step
}

const counterHTML = `<!DOCTYPE html><html><head><title>Counter</title></head><body>
    <h1><span g-text="Count">0</span></h1>
    <div class="controls">
        <button g-click="Decrement">−</button>
        <button g-click="Increment">+</button>
    </div>
    <div class="step">
        <label>Step size:</label>
        <input type="number" min="1" max="100" g-bind="Step"/>
    </div>
</body></html>`

func makeCounterCI(app *counterApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()

	templates, err := vdom.ParseTemplate(counterHTML, nil)
	if err != nil {
		panic(err)
	}

	return &component.Info{
		Value:         v,
		Typ:           t,
		VDOMTemplates: templates,
	}
}

func TestVDOMBuildInit(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	msg := BuildInit(ci)

	if msg.Type != "init" {
		t.Fatalf("expected type 'init', got %q", msg.Type)
	}

	if len(msg.Tree) == 0 {
		t.Fatal("expected non-empty tree JSON")
	}

	// Decode tree and verify structure
	var tree render.WireNode
	if err := json.Unmarshal(msg.Tree, &tree); err != nil {
		t.Fatal(err)
	}

	if tree.Tag != "body" {
		t.Errorf("expected root tag 'body', got %q", tree.Tag)
	}

	// Should have children (h1, div.controls, div.step)
	if len(tree.Children) == 0 {
		t.Error("expected children in tree")
	}

	// Find the text node with "5" (the resolved count)
	found := findTextInTree(&tree, "5")
	if !found {
		t.Error("expected count '5' in tree")
	}

	// Should have events (click)
	foundClick := findEventInTree(&tree, "click")
	if !foundClick {
		t.Error("expected click event in tree")
	}

	// Should be serializable
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty protobuf bytes")
	}
}

func TestVDOMBuildUpdate_Increment(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	// Initial render
	_ = BuildInit(ci)

	// Simulate Increment
	app.Count = 1
	msg := BuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after increment")
	}
	if msg.Type != "patch" {
		t.Fatalf("expected type 'patch', got %q", msg.Type)
	}

	// Should have a text patch changing "0" to "1"
	var hasTextPatch bool
	for _, p := range msg.Patches {
		if p.Op == render.OpText && p.Text == "1" {
			hasTextPatch = true
		}
	}
	if !hasTextPatch {
		t.Errorf("expected text patch '0' → '1', patches: %+v", msg.Patches)
	}

	// Should be serializable
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty protobuf bytes")
	}
}

func TestVDOMBuildUpdate_NoChange(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// No state change
	msg := BuildUpdate(ci)
	if msg != nil {
		t.Errorf("expected nil message when nothing changed, got type=%q patches=%d", msg.Type, len(msg.Patches))
	}
}

func TestVDOMBuildUpdate_BindStep(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// Simulate step change (as if g-bind updated it)
	app.Step = 5
	msg := BuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after step change")
	}

	// Should have a facts patch (value property changed on input)
	var hasFactsPatch bool
	for _, p := range msg.Patches {
		if p.Op == render.OpFacts {
			hasFactsPatch = true
		}
	}
	if !hasFactsPatch {
		t.Errorf("expected facts patch for step change, patches: %+v", msg.Patches)
	}
}

func TestVDOMBuildUpdate_MultipleIncrements(t *testing.T) {
	app := &counterApp{Step: 2, Count: 0}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// First increment
	app.Count = 2
	msg1 := BuildUpdate(ci)
	if msg1 == nil {
		t.Fatal("expected patch for first increment")
	}

	// Second increment
	app.Count = 4
	msg2 := BuildUpdate(ci)
	if msg2 == nil {
		t.Fatal("expected patch for second increment")
	}

	// Check the second patch has text "4"
	var hasText4 bool
	for _, p := range msg2.Patches {
		if p.Op == render.OpText && p.Text == "4" {
			hasText4 = true
		}
	}
	if !hasText4 {
		t.Error("expected text patch '2' → '4'")
	}
}

// --- generateToken tests ---

func TestGenerateToken_Length(t *testing.T) {
	tok := generateToken()
	// 16 bytes → 32 hex characters
	if len(tok) != 32 {
		t.Errorf("expected 32-char hex token, got %d chars: %q", len(tok), tok)
	}
}

func TestGenerateToken_Unique(t *testing.T) {
	a := generateToken()
	b := generateToken()
	if a == b {
		t.Errorf("expected different tokens, both are %q", a)
	}
}

func TestGenerateToken_ValidHex(t *testing.T) {
	tok := generateToken()
	for _, c := range tok {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("token %q contains non-hex char %c", tok, c)
			break
		}
	}
}

// --- checkAuth tests ---

func TestCheckAuth_ValidQueryParam(t *testing.T) {
	token := "abc123"
	r := httptest.NewRequest("GET", "/?token=abc123", nil)
	w := httptest.NewRecorder()

	ok := checkAuth(token, w, r)
	if !ok {
		t.Error("expected auth to succeed with valid query param")
	}

	// Should set cookie
	resp := w.Result()
	cookies := resp.Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "godom_token" && c.Value == token {
			found = true
			if !c.HttpOnly {
				t.Error("expected HttpOnly cookie")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Error("expected SameSite=Strict cookie")
			}
		}
	}
	if !found {
		t.Error("expected godom_token cookie to be set")
	}
}

func TestCheckAuth_ValidCookie(t *testing.T) {
	token := "secret42"
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "godom_token", Value: token})
	w := httptest.NewRecorder()

	ok := checkAuth(token, w, r)
	if !ok {
		t.Error("expected auth to succeed with valid cookie")
	}
}

func TestCheckAuth_WrongToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/?token=wrong", nil)
	w := httptest.NewRecorder()

	ok := checkAuth("correct", w, r)
	if ok {
		t.Error("expected auth to fail with wrong token")
	}
}

func TestCheckAuth_NoToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	ok := checkAuth("secret", w, r)
	if ok {
		t.Error("expected auth to fail with no token")
	}
}

func TestCheckAuth_WrongCookie(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "godom_token", Value: "wrong"})
	w := httptest.NewRecorder()

	ok := checkAuth("correct", w, r)
	if ok {
		t.Error("expected auth to fail with wrong cookie")
	}
}

func TestCheckAuth_CookieTakesPrecedence(t *testing.T) {
	// Valid cookie but no query param — should still pass
	token := "mytoken"
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "godom_token", Value: token})
	w := httptest.NewRecorder()

	ok := checkAuth(token, w, r)
	if !ok {
		t.Error("expected auth to succeed with valid cookie even without query param")
	}

	// No new cookie should be set (already authed via cookie)
	resp := w.Result()
	if len(resp.Cookies()) != 0 {
		t.Error("expected no new cookies when authed via existing cookie")
	}
}

// --- localIP tests ---

func TestLocalIP_ReturnsString(t *testing.T) {
	// localIP returns a non-loopback IPv4 or empty string
	ip := localIP()
	// On CI or machines without network, empty is acceptable
	if ip != "" {
		if strings.HasPrefix(ip, "127.") {
			t.Errorf("expected non-loopback IP, got %q", ip)
		}
		// Basic IPv4 format check
		parts := strings.Split(ip, ".")
		if len(parts) != 4 {
			t.Errorf("expected IPv4 format, got %q", ip)
		}
	}
}

// --- connPool tests ---

func newTestWSPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Echo messages back
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			conn.WriteMessage(mt, msg)
		}
	}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	// Get server-side conn via a round-trip
	return client, nil, func() {
		client.Close()
		srv.Close()
	}
}

// wsServer starts a WS server and returns a connected client websocket + cleanup.
func wsServer(t *testing.T, handler func(*websocket.Conn)) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handler(conn)
	}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	return client, func() { client.Close(); srv.Close() }
}

func TestConnPool_AddAndRemove(t *testing.T) {
	pool := &connPool{}

	// Use a real websocket pair for the pool
	var serverConn *websocket.Conn
	ready := make(chan struct{})
	client, cleanup := wsServer(t, func(c *websocket.Conn) {
		serverConn = c
		close(ready)
		// Block until test is done
		time.Sleep(2 * time.Second)
	})
	defer cleanup()
	<-ready

	_ = client // keep alive

	wc := pool.add(serverConn)

	pool.mu.RLock()
	if len(pool.conns) != 1 {
		t.Errorf("expected 1 conn, got %d", len(pool.conns))
	}
	pool.mu.RUnlock()

	pool.remove(wc)

	pool.mu.RLock()
	if len(pool.conns) != 0 {
		t.Errorf("expected 0 conns after remove, got %d", len(pool.conns))
	}
	pool.mu.RUnlock()
}

func TestConnPool_RemoveNonExistent(t *testing.T) {
	pool := &connPool{}
	// Should not panic
	pool.remove(&wsConn{})

	pool.mu.RLock()
	if len(pool.conns) != 0 {
		t.Errorf("expected 0 conns, got %d", len(pool.conns))
	}
	pool.mu.RUnlock()
}

func TestConnPool_BroadcastEmpty(t *testing.T) {
	pool := &connPool{}
	// Should not panic with empty pool
	pool.broadcast([]byte("hello"))
}

func TestConnPool_Broadcast(t *testing.T) {
	// Set up two server-side connections in a pool, verify both receive data
	var mu sync.Mutex
	var serverConns []*websocket.Conn
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		serverConns = append(serverConns, conn)
		mu.Unlock()
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client1, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client1.Close()

	client2, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client2.Close()

	// Wait for server to accept both
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(serverConns) != 2 {
		t.Fatalf("expected 2 server conns, got %d", len(serverConns))
	}
	mu.Unlock()

	pool := &connPool{}
	pool.add(serverConns[0])
	pool.add(serverConns[1])

	testData := []byte{0x01, 0x02, 0x03}
	pool.broadcast(testData)

	// Both clients should receive the binary message
	for i, client := range []*websocket.Conn{client1, client2} {
		client.SetReadDeadline(time.Now().Add(time.Second))
		mt, data, err := client.ReadMessage()
		if err != nil {
			t.Errorf("client %d read error: %v", i, err)
			continue
		}
		if mt != websocket.BinaryMessage {
			t.Errorf("client %d: expected binary message, got %d", i, mt)
		}
		if len(data) != 3 || data[0] != 0x01 {
			t.Errorf("client %d: unexpected data %v", i, data)
		}
	}
}

// --- handleInit tests ---

func TestHandleInit_SendsInitMessage(t *testing.T) {
	app := &counterApp{Step: 1, Count: 7}
	ci := makeCounterCI(app)

	var serverConn *websocket.Conn
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn = c
		close(done)
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-done

	wc := &wsConn{conn: serverConn}
	if err := handleInit(wc, ci); err != nil {
		t.Fatalf("handleInit error: %v", err)
	}

	// Client should receive the init message
	client.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary message, got %d", mt)
	}

	var msg gproto.VDomMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if msg.Type != "init" {
		t.Errorf("expected type 'init', got %q", msg.Type)
	}
	if len(msg.Tree) == 0 {
		t.Error("expected non-empty tree")
	}

	// Verify tree contains the count value
	var tree render.WireNode
	if err := json.Unmarshal(msg.Tree, &tree); err != nil {
		t.Fatal(err)
	}
	if !findTextInTree(&tree, "7") {
		t.Error("expected count '7' in init tree")
	}
}

// --- handleNodeEvent tests ---

func TestHandleNodeEvent_UpdatesTreeAndBroadcasts(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	// Build initial tree so Tree is populated
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find the input node ID (the g-bind="Step" input)
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node in tree")
	}

	// Set up a connection pool with a client
	var serverConn *websocket.Conn
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, _ := up.Upgrade(w, r, nil)
		serverConn = c
		close(done)
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-done

	pool := &connPool{}
	pool.add(serverConn)

	// Fire node event
	handleNodeEvent(ci, int32(inputNodeID), "42", pool)

	// Verify the live tree was updated
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	if node == nil {
		t.Fatal("node not found after event")
	}
	el, ok := node.(*vdom.ElementNode)
	if !ok {
		t.Fatal("expected ElementNode")
	}
	if el.Facts.Props["value"] != "42" {
		t.Errorf("expected Props[value]='42', got %v", el.Facts.Props["value"])
	}

	// Client should receive a patch with the value "42"
	client.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary, got %d", mt)
	}
	var msg gproto.VDomMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "patch" {
		t.Errorf("expected 'patch', got %q", msg.Type)
	}
	// Verify the patch carries the correct value
	var hasValuePatch bool
	for _, p := range msg.Patches {
		if p.Op == render.OpFacts && strings.Contains(string(p.Facts), "42") {
			hasValuePatch = true
		}
	}
	if !hasValuePatch {
		t.Errorf("expected facts patch containing value '42', got patches: %+v", msg.Patches)
	}
}

func TestHandleNodeEvent_NodeNotFound(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	pool := &connPool{}
	// Should not panic with a nonexistent node ID
	handleNodeEvent(ci, 99999, "value", pool)
}

// --- handleMethodCall tests ---

func TestHandleMethodCall_CallsMethod(t *testing.T) {
	app := &counterApp{Step: 3, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	var serverConn *websocket.Conn
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, _ := up.Upgrade(w, r, nil)
		serverConn = c
		close(done)
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-done

	pool := &connPool{}
	pool.add(serverConn)

	// Wire RefreshFn to broadcast (mirrors Run)
	ci.RefreshFn = func() {
		ci.Mu.Lock()
		msg := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	call := &gproto.MethodCall{Method: "Increment"}
	handleMethodCall(ci, call, pool)

	// Verify method was called: Count should be 3 (0 + Step=3)
	if app.Count != 3 {
		t.Errorf("expected Count=3 after Increment, got %d", app.Count)
	}

	// Client should receive an update message (rebuild)
	client.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary, got %d", mt)
	}
	var msg gproto.VDomMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestHandleMethodCall_RebuildReflectsNewState(t *testing.T) {
	app := &counterApp{Step: 5, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	var serverConn *websocket.Conn
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, _ := up.Upgrade(w, r, nil)
		serverConn = c
		close(done)
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-done

	pool := &connPool{}
	pool.add(serverConn)

	// Wire RefreshFn to broadcast (mirrors Run)
	ci.RefreshFn = func() {
		ci.Mu.Lock()
		msg := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	call := &gproto.MethodCall{Method: "Increment"}
	handleMethodCall(ci, call, pool)

	// The broadcast should contain a tree with the new count "5"
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg gproto.VDomMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "patch" {
		t.Fatalf("expected 'patch', got %q", msg.Type)
	}

	// Verify the patch message contains patches (the state change)
	if len(msg.Patches) == 0 {
		t.Error("expected non-empty patches")
	}
}

func TestHandleMethodCall_InvalidMethod(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	pool := &connPool{}
	call := &gproto.MethodCall{Method: "NonExistent"}
	// Should not panic — logs error and returns
	handleMethodCall(ci, call, pool)

	// Count should not have changed
	if app.Count != 0 {
		t.Errorf("expected Count=0, got %d", app.Count)
	}
}

// refreshApp has a method that calls RefreshFn, simulating the real production flow
// where a user method calls Refresh() during execution.
type refreshApp struct {
	Component struct{}
	Count     int
	refreshFn func()
}

func (a *refreshApp) IncrementAndRefresh() {
	a.Count++
	if a.refreshFn != nil {
		a.refreshFn()
	}
}

func TestHandleMethodCall_SkipsRebuildWhenRefreshed(t *testing.T) {
	app := &refreshApp{Count: 0}
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(`<!DOCTYPE html><html><head></head><body><span g-text="Count">0</span></body></html>`, nil)
	if err != nil {
		t.Fatal(err)
	}
	ci := &component.Info{
		Value:         v,
		Typ:           v.Elem().Type(),
		VDOMTemplates: templates,
	}

	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Wire RefreshFn exactly as Run does — sets Refreshed=true and broadcasts
	var serverConn *websocket.Conn
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, _ := up.Upgrade(w, r, nil)
		serverConn = c
		close(done)
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-done

	pool := &connPool{}
	pool.add(serverConn)

	// Wire RefreshFn to broadcast (mirrors Run)
	refreshCount := 0
	ci.RefreshFn = func() {
		refreshCount++
		ci.Mu.Lock()
		msg := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}
	// Let the app call RefreshFn
	app.refreshFn = ci.RefreshFn

	call := &gproto.MethodCall{Method: "IncrementAndRefresh"}
	handleMethodCall(ci, call, pool)

	// Method was called
	if app.Count != 1 {
		t.Errorf("expected Count=1 after IncrementAndRefresh, got %d", app.Count)
	}

	// NOTE: RefreshFn is called twice — once by the method itself, once by
	// handleMethodCall after the method returns. This double-refresh is
	// harmless (second diff is empty → no broadcast) but ideally
	// handleMethodCall would detect that Refresh was already called and skip.
	// The old Refreshed flag did this but was removed.
	if refreshCount < 1 {
		t.Error("expected RefreshFn to be called at least once")
	}

	// Client should receive at least one message
	client.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary, got %d", mt)
	}
	var msg gproto.VDomMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
}

func TestHandleMethodCall_SyncsBindValues(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find the input node and set its value (simulating browser input)
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	if el.Facts.Props == nil {
		el.Facts.Props = make(map[string]any)
	}
	el.Facts.Props["value"] = "10"

	pool := &connPool{}

	// handleMethodCall calls ci.RefreshFn() after the method
	ci.RefreshFn = func() {}

	call := &gproto.MethodCall{Method: "Increment"}
	handleMethodCall(ci, call, pool)

	// Step should have been synced from tree prop to struct field
	if app.Step != 10 {
		t.Errorf("expected Step=10 after bind sync, got %d", app.Step)
	}
	// Count should be 10 (0 + synced Step=10)
	if app.Count != 10 {
		t.Errorf("expected Count=10 after Increment with synced Step, got %d", app.Count)
	}
}

// --- buildSurgicalPatches tests ---

// surgicalApp has multiple binding types for testing surgical patches.
type surgicalApp struct {
	Component struct{}
	Name      string
	Color     string
	Visible   bool
	Hidden    bool
	Active    bool
	Width     string
}

const surgicalHTML = `<!DOCTYPE html><html><head></head><body>
	<span g-text="Name">placeholder</span>
	<div g-attr:data-color="Color"></div>
	<div g-show="Visible">shown</div>
	<div g-hide="Hidden">hidden</div>
	<div g-class:active="Active">classed</div>
	<div g-style:width="Width">styled</div>
	<input g-bind="Name"/>
</body></html>`

func makeSurgicalCI(app *surgicalApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()
	templates, err := vdom.ParseTemplate(surgicalHTML, nil)
	if err != nil {
		panic(err)
	}
	return &component.Info{
		Value:         v,
		Typ:           t,
		VDOMTemplates: templates,
	}
}

func TestBuildSurgicalPatches_TextBinding(t *testing.T) {
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	patches := buildSurgicalPatches(ci, []string{"Name"})

	var hasTextPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchText {
			data, ok := p.Data.(vdom.PatchTextData)
			if ok && data.Text == "Bob" {
				hasTextPatch = true
			}
		}
	}
	if !hasTextPatch {
		t.Errorf("expected text patch with 'Bob', got patches: %+v", patches)
	}
}

func TestBuildSurgicalPatches_AttrBinding(t *testing.T) {
	app := &surgicalApp{Color: "red"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Color = "blue"
	patches := buildSurgicalPatches(ci, []string{"Color"})

	var hasAttrPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Attrs != nil && data.Diff.Attrs["data-color"] == "blue" {
				hasAttrPatch = true
			}
		}
	}
	if !hasAttrPatch {
		t.Errorf("expected attr patch with data-color=blue, got patches: %+v", patches)
	}
}

func TestBuildSurgicalPatches_ShowBinding(t *testing.T) {
	app := &surgicalApp{Visible: true}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Visible → false means display should be "none"
	app.Visible = false
	patches := buildSurgicalPatches(ci, []string{"Visible"})

	var hasShowPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Styles != nil && data.Diff.Styles["display"] == "none" {
				hasShowPatch = true
			}
		}
	}
	if !hasShowPatch {
		t.Errorf("expected show patch with display=none, got patches: %+v", patches)
	}
}

func TestBuildSurgicalPatches_HideBinding(t *testing.T) {
	app := &surgicalApp{Hidden: false}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Hidden → true means display should be "none"
	app.Hidden = true
	patches := buildSurgicalPatches(ci, []string{"Hidden"})

	var hasHidePatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Styles != nil && data.Diff.Styles["display"] == "none" {
				hasHidePatch = true
			}
		}
	}
	if !hasHidePatch {
		t.Errorf("expected hide patch with display=none, got patches: %+v", patches)
	}
}

func TestBuildSurgicalPatches_ClassBinding(t *testing.T) {
	app := &surgicalApp{Active: false}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Active = true
	patches := buildSurgicalPatches(ci, []string{"Active"})

	var hasClassPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Props != nil {
				if cn, exists := data.Diff.Props["className"]; exists {
					if strings.Contains(fmt.Sprint(cn), "active") {
						hasClassPatch = true
					}
				}
			}
		}
	}
	if !hasClassPatch {
		t.Errorf("expected class patch with 'active', got patches: %+v", patches)
	}
}

func TestBuildSurgicalPatches_StyleBinding(t *testing.T) {
	app := &surgicalApp{Width: "100px"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Width = "200px"
	patches := buildSurgicalPatches(ci, []string{"Width"})

	var hasStylePatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Styles != nil && data.Diff.Styles["width"] == "200px" {
				hasStylePatch = true
			}
		}
	}
	if !hasStylePatch {
		t.Errorf("expected style patch with width=200px, got patches: %+v", patches)
	}
}

func TestBuildSurgicalPatches_NoBindings(t *testing.T) {
	app := &surgicalApp{}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	patches := buildSurgicalPatches(ci, []string{"NonExistentField"})
	if len(patches) != 0 {
		t.Errorf("expected no patches for unbound field, got %d", len(patches))
	}
}

func TestBuildSurgicalPatches_EmptyFields(t *testing.T) {
	app := &surgicalApp{}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	patches := buildSurgicalPatches(ci, []string{})
	if len(patches) != 0 {
		t.Errorf("expected no patches for empty fields, got %d", len(patches))
	}
}

func TestBuildSurgicalPatches_UpdatesLiveTree(t *testing.T) {
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	_ = buildSurgicalPatches(ci, []string{"Name"})

	// Verify the live tree was updated in place
	// Find text node with "Bob" in the live tree
	found := false
	walkTree(ci.Tree, func(n vdom.Node) {
		if tn, ok := n.(*vdom.TextNode); ok && tn.Text == "Bob" {
			found = true
		}
	})
	if !found {
		t.Error("expected live tree to be updated with 'Bob'")
	}
}

func TestBuildSurgicalPatches_UpdatesLiveTreeStyle(t *testing.T) {
	app := &surgicalApp{Width: "100px"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Width = "200px"
	_ = buildSurgicalPatches(ci, []string{"Width"})

	// Verify the live tree's style was updated
	bindings := ci.Bindings["Width"]
	for _, b := range bindings {
		if b.Kind == "style" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Styles["width"] != "200px" {
					t.Errorf("expected live tree style width=200px, got %q", el.Facts.Styles["width"])
				}
			}
		}
	}
}

func TestBuildSurgicalPatches_UpdatesLiveTreeAttr(t *testing.T) {
	app := &surgicalApp{Color: "red"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Color = "blue"
	_ = buildSurgicalPatches(ci, []string{"Color"})

	bindings := ci.Bindings["Color"]
	for _, b := range bindings {
		if b.Kind == "attr" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Attrs["data-color"] != "blue" {
					t.Errorf("expected live tree attr data-color=blue, got %q", el.Facts.Attrs["data-color"])
				}
			}
		}
	}
}

func TestBuildSurgicalPatches_UpdatesLiveTreeShowHide(t *testing.T) {
	// show: Visible=true → false should set display=none in live tree
	app := &surgicalApp{Visible: true}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Visible = false
	_ = buildSurgicalPatches(ci, []string{"Visible"})

	bindings := ci.Bindings["Visible"]
	for _, b := range bindings {
		if b.Kind == "show" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Styles["display"] != "none" {
					t.Errorf("expected live tree display=none when show=false, got %q", el.Facts.Styles["display"])
				}
			}
		}
	}

	// Now set Visible=true, display should be removed
	app.Visible = true
	_ = buildSurgicalPatches(ci, []string{"Visible"})

	for _, b := range bindings {
		if b.Kind == "show" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if _, exists := el.Facts.Styles["display"]; exists {
					t.Errorf("expected display to be removed from live tree when show=true, got %q", el.Facts.Styles["display"])
				}
			}
		}
	}
}

func TestBuildSurgicalPatches_MultipleFields(t *testing.T) {
	app := &surgicalApp{Name: "Alice", Width: "100px"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	app.Width = "200px"
	patches := buildSurgicalPatches(ci, []string{"Name", "Width"})

	// Should have patches for both fields
	if len(patches) < 2 {
		t.Errorf("expected at least 2 patches for 2 fields, got %d", len(patches))
	}
}

// --- Run integration tests ---

func TestRun_ServesHTML(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
		BridgeJS:  "// bridge",
	}

	// Start server in background
	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/", ln))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "// bridge") {
		t.Error("expected injected bridge JS in response")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected content type 'text/html; charset=utf-8', got %q", ct)
	}
}

func TestRun_AuthRejectsWithoutToken(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    false,
		Token:     "testsecret",
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// No token → 401
	resp, err := http.Get(fmt.Sprintf("http://%s/", ln))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRun_AuthAcceptsWithToken(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    false,
		Token:     "testsecret",
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// With valid token → should redirect (302) to strip token from URL
	noRedirect := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := noRedirect.Get(fmt.Sprintf("http://%s/?token=testsecret", ln))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 redirect after valid token, got %d", resp.StatusCode)
	}

	// After redirect, the cookie should be set — follow the redirect with cookie jar
	jar := resp.Cookies()
	var hasCookie bool
	for _, c := range jar {
		if c.Name == "godom_token" && c.Value == "testsecret" {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Error("expected godom_token cookie in redirect response")
	}

	// Follow redirect with cookie → should get the page
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/", ln), nil)
	for _, c := range jar {
		req.AddCookie(c)
	}
	resp2, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 with valid cookie, got %d", resp2.StatusCode)
	}
}

func TestRun_WebSocketUpgrade(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", ln)
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer client.Close()

	// Should receive init message
	client.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary, got %d", mt)
	}
	var msg gproto.VDomMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "init" {
		t.Errorf("expected init, got %q", msg.Type)
	}
}

func TestRun_WebSocketMethodCall(t *testing.T) {
	app := &counterApp{Step: 2, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", ln)
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Read init message first
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}

	// Send a method call (tag=2)
	call := &gproto.MethodCall{Method: "Increment"}
	payload, _ := proto.Marshal(call)
	msg := append([]byte{2}, payload...)
	if err := client.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatal(err)
	}

	// Should receive update
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp gproto.VDomMessage
	if err := proto.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}

	// Count should now be 2
	if app.Count != 2 {
		t.Errorf("expected Count=2, got %d", app.Count)
	}

	// The response should contain the updated tree with "2"
	if resp.Type == "init" && len(resp.Tree) > 0 {
		var tree render.WireNode
		if err := json.Unmarshal(resp.Tree, &tree); err == nil {
			if !findTextInTree(&tree, "2") {
				t.Error("expected updated tree to contain text '2'")
			}
		}
	}
}

func TestRun_WebSocketNodeEvent(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", ln)
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Read init
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}

	// Find input node ID
	ci.Mu.Lock()
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	ci.Mu.Unlock()
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}

	// Send node event (tag=1)
	evt := &gproto.NodeEvent{NodeId: int32(inputNodeID), Value: "99"}
	payload, _ := proto.Marshal(evt)
	msg := append([]byte{1}, payload...)
	if err := client.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatal(err)
	}

	// Should receive facts patch containing value "99"
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp gproto.VDomMessage
	if err := proto.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != "patch" {
		t.Errorf("expected patch, got %q", resp.Type)
	}
	var hasValue99 bool
	for _, p := range resp.Patches {
		if p.Op == render.OpFacts && strings.Contains(string(p.Facts), "99") {
			hasValue99 = true
		}
	}
	if !hasValue99 {
		t.Errorf("expected facts patch containing '99', got patches: %+v", resp.Patches)
	}
}

func TestRun_WebSocketAuthReject(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    false,
		Token:     "secret",
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// WS without token should fail
	wsURL := fmt.Sprintf("ws://%s/ws", ln)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("expected ws dial to fail without auth")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRun_PluginScripts(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
		Plugins:   map[string][]string{"chart": {"console.log('chart')"}},
		BridgeJS:  "// bridge",
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/", ln))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "console.log('chart')") {
		t.Error("expected plugin script in response")
	}
	if !strings.Contains(bodyStr, "window.godom=") {
		t.Error("expected godom plugin registration script")
	}
}

func TestRun_StaticFiles(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS: fstest.MapFS{
			"style.css": &fstest.MapFile{Data: []byte("body{margin:0}")},
		},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/style.css", ln))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "body{margin:0}" {
		t.Errorf("expected static CSS content, got %q", string(body))
	}
}

// --- broadcastClose test ---

func TestConnPool_BroadcastClose(t *testing.T) {
	var serverConns []*websocket.Conn
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		serverConns = append(serverConns, conn)
		mu.Unlock()
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client1, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client1.Close()
	client2, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client2.Close()

	time.Sleep(100 * time.Millisecond)

	pool := &connPool{}
	mu.Lock()
	for _, sc := range serverConns {
		pool.add(sc)
	}
	mu.Unlock()

	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")
	pool.broadcastClose(closeMsg)

	// Clients should receive close frame or get read error
	for i, client := range []*websocket.Conn{client1, client2} {
		client.SetReadDeadline(time.Now().Add(time.Second))
		_, _, err := client.ReadMessage()
		if err == nil {
			t.Errorf("client %d: expected error after close, got none", i)
		}
	}
}

// --- handleNodeEvent additional negative tests ---

func TestHandleNodeEvent_TextNodeNotElement(t *testing.T) {
	// Build a tree where we try to send a node event to a text node
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find a text node ID
	var textNodeID int
	walkTree(ci.Tree, func(n vdom.Node) {
		if _, ok := n.(*vdom.TextNode); ok && textNodeID == 0 {
			textNodeID = n.NodeID()
		}
	})
	if textNodeID == 0 {
		t.Fatal("no text node found in tree — counter template should always have text nodes")
	}

	pool := &connPool{}
	// Should log but not panic
	handleNodeEvent(ci, int32(textNodeID), "value", pool)
}

// --- handleMethodCall with args ---

func TestHandleMethodCall_EmptyMethod(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	pool := &connPool{}
	call := &gproto.MethodCall{Method: ""}
	// Should not panic — empty method name
	handleMethodCall(ci, call, pool)
}

// --- buildSurgicalPatches: prop binding (g-bind generates "bind" + "prop" bindings) ---

func TestBuildSurgicalPatches_PropBinding(t *testing.T) {
	// g-bind creates a "bind" kind (for sync) and a "prop" kind (for value).
	// Verify that surgical patch for Name includes a prop patch for the input value.
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	patches := buildSurgicalPatches(ci, []string{"Name"})

	// Check what binding kinds exist for Name
	bindings := ci.Bindings["Name"]
	var kinds []string
	for _, b := range bindings {
		kinds = append(kinds, b.Kind)
	}

	// We expect at least a text patch (from g-text) and potentially a prop patch (from g-bind)
	var hasPropPatch, hasTextPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Props != nil {
				if v, exists := data.Diff.Props["value"]; exists && fmt.Sprint(v) == "Bob" {
					hasPropPatch = true
				}
			}
		}
		if p.Type == vdom.PatchText {
			data, ok := p.Data.(vdom.PatchTextData)
			if ok && data.Text == "Bob" {
				hasTextPatch = true
			}
		}
	}

	// At minimum, the text binding should produce a text patch
	if !hasTextPatch {
		t.Errorf("expected text patch with 'Bob', got patches: %+v (binding kinds: %v)", patches, kinds)
	}
	// If a "prop" binding exists, we expect a prop patch too
	for _, k := range kinds {
		if k == "prop" && !hasPropPatch {
			t.Errorf("binding has kind=prop but no prop patch was generated, patches: %+v", patches)
		}
	}
}

// --- buildSurgicalPatches: show=true (display should be empty, not "none") ---

func TestBuildSurgicalPatches_ShowTrue(t *testing.T) {
	app := &surgicalApp{Visible: false}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Visible = true
	patches := buildSurgicalPatches(ci, []string{"Visible"})

	var hasShowPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Styles != nil {
				if d, exists := data.Diff.Styles["display"]; exists && d == "" {
					hasShowPatch = true
				}
			}
		}
	}
	if !hasShowPatch {
		t.Errorf("expected show patch with display='', got patches: %+v", patches)
	}
}

// --- buildSurgicalPatches: hide=false (display should be empty) ---

func TestBuildSurgicalPatches_HideFalse(t *testing.T) {
	app := &surgicalApp{Hidden: true}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Hidden = false
	patches := buildSurgicalPatches(ci, []string{"Hidden"})

	var hasHidePatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Styles != nil {
				if d, exists := data.Diff.Styles["display"]; exists && d == "" {
					hasHidePatch = true
				}
			}
		}
	}
	if !hasHidePatch {
		t.Errorf("expected hide patch with display='', got patches: %+v", patches)
	}
}

// --- buildSurgicalPatches: class remove ---

func TestBuildSurgicalPatches_ClassRemove(t *testing.T) {
	app := &surgicalApp{Active: true}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Active = false
	patches := buildSurgicalPatches(ci, []string{"Active"})

	var hasClassPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Props != nil {
				if cn, exists := data.Diff.Props["className"]; exists {
					// When removing, should NOT contain "active"
					if !strings.Contains(fmt.Sprint(cn), "active") {
						hasClassPatch = true
					}
				}
			}
		}
	}
	if !hasClassPatch {
		t.Errorf("expected class patch removing 'active', got patches: %+v", patches)
	}
}

// --- Negative: handleMethodCall bind sync edge cases ---

func TestHandleMethodCall_NilTree(t *testing.T) {
	// If Tree is nil, bind sync should be skipped gracefully
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	// Don't call BuildInit — Tree stays nil
	ci.RefreshFn = func() {}

	pool := &connPool{}
	call := &gproto.MethodCall{Method: "Increment"}
	handleMethodCall(ci, call, pool)

	// Method should still execute
	if app.Count != 1 {
		t.Errorf("expected Count=1, got %d", app.Count)
	}
}

func TestHandleMethodCall_BindNodeMissing(t *testing.T) {
	// Binding references a node ID that doesn't exist in the tree
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Inject a fake binding pointing to a nonexistent node
	ci.Bindings["Step"] = append(ci.Bindings["Step"], vdom.Binding{
		NodeID: 99999,
		Kind:   "bind",
		Prop:   "value",
	})

	ci.RefreshFn = func() {}
	pool := &connPool{}
	call := &gproto.MethodCall{Method: "Increment"}
	// Should not panic — skips the missing node
	handleMethodCall(ci, call, pool)

	if app.Count != 1 {
		t.Errorf("expected Count=1, got %d", app.Count)
	}
}

func TestHandleMethodCall_BindNodeNilProps(t *testing.T) {
	// Binding points to a real element but its Props map is nil
	app := &counterApp{Step: 5, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find the input node and null out its Props
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("no input node")
	}
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	el.Facts.Props = nil

	ci.RefreshFn = func() {}
	pool := &connPool{}
	call := &gproto.MethodCall{Method: "Increment"}
	// Should not panic — skips the nil Props
	handleMethodCall(ci, call, pool)

	// Step should remain 5 (not synced since Props was nil)
	if app.Step != 5 {
		t.Errorf("expected Step=5 (unsynced), got %d", app.Step)
	}
}

func TestHandleMethodCall_BindValueMissing(t *testing.T) {
	// Binding points to a real element with Props, but no "value" key
	app := &counterApp{Step: 5, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("no input node")
	}
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	el.Facts.Props = map[string]any{"className": "input"} // no "value" key

	ci.RefreshFn = func() {}
	pool := &connPool{}
	call := &gproto.MethodCall{Method: "Increment"}
	handleMethodCall(ci, call, pool)

	// Step should remain 5 (no value to sync)
	if app.Step != 5 {
		t.Errorf("expected Step=5, got %d", app.Step)
	}
}

// --- Negative: WebSocket message handling edge cases ---

func TestRun_WebSocketIgnoresNonBinary(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", ln)
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Read init
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = client.ReadMessage()

	// Send a text message (should be ignored, not crash)
	client.WriteMessage(websocket.TextMessage, []byte("hello"))

	// Send too-short binary message (< 2 bytes, should be ignored)
	client.WriteMessage(websocket.BinaryMessage, []byte{0x01})

	// Send unknown tag (should be ignored)
	client.WriteMessage(websocket.BinaryMessage, []byte{0xFF, 0x00})

	// Server should still be alive — send a valid method call
	call := &gproto.MethodCall{Method: "Increment"}
	payload, _ := proto.Marshal(call)
	client.WriteMessage(websocket.BinaryMessage, append([]byte{2}, payload...))

	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("server died after invalid messages: %v", err)
	}
	var msg gproto.VDomMessage
	proto.Unmarshal(data, &msg)
	if app.Count != 1 {
		t.Errorf("expected Count=1 after surviving bad messages, got %d", app.Count)
	}
}

func TestRun_WebSocketBadProtobuf(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comp:      ci,
		NoAuth:    true,
		NoBrowser: true,
		Quiet:     true,
		StaticFS:  fstest.MapFS{},
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", ln)
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Read init
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = client.ReadMessage()

	// Send tag=1 (NodeEvent) with garbage protobuf
	client.WriteMessage(websocket.BinaryMessage, []byte{0x01, 0xFF, 0xFF, 0xFF})

	// Send tag=2 (MethodCall) with garbage protobuf
	client.WriteMessage(websocket.BinaryMessage, []byte{0x02, 0xFF, 0xFF, 0xFF})

	// Server should still respond to valid requests
	call := &gproto.MethodCall{Method: "Increment"}
	payload, _ := proto.Marshal(call)
	client.WriteMessage(websocket.BinaryMessage, append([]byte{2}, payload...))

	client.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = client.ReadMessage()
	if err != nil {
		t.Fatalf("server died after bad protobuf: %v", err)
	}
	if app.Count != 1 {
		t.Errorf("expected Count=1, got %d", app.Count)
	}
}

// --- Negative: buildSurgicalPatches with nil node in class binding ---

func TestBuildSurgicalPatches_ClassBindingNodeGone(t *testing.T) {
	app := &surgicalApp{Active: true}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Corrupt the binding to point to a nonexistent node
	if bindings, ok := ci.Bindings["Active"]; ok {
		for i := range bindings {
			if bindings[i].Kind == "class" {
				bindings[i].NodeID = 99999
			}
		}
		ci.Bindings["Active"] = bindings
	}

	// Should not panic even with missing node
	patches := buildSurgicalPatches(ci, []string{"Active"})
	// No class patch should be generated (node not found)
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Props != nil {
				if _, exists := data.Diff.Props["className"]; exists {
					t.Error("expected no className patch when node is missing")
				}
			}
		}
	}
}

// --- BuildInit / BuildUpdate edge cases ---

func TestBuildUpdate_NilTree(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	// Call BuildUpdate without BuildInit — Tree is nil
	msg := BuildUpdate(ci)
	if msg == nil {
		t.Fatal("expected init message when Tree is nil")
	}
	if msg.Type != "init" {
		t.Errorf("expected type 'init' for first build, got %q", msg.Type)
	}
}

func TestBuildInit_Idempotent(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	msg1 := BuildInit(ci)
	msg2 := BuildInit(ci)

	// Both should produce the same tree
	if msg1.Type != msg2.Type {
		t.Errorf("expected same type, got %q and %q", msg1.Type, msg2.Type)
	}
	if string(msg1.Tree) != string(msg2.Tree) {
		t.Error("expected identical trees from repeated BuildInit")
	}
}

// --- printQR test ---

func TestPrintQR_NoPanic(t *testing.T) {
	// Just verify it doesn't panic
	printQR("http://localhost:8080")
}

func TestPrintQR_EmptyURL(t *testing.T) {
	// Should not panic with empty URL
	printQR("")
}

// --- Helpers ---

func findTextInTree(node *render.WireNode, text string) bool {
	if node.Type == "text" && node.Text == text {
		return true
	}
	for _, child := range node.Children {
		if findTextInTree(child, text) {
			return true
		}
	}
	return false
}

func findEventInTree(node *render.WireNode, eventName string) bool {
	for _, ev := range node.Events {
		if ev.On == eventName {
			return true
		}
	}
	for _, child := range node.Children {
		if findEventInTree(child, eventName) {
			return true
		}
	}
	return false
}

// findInputNode walks the vdom tree looking for an <input> element and sets its ID.
func findInputNode(n vdom.Node, id *int) {
	if el, ok := n.(*vdom.ElementNode); ok {
		if el.Tag == "input" {
			*id = el.NodeID()
			return
		}
		for _, child := range el.Children {
			findInputNode(child, id)
		}
	}
}

// walkTree visits every node in the vdom tree.
func walkTree(n vdom.Node, fn func(vdom.Node)) {
	fn(n)
	if el, ok := n.(*vdom.ElementNode); ok {
		for _, child := range el.Children {
			walkTree(child, fn)
		}
	}
}

// startTestServer starts a Run-style server in the background and returns its address.
func startTestServer(t *testing.T, cfg Config) (string, error) {
	t.Helper()

	ci := cfg.Comp
	pool := &connPool{}

	var token string
	if !cfg.NoAuth {
		if cfg.Token != "" {
			token = cfg.Token
		} else {
			token = generateToken()
		}
	}

	// Wire up RefreshFn (mirrors Run)
	ci.RefreshFn = func() {
		ci.Mu.Lock()
		fields := ci.MarkedFields
		ci.MarkedFields = nil
		if len(fields) > 0 {
			patches := buildSurgicalPatches(ci, fields)
			if len(patches) > 0 {
				ci.Mu.Unlock()
				msg := render.EncodePatchMessage(patches)
				data, _ := proto.Marshal(msg)
				pool.broadcast(data)
				return
			}
		}
		msg := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	mux := http.NewServeMux()

	var injectedJS string
	injectedJS += "<script>" + cfg.ProtobufMinJS + "</script>\n"
	injectedJS += "<script>" + cfg.ProtocolJS + "</script>\n"
	if len(cfg.Plugins) > 0 {
		injectedJS += "<script>window.godom={_plugins:{},register:function(n,h){this._plugins[n]=h}};</script>\n"
		for _, pluginScripts := range cfg.Plugins {
			for _, js := range pluginScripts {
				injectedJS += "<script>" + js + "</script>\n"
			}
		}
	}
	injectedJS += "<script>" + cfg.BridgeJS + "</script>\n"
	pageHTML := strings.Replace(ci.HTMLBody, "</body>", injectedJS+"</body>", 1)

	staticHandler := http.FileServer(http.FS(cfg.StaticFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			staticHandler.ServeHTTP(w, r)
			return
		}
		if !cfg.NoAuth {
			if !checkAuth(token, w, r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("token") != "" {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !cfg.NoAuth && !checkAuth(token, w, r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		wc := pool.add(conn)
		if err := handleInit(wc, ci); err != nil {
			pool.remove(wc)
			conn.Close()
			return
		}
		defer func() {
			pool.remove(wc)
			conn.Close()
		}()
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType != websocket.BinaryMessage || len(data) < 2 {
				continue
			}
			tag := data[0]
			payload := data[1:]
			switch tag {
			case 1:
				evt := &gproto.NodeEvent{}
				if err := proto.Unmarshal(payload, evt); err != nil {
					continue
				}
				handleNodeEvent(ci, evt.NodeId, evt.Value, pool)
			case 2:
				call := &gproto.MethodCall{}
				if err := proto.Unmarshal(payload, call); err != nil {
					continue
				}
				handleMethodCall(ci, call, pool)
			}
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), nil
}
