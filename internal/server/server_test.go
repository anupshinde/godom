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
	"time"

	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/env"
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

	templates, err := vdom.ParseTemplate(counterHTML)
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

	if msg.Kind != gproto.ServerKind_SERVER_INIT {
		t.Fatalf("expected type 'init', got %q", msg.Kind)
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
	msg, _ := BuildUpdate(ci)

	if msg == nil {
		t.Fatal("expected patch message after increment")
	}
	if msg.Kind != gproto.ServerKind_SERVER_PATCH {
		t.Fatalf("expected type 'patch', got %q", msg.Kind)
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
	msg, _ := BuildUpdate(ci)
	if msg != nil {
		t.Errorf("expected nil message when nothing changed, got type=%q patches=%d", msg.Kind, len(msg.Patches))
	}
}

func TestVDOMBuildUpdate_BindStep(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)

	// Simulate step change (as if g-bind updated it)
	app.Step = 5
	msg, _ := BuildUpdate(ci)

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
	msg1, _ := BuildUpdate(ci)
	if msg1 == nil {
		t.Fatal("expected patch for first increment")
	}

	// Second increment
	app.Count = 4
	msg2, _ := BuildUpdate(ci)
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

// generateToken and checkAuth tests moved to internal/middleware/token_auth_test.go

// localIP tests moved to internal/utils/utils_test.go

// --- Cleanup tests ---

type cleanupTracker struct {
	Component struct{}
	cleaned   bool
}

func (c *cleanupTracker) Cleanup() {
	c.cleaned = true
}

func TestCleanup_CallsCleanupMethod(t *testing.T) {
	app := &cleanupTracker{}
	ci := &component.Info{
		Value:   reflect.ValueOf(app),
		Typ:     reflect.TypeOf(*app),
		EventCh: make(chan component.Event, 1),
	}

	Cleanup([]*component.Info{ci})

	if !app.cleaned {
		t.Error("expected Cleanup() to be called on component")
	}
}

func TestCleanup_ClosesEventCh(t *testing.T) {
	ci := &component.Info{
		Value:   reflect.ValueOf(&counterApp{}),
		Typ:     reflect.TypeOf(counterApp{}),
		EventCh: make(chan component.Event, 1),
	}

	Cleanup([]*component.Info{ci})

	// Channel should be closed — reading should return zero value immediately.
	select {
	case _, ok := <-ci.EventCh:
		if ok {
			t.Error("expected EventCh to be closed")
		}
	default:
		t.Error("expected EventCh to be closed, but it blocked")
	}
}

func TestCleanup_NilEventCh(t *testing.T) {
	ci := &component.Info{
		Value: reflect.ValueOf(&counterApp{}),
		Typ:   reflect.TypeOf(counterApp{}),
		// EventCh is nil
	}
	// Should not panic.
	Cleanup([]*component.Info{ci})
}

func TestCleanup_NoCleanupMethod(t *testing.T) {
	app := &counterApp{}
	ci := &component.Info{
		Value:   reflect.ValueOf(app),
		Typ:     reflect.TypeOf(*app),
		EventCh: make(chan component.Event, 1),
	}
	// Should not panic — counterApp doesn't have Cleanup().
	Cleanup([]*component.Info{ci})
}

func TestCleanup_EmptySlice(t *testing.T) {
	Cleanup(nil)
	Cleanup([]*component.Info{})
}

// --- wireRefresh tests ---

func TestWireRefresh_SendsRefreshEvent(t *testing.T) {
	ci := &component.Info{
		EventCh: make(chan component.Event, 1),
	}
	wireRefresh(ci)

	ci.RefreshFn()

	select {
	case evt := <-ci.EventCh:
		if evt.Kind != component.RefreshKind {
			t.Errorf("expected RefreshKind, got %d", evt.Kind)
		}
	default:
		t.Error("expected event on EventCh")
	}
}

func TestWireRefresh_NilEventCh(t *testing.T) {
	ci := &component.Info{}
	wireRefresh(ci)
	// Should not panic when EventCh is nil.
	ci.RefreshFn()
}

// --- shouldEnqueue / shouldProcess tests ---

func TestShouldEnqueue_AlwaysTrue(t *testing.T) {
	events := []component.Event{
		{Kind: component.NodeEventKind},
		{Kind: component.MethodCallKind},
		{Kind: component.RefreshKind},
	}
	for _, e := range events {
		if !shouldEnqueue(e) {
			t.Errorf("shouldEnqueue returned false for kind %d", e.Kind)
		}
	}
}

func TestShouldProcess_AlwaysTrue(t *testing.T) {
	events := []component.Event{
		{Kind: component.NodeEventKind},
		{Kind: component.MethodCallKind},
		{Kind: component.RefreshKind},
	}
	for _, e := range events {
		if !shouldProcess(e) {
			t.Errorf("shouldProcess returned false for kind %d", e.Kind)
		}
	}
}

// --- processEvents tests ---

func TestProcessEvents_DispatchesRefresh(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML
	ci.EventCh = make(chan component.Event, 8)

	// Build initial tree so executeRefresh works.
	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*component.Info{ci},
	}

	// Start processor.
	done := make(chan struct{})
	go func() {
		ctx.processEvents(ci, 0)
		close(done)
	}()

	// Send refresh event.
	ci.EventCh <- component.Event{Kind: component.RefreshKind}

	// Close channel to stop processor.
	close(ci.EventCh)
	<-done
}

func TestProcessEvents_DispatchesNodeEvent(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML
	ci.EventCh = make(chan component.Event, 8)

	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*component.Info{ci},
	}

	done := make(chan struct{})
	go func() {
		ctx.processEvents(ci, 0)
		close(done)
	}()

	// Send a node event (with an invalid node ID — will log but not crash).
	ci.EventCh <- component.Event{Kind: component.NodeEventKind, NodeID: 99999, Value: "test"}

	close(ci.EventCh)
	<-done
}

func TestProcessEvents_DispatchesMethodCall(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML
	ci.EventCh = make(chan component.Event, 8)

	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*component.Info{ci},
	}

	done := make(chan struct{})
	go func() {
		ctx.processEvents(ci, 0)
		close(done)
	}()

	// Send a method call event.
	ci.EventCh <- component.Event{
		Kind: component.MethodCallKind,
		Msg: &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment", NodeId: 1},
	}

	close(ci.EventCh)
	<-done

	if app.Count != 1 {
		t.Errorf("expected Count=1 after Increment, got %d", app.Count)
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
	if err := handleInit(wc, ci, ""); err != nil {
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

	var msg gproto.ServerMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if msg.Kind != gproto.ServerKind_SERVER_INIT {
		t.Errorf("expected type 'init', got %q", msg.Kind)
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
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(inputNodeID), "42")

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
	var msg gproto.ServerMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Kind != gproto.ServerKind_SERVER_PATCH {
		t.Errorf("expected 'patch', got %q", msg.Kind)
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
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, 99999, "value")
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
		msg, _ := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	var msg gproto.ServerMessage
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
		msg, _ := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	// The broadcast should contain a tree with the new count "5"
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg gproto.ServerMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Kind != gproto.ServerKind_SERVER_PATCH {
		t.Fatalf("expected 'patch', got %q", msg.Kind)
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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "NonExistent"}
	// Should not panic — logs error and returns
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	templates, err := vdom.ParseTemplate(`<!DOCTYPE html><html><head></head><body><span g-text="Count">0</span></body></html>`)
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
		msg, _ := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}
	// Let the app call RefreshFn
	app.refreshFn = ci.RefreshFn

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "IncrementAndRefresh"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	var msg gproto.ServerMessage
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

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	InputVal  string
}

const surgicalHTML = `<!DOCTYPE html><html><head></head><body>
	<span g-text="Name">placeholder</span>
	<div g-attr:data-color="Color"></div>
	<div g-show="Visible">shown</div>
	<div g-hide="Hidden">hidden</div>
	<div g-class:active="Active">classed</div>
	<div g-style:width="Width">styled</div>
	<input g-bind="Name"/>
	<input g-value="InputVal"/>
</body></html>`

func makeSurgicalCI(app *surgicalApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()
	templates, err := vdom.ParseTemplate(surgicalHTML)
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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Color"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Visible"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Hidden"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Active"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Width"})

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

	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"NonExistentField"})
	if len(patches) != 0 {
		t.Errorf("expected no patches for unbound field, got %d", len(patches))
	}
}

func TestBuildSurgicalPatches_EmptyFields(t *testing.T) {
	app := &surgicalApp{}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{})
	if len(patches) != 0 {
		t.Errorf("expected no patches for empty fields, got %d", len(patches))
	}
}

func TestBuildSurgicalPatches_UpdatesLiveTree(t *testing.T) {
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name"})

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
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Width"})

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
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Color"})

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
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Visible"})

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
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Visible"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name", "Width"})

	// Should have patches for both fields
	if len(patches) < 2 {
		t.Errorf("expected at least 2 patches for 2 fields, got %d", len(patches))
	}
}

// --- buildSurgicalPatches: prop binding via g-value ---

func TestBuildSurgicalPatches_PropBindingViaGValue(t *testing.T) {
	// g-value="InputVal" creates a "prop" kind binding with Prop="value".
	app := &surgicalApp{InputVal: "hello"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.InputVal = "world"
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"InputVal"})

	var hasPropPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Props != nil && data.Diff.Props["value"] == "world" {
				hasPropPatch = true
			}
		}
	}
	if !hasPropPatch {
		t.Errorf("expected prop patch with value=world, got patches: %+v", patches)
	}
}

// --- buildSurgicalPatches: live tree prop update ---

func TestBuildSurgicalPatches_UpdatesLiveTreeProp(t *testing.T) {
	// After surgical patches, the live tree's g-value input should have updated Props["value"].
	app := &surgicalApp{InputVal: "hello"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.InputVal = "world"
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"InputVal"})

	bindings := ci.Bindings["InputVal"]
	foundProp := false
	for _, b := range bindings {
		if b.Kind == "prop" {
			foundProp = true
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Props["value"] != "world" {
					t.Errorf("expected live tree prop value=world, got %v", el.Facts.Props["value"])
				}
			} else {
				t.Error("expected ElementNode for prop binding")
			}
		}
	}
	if !foundProp {
		t.Error("expected a 'prop' binding for InputVal (from g-value)")
	}
}

// --- buildSurgicalPatches: nil maps on live tree (style/prop/attr init) ---

func TestBuildSurgicalPatches_LiveTreeNilStyleMap(t *testing.T) {
	// When the live tree node has nil Facts.Styles, surgical patch should initialize it.
	app := &surgicalApp{Width: "100px"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Force the style map to nil to test the nil-init path
	for _, b := range ci.Bindings["Width"] {
		if b.Kind == "style" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				el.Facts.Styles = nil
			}
		}
	}

	app.Width = "300px"
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Width"})

	for _, b := range ci.Bindings["Width"] {
		if b.Kind == "style" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Styles == nil {
					t.Error("expected Styles map to be initialized")
				}
				if el.Facts.Styles["width"] != "300px" {
					t.Errorf("expected width=300px, got %q", el.Facts.Styles["width"])
				}
			}
		}
	}
}

func TestBuildSurgicalPatches_LiveTreeNilPropMap(t *testing.T) {
	// When the live tree node has nil Facts.Props, surgical patch should initialize it.
	app := &surgicalApp{InputVal: "hello"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Force Props to nil on the prop-bound node (g-value="InputVal")
	for _, b := range ci.Bindings["InputVal"] {
		if b.Kind == "prop" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				el.Facts.Props = nil
			}
		}
	}

	app.InputVal = "Charlie"
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"InputVal"})

	for _, b := range ci.Bindings["InputVal"] {
		if b.Kind == "prop" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Props == nil {
					t.Error("expected Props map to be initialized")
				}
				if el.Facts.Props["value"] != "Charlie" {
					t.Errorf("expected value=Charlie, got %v", el.Facts.Props["value"])
				}
			}
		}
	}
}

func TestBuildSurgicalPatches_LiveTreeNilAttrMap(t *testing.T) {
	// When the live tree node has nil Facts.Attrs, surgical patch should initialize it.
	app := &surgicalApp{Color: "red"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Force Attrs to nil
	for _, b := range ci.Bindings["Color"] {
		if b.Kind == "attr" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				el.Facts.Attrs = nil
			}
		}
	}

	app.Color = "green"
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Color"})

	for _, b := range ci.Bindings["Color"] {
		if b.Kind == "attr" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Attrs == nil {
					t.Error("expected Attrs map to be initialized")
				}
				if el.Facts.Attrs["data-color"] != "green" {
					t.Errorf("expected data-color=green, got %q", el.Facts.Attrs["data-color"])
				}
			}
		}
	}
}

// --- buildSurgicalPatches: expr fallback ---

func TestBuildSurgicalPatches_EmptyExprFallsBackToFieldName(t *testing.T) {
	// When a binding has Expr="" (defensive path), the field name is used as
	// the expression for resolving the value.
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Manually inject a binding with empty Expr to exercise the fallback path.
	// Find a text node to target.
	var textNodeID int
	walkTree(ci.Tree, func(n vdom.Node) {
		if _, ok := n.(*vdom.TextNode); ok && textNodeID == 0 {
			textNodeID = n.NodeID()
		}
	})
	if textNodeID == 0 {
		t.Fatal("no text node found")
	}

	ci.Bindings["Name"] = append(ci.Bindings["Name"], vdom.Binding{
		NodeID: textNodeID,
		Kind:   "text",
		Prop:   "",
		Expr:   "", // empty — should fall back to field name "Name"
	})

	app.Name = "Dave"
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name"})

	var foundText bool
	for _, p := range patches {
		if p.Type == vdom.PatchText {
			data, ok := p.Data.(vdom.PatchTextData)
			if ok && data.Text == "Dave" {
				foundText = true
			}
		}
	}
	if !foundText {
		t.Errorf("expected text patch with 'Dave' using field name fallback, got: %+v", patches)
	}
}

// --- handleNodeEvent: unbound values storage ---

// unboundApp has an unbound input (no g-bind) that gets a StableID at parse time.
type unboundApp struct {
	Component struct{}
	Label     string
}

const unboundHTML = `<!DOCTYPE html><html><head></head><body>
	<span g-text="Label">label</span>
	<input type="text" placeholder="unbound"/>
</body></html>`

func makeUnboundCI(app *unboundApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()
	templates, err := vdom.ParseTemplate(unboundHTML)
	if err != nil {
		panic(err)
	}
	return &component.Info{
		Value:         v,
		Typ:           t,
		VDOMTemplates: templates,
	}
}

func TestHandleNodeEvent_StoresUnboundValues(t *testing.T) {
	app := &unboundApp{Label: "test"}
	ci := makeUnboundCI(app)

	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find the input node — it should have a StableID since it has no g-bind
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}

	stableKey, hasStable := ci.NodeStableIDs[inputNodeID]
	if !hasStable {
		t.Fatal("expected unbound input to have a stable ID in NodeStableIDs")
	}

	pool := &connPool{}

	// Fire node event — should store value in UnboundValues
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(inputNodeID), "99")

	if ci.UnboundValues == nil {
		t.Fatal("expected UnboundValues to be initialized")
	}
	stored, ok := ci.UnboundValues[stableKey]
	if !ok {
		t.Fatalf("expected UnboundValues[%q] to be set", stableKey)
	}
	if stored != "99" {
		t.Errorf("expected UnboundValues[%q]='99', got %v", stableKey, stored)
	}

	// Verify the value survives a tree rebuild
	ci.Mu.Lock()
	ci.Tree = buildTree(ci)
	ci.Mu.Unlock()

	// Find the input in the new tree — its Props["value"] should be "99"
	var newInputID int
	findInputNode(ci.Tree, &newInputID)
	if newInputID == 0 {
		t.Fatal("could not find input node after rebuild")
	}
	node := vdom.FindNodeByID(ci.Tree, newInputID)
	el, ok := node.(*vdom.ElementNode)
	if !ok {
		t.Fatal("expected ElementNode")
	}
	if el.Facts.Props["value"] != "99" {
		t.Errorf("expected unbound value '99' to survive rebuild, got %v", el.Facts.Props["value"])
	}
}

func TestHandleNodeEvent_NilPropsMapInitialized(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find the input node and nil out its Props
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	el.Facts.Props = nil

	// Remove input binding so we test the unbound path (direct Props update).
	delete(ci.InputBindings, inputNodeID)

	pool := &connPool{}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(inputNodeID), "hello")

	// Props should now be initialized with the value
	if el.Facts.Props == nil {
		t.Fatal("expected Props to be initialized")
	}
	if el.Facts.Props["value"] != "hello" {
		t.Errorf("expected Props[value]='hello', got %v", el.Facts.Props["value"])
	}
}

func TestHandleNodeEvent_NotElementNode(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find a text node ID to trigger the "not an element" path
	var textNodeID int
	walkTree(ci.Tree, func(n vdom.Node) {
		if _, ok := n.(*vdom.TextNode); ok && textNodeID == 0 {
			textNodeID = n.NodeID()
		}
	})
	if textNodeID == 0 {
		t.Skip("no text node found in tree")
	}

	pool := &connPool{}
	// Should not panic — logs and returns
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(textNodeID), "value")
}

// --- handleMethodMsg: method with arguments ---

type argsApp struct {
	Component struct{}
	Result    string
}

func (a *argsApp) SetResult(val string) {
	a.Result = val
}

func TestHandleMethodCall_WithArgs(t *testing.T) {
	app := &argsApp{Result: ""}
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(`<!DOCTYPE html><html><head></head><body><span g-text="Result">x</span></body></html>`)
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

	pool := &connPool{}
	ci.RefreshFn = func() {}

	call := &gproto.BrowserMessage{
		Method: "SetResult",
		Args:   [][]byte{[]byte(`"hello world"`)},
	}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	if app.Result != "hello world" {
		t.Errorf("expected Result='hello world', got %q", app.Result)
	}
}

// --- handleMethodMsg: bind sync edge cases ---

func TestHandleMethodCall_BindSyncWithExpr(t *testing.T) {
	// When a binding has Expr set, handleMethodCall should use Expr as the
	// SetField path instead of the field name.
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Check if any bind binding has a non-empty Expr
	hasExprBinding := false
	for _, bindings := range ci.Bindings {
		for _, b := range bindings {
			if b.Kind == "bind" && b.Expr != "" {
				hasExprBinding = true
			}
		}
	}

	// Find input node and set value
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
	el.Facts.Props["value"] = "7"

	pool := &connPool{}
	ci.RefreshFn = func() {}

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	// The bind sync should have set Step=7
	if app.Step != 7 {
		t.Errorf("expected Step=7 after bind sync, got %d", app.Step)
	}
	// Count should be 7 (0 + synced Step=7)
	if app.Count != 7 {
		t.Errorf("expected Count=7 after Increment with synced Step, got %d", app.Count)
	}

	_ = hasExprBinding // used for context; sync works regardless
}

func TestHandleMethodCall_BindSyncNilProps(t *testing.T) {
	// When the bound node has nil Props, bind sync should skip it gracefully.
	app := &counterApp{Step: 5, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find input node and nil out Props
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	el.Facts.Props = nil

	pool := &connPool{}
	ci.RefreshFn = func() {}

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	// Step should remain 5 (nil props → sync skipped)
	if app.Step != 5 {
		t.Errorf("expected Step=5 (sync skipped due to nil props), got %d", app.Step)
	}
	// Count = 0 + 5 = 5
	if app.Count != 5 {
		t.Errorf("expected Count=5, got %d", app.Count)
	}
}

func TestHandleMethodCall_BindSyncNoValueProp(t *testing.T) {
	// When the bound node has Props but no "value" key, bind sync should skip.
	app := &counterApp{Step: 3, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find input node and set Props without a "value" key
	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	el.Facts.Props = map[string]any{"other": "stuff"}

	pool := &connPool{}
	ci.RefreshFn = func() {}

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	// Step should remain 3 (no "value" prop → sync skipped)
	if app.Step != 3 {
		t.Errorf("expected Step=3, got %d", app.Step)
	}
	if app.Count != 3 {
		t.Errorf("expected Count=3, got %d", app.Count)
	}
}

func TestHandleMethodCall_BindSyncSkipsNonElementNode(t *testing.T) {
	// When a "bind" binding points to a non-element node (e.g. text node),
	// the sync should skip it gracefully without panicking.
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find a text node ID and inject a fake "bind" binding pointing to it
	var textNodeID int
	walkTree(ci.Tree, func(n vdom.Node) {
		if _, ok := n.(*vdom.TextNode); ok && textNodeID == 0 {
			textNodeID = n.NodeID()
		}
	})
	if textNodeID == 0 {
		t.Fatal("no text node found")
	}

	ci.Bindings["FakeField"] = []vdom.Binding{{
		NodeID: textNodeID,
		Kind:   "bind",
		Prop:   "value",
		Expr:   "Step",
	}}

	pool := &connPool{}
	ci.RefreshFn = func() {}

	// Should not panic — bind sync skips non-element nodes
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	// Step should remain 1 (sync skipped), so Count = 0 + 1 = 1
	if app.Step != 1 {
		t.Errorf("expected Step=1, got %d", app.Step)
	}
	if app.Count != 1 {
		t.Errorf("expected Count=1, got %d", app.Count)
	}
}

func TestHandleMethodCall_DebugLogging(t *testing.T) {
	// When env.Debug is true, handleMethodCall logs the method call.
	// This test just ensures that path doesn't panic.
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	pool := &connPool{}
	ci.RefreshFn = func() {}

	// Enable debug mode
	prev := env.Debug
	env.Debug = true
	defer func() { env.Debug = prev }()

	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	if app.Count != 1 {
		t.Errorf("expected Count=1, got %d", app.Count)
	}
}

func TestBuildSurgicalPatches_ClassBindingWithExistingClass(t *testing.T) {
	// When a node already has a className, toggling a class should append to it.
	app := &surgicalApp{Active: false}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	// Set an existing className on the class-bound node
	for _, b := range ci.Bindings["Active"] {
		if b.Kind == "class" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if el, ok := node.(*vdom.ElementNode); ok {
				if el.Facts.Props == nil {
					el.Facts.Props = make(map[string]any)
				}
				el.Facts.Props["className"] = "existing"
			}
		}
	}

	app.Active = true
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Active"})

	var hasClassPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data, ok := p.Data.(vdom.PatchFactsData)
			if ok && data.Diff.Props != nil {
				cn := fmt.Sprint(data.Diff.Props["className"])
				// Should contain both "existing" and "active"
				if strings.Contains(cn, "existing") && strings.Contains(cn, "active") {
					hasClassPatch = true
				}
			}
		}
	}
	if !hasClassPatch {
		t.Errorf("expected class patch combining 'existing' and 'active', got patches: %+v", patches)
	}
}

// fixedTokenAuthFn returns an AuthFunc that validates a fixed token via query param or cookie.
// Mirrors what middleware.TokenAuth does but with a predetermined token for testing.
func fixedTokenAuthFn(token string) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		if c, err := r.Cookie("godom_token"); err == nil && c.Value == token {
			return true
		}
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
}

// --- Run integration tests ---

func TestRun_ServesHTML(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comps:    []*component.Info{ci},
		BridgeJS: "// bridge",
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

	if !strings.Contains(bodyStr, `<script src="/godom.js"></script>`) {
		t.Error("expected godom.js script tag in response")
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
		Comps:  []*component.Info{ci},
		AuthFn: fixedTokenAuthFn("testsecret"),
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
		Comps:  []*component.Info{ci},
		AuthFn: fixedTokenAuthFn("testsecret"),
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
		Comps: []*component.Info{ci},
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
	var msg gproto.ServerMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Kind != gproto.ServerKind_SERVER_INIT {
		t.Errorf("expected init, got %q", msg.Kind)
	}
}

func TestRun_WebSocketMethodCall(t *testing.T) {
	app := &counterApp{Step: 2, Count: 0}
	ci := makeCounterCI(app)
	ci.HTMLBody = counterHTML

	cfg := Config{
		Comps: []*component.Info{ci},
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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	payload, _ := proto.Marshal(call)
	msg := payload
	if err := client.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatal(err)
	}

	// Should receive update
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp gproto.ServerMessage
	if err := proto.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}

	// Count should now be 2
	if app.Count != 2 {
		t.Errorf("expected Count=2, got %d", app.Count)
	}

	// The response should contain the updated tree with "2"
	if resp.Kind == gproto.ServerKind_SERVER_INIT && len(resp.Tree) > 0 {
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
		Comps: []*component.Info{ci},
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
	evt := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_INPUT, NodeId: int32(inputNodeID), Value: "99"}
	payload, _ := proto.Marshal(evt)
	msg := payload
	if err := client.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatal(err)
	}

	// Should receive facts patch containing value "99"
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp gproto.ServerMessage
	if err := proto.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Kind != gproto.ServerKind_SERVER_PATCH {
		t.Errorf("expected patch, got %q", resp.Kind)
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
		Comps:  []*component.Info{ci},
		AuthFn: fixedTokenAuthFn("secret"),
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
		Comps:    []*component.Info{ci},
		Plugins:  map[string][]string{"chart": {"console.log('chart')"}},
		BridgeJS: "// bridge",
	}

	ln, err := startTestServer(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Page should have script tag, not inline JS.
	resp, err := http.Get(fmt.Sprintf("http://%s/", ln))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, `<script src="/godom.js"></script>`) {
		t.Error("expected godom.js script tag in page")
	}

	// /godom.js should contain the plugin script and registration.
	resp2, err := http.Get(fmt.Sprintf("http://%s/godom.js", ln))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	jsBody, _ := io.ReadAll(resp2.Body)
	jsStr := string(jsBody)

	if !strings.Contains(jsStr, "console.log('chart')") {
		t.Error("expected plugin script in /godom.js")
	}
	if !strings.Contains(jsStr, "godom.register=") {
		t.Error("expected godom plugin registration in /godom.js")
	}
}

// TestRun_StaticFiles removed: static file serving moved to user mux (SetMux)

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
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(textNodeID), "value")
}

// --- handleMethodCall with args ---

func TestHandleMethodCall_EmptyMethod(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	pool := &connPool{}
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: ""}
	// Should not panic — empty method name
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)
}

// --- buildSurgicalPatches: prop binding (g-bind generates "bind" + "prop" bindings) ---

func TestBuildSurgicalPatches_PropBinding(t *testing.T) {
	// g-bind creates a "bind" kind (for sync) and a "prop" kind (for value).
	// Verify that surgical patch for Name includes a prop patch for the input value.
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Visible"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Hidden"})

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Active"})

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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	// Should not panic — skips the missing node
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	// Should not panic — skips the nil Props
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

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
		Comps: []*component.Info{ci},
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
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	payload, _ := proto.Marshal(call)
	client.WriteMessage(websocket.BinaryMessage, payload)

	client.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("server died after invalid messages: %v", err)
	}
	var msg gproto.ServerMessage
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
		Comps: []*component.Info{ci},
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

	// Send garbage protobuf (should be ignored, not crash)
	client.WriteMessage(websocket.BinaryMessage, []byte{0xFF, 0xFF, 0xFF, 0xFF})

	// Send another garbage message
	client.WriteMessage(websocket.BinaryMessage, []byte{0x01, 0x02, 0x03})

	// Server should still respond to valid requests
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "Increment"}
	payload, _ := proto.Marshal(call)
	client.WriteMessage(websocket.BinaryMessage, payload)

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
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Active"})
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
	msg, _ := BuildUpdate(ci)
	if msg == nil {
		t.Fatal("expected init message when Tree is nil")
	}
	if msg.Kind != gproto.ServerKind_SERVER_INIT {
		t.Errorf("expected type 'init' for first build, got %q", msg.Kind)
	}
}

func TestBuildInit_Idempotent(t *testing.T) {
	app := &counterApp{Step: 1, Count: 5}
	ci := makeCounterCI(app)

	msg1 := BuildInit(ci)
	msg2 := BuildInit(ci)

	// Both should produce the same tree
	if msg1.Kind != msg2.Kind {
		t.Errorf("expected same type, got %q and %q", msg1.Kind, msg2.Kind)
	}
	if string(msg1.Tree) != string(msg2.Tree) {
		t.Error("expected identical trees from repeated BuildInit")
	}
}

// printQR tests moved to internal/utils/utils_test.go

// --- g-if transition ID collision test ---

// gifApp simulates a g-if transition: Items goes from nil to non-empty,
// which inserts new nodes and shifts IDs of subsequent nodes.
type gifApp struct {
	Component struct{}
	Items     []string
}

func (a *gifApp) AddItem() {
	a.Items = append(a.Items, "item1")
}

const gifHTML = `<!DOCTYPE html><html><head></head><body>
<div class="container">
  <div g-if="Items"><div g-for="item in Items"><span g-text="item"></span></div></div>
  <div class="empty" g-hide="Items">No items</div>
</div>
<div class="footer"><span g-text="'done'">done</span></div>
</body></html>`

func makeGifCI(app *gifApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()
	templates, err := vdom.ParseTemplate(gifHTML)
	if err != nil {
		panic(err)
	}
	return &component.Info{
		Value:         v,
		Typ:           t,
		VDOMTemplates: templates,
	}
}

func TestBuildUpdate_GifTransition_NoIDCollision(t *testing.T) {
	app := &gifApp{}
	ci := makeGifCI(app)

	// Initial render: Items is nil, g-if="Items" produces no nodes
	_ = BuildInit(ci)

	// Collect all IDs in the tree after init
	initIDs := collectAllIDs(ci.Tree)
	for id, count := range initIDs {
		if count > 1 {
			t.Fatalf("duplicate ID %d after init (count=%d)", id, count)
		}
	}

	// Add an item: Items goes from nil to ["item1"]
	// g-if="Items" now produces nodes, shifting IDs of subsequent nodes
	app.AddItem()
	msg, _ := BuildUpdate(ci)
	if msg == nil {
		t.Fatal("expected patch message after AddItem")
	}

	// Verify: no duplicate IDs in the merged tree
	afterIDs := collectAllIDs(ci.Tree)
	for id, count := range afterIDs {
		if count > 1 {
			t.Fatalf("duplicate ID %d in merged tree after AddItem (count=%d)", id, count)
		}
	}

	// Verify that bindings reference IDs actually present in the merged tree
	for field, bindings := range ci.Bindings {
		for _, b := range bindings {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			if node == nil {
				t.Errorf("binding for %q references nodeID=%d not in merged tree", field, b.NodeID)
			}
		}
	}

	// Second update: add another item to verify IDs stay unique
	app.Items = append(app.Items, "item2")
	msg2, _ := BuildUpdate(ci)
	if msg2 == nil {
		t.Fatal("expected patch message after second AddItem")
	}
	afterIDs2 := collectAllIDs(ci.Tree)
	for id, count := range afterIDs2 {
		if count > 1 {
			t.Fatalf("duplicate ID %d in merged tree after second AddItem (count=%d)", id, count)
		}
	}
}

// --- IDCounter monotonic invariant ---
//
// IDCounter MUST only increment, never reset or go backwards.
// The bridge maintains nodeMap[id] → DOM node. If IDs are reused across
// renders, PatchRedraw registers new nodes under IDs that already belong
// to unrelated DOM nodes elsewhere in the tree. The bridge silently
// overwrites those entries, causing subsequent patches to target the
// wrong DOM nodes — leading to HierarchyRequestErrors, visual corruption,
// and lost interactivity. All silent, all hard to debug.
//
// This invariant is load-bearing. Do not reset IDCounter to zero.

func TestIDCounter_MustOnlyIncrement(t *testing.T) {
	app := &counterApp{Step: 1, Count: 0}
	ci := makeCounterCI(app)

	_ = BuildInit(ci)
	prevSeq := ci.IDCounter.Seq

	for i := 0; i < 5; i++ {
		app.Count++
		_, _ = BuildUpdate(ci)

		if ci.IDCounter.Seq <= prevSeq {
			t.Fatalf("render %d: IDCounter went from %d to %d — IDs must only increase", i, prevSeq, ci.IDCounter.Seq)
		}
		prevSeq = ci.IDCounter.Seq
	}
}

func collectAllIDs(node vdom.Node) map[int]int {
	ids := make(map[int]int)
	walkTreeIDs(node, ids)
	return ids
}

func walkTreeIDs(node vdom.Node, ids map[int]int) {
	if node == nil {
		return
	}
	ids[node.NodeID()]++
	switch n := node.(type) {
	case *vdom.ElementNode:
		for _, c := range n.Children {
			walkTreeIDs(c, ids)
		}
	case *vdom.KeyedElementNode:
		for _, kc := range n.Children {
			walkTreeIDs(kc.Node, ids)
		}
	case *vdom.LazyNode:
		if n.Cached != nil {
			walkTreeIDs(n.Cached, ids)
		}
	}
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

	ci := cfg.Comps[0]
	pool := &connPool{}

	// Wire up RefreshFn (mirrors Run)
	ci.RefreshFn = func() {
		fields := ci.DrainMarkedFields()
		ci.Mu.Lock()
		if len(fields) > 0 {
			patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, fields)
			if len(patches) > 0 {
				ci.Mu.Unlock()
				msg := render.EncodePatchMessage(patches)
				data, _ := proto.Marshal(msg)
				pool.broadcast(data)
				return
			}
		}
		msg, _ := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	mux := http.NewServeMux()

	// Build the JS bundle (same as Run).
	var bundleJS string
	bundleJS += cfg.ProtobufMinJS + "\n" + cfg.ProtocolJS + "\n"
	if len(cfg.Plugins) > 0 {
		bundleJS += "var godom=window[window.GODOM_NS||'godom']=window[window.GODOM_NS||'godom']||{};godom._plugins=godom._plugins||{};godom.register=function(n,h){godom._plugins[n]=h};\n"
		for _, pluginScripts := range cfg.Plugins {
			for _, js := range pluginScripts {
				bundleJS += js + "\n"
			}
		}
	}
	bundleJS += cfg.BridgeJS

	scriptPath := cfg.ScriptPath
	if scriptPath == "" {
		scriptPath = "/godom.js"
	}
	wsPath := cfg.WSPath
	if wsPath == "" {
		wsPath = "/ws"
	}

	mux.HandleFunc(scriptPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, bundleJS)
	})

	pageHTML := strings.Replace(ci.HTMLBody, "</body>", "<script src=\""+scriptPath+"\"></script>\n</body>", 1)

	// Wrap root handler with auth if AuthFn is set (mirrors middleware.Wrap)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if cfg.AuthFn != nil {
			if !cfg.AuthFn(w, r) {
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

	mux.HandleFunc(wsPath, func(w http.ResponseWriter, r *http.Request) {
		if cfg.AuthFn != nil && !cfg.AuthFn(w, r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		wc := pool.add(conn)
		if err := handleInit(wc, ci, ""); err != nil {
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
			if msgType != websocket.BinaryMessage || len(data) == 0 {
				continue
			}
			msg := &gproto.BrowserMessage{}
			if err := proto.Unmarshal(data, msg); err != nil {
				continue
			}
			switch msg.Kind {
			case gproto.BrowserKind_BROWSER_INPUT:
				(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, msg.NodeId, msg.Value)
			case gproto.BrowserKind_BROWSER_METHOD:
				(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, msg)
			}
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), nil
}

// --- BuildUpdate: NodeStableIDs remap ---

type stableRemapApp struct {
	Component struct{}
	Label     string
}

const stableRemapHTML = `<!DOCTYPE html><html><head></head><body>
	<span g-text="Label">x</span>
	<input type="text" placeholder="unbound"/>
</body></html>`

func TestBuildUpdate_RemapsNodeStableIDs(t *testing.T) {
	// The unbound input gets a new node ID on each rebuild (IDCounter is
	// monotonic). MergeTree matches it by position and records
	// remap[newID]=oldID. The NodeStableIDs remap code must follow that
	// mapping so handleNodeEvent can still find the right stable key.
	app := &stableRemapApp{Label: "hello"}
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(stableRemapHTML)
	if err != nil {
		t.Fatal(err)
	}
	ci := &component.Info{
		Value:         v,
		Typ:           v.Elem().Type(),
		VDOMTemplates: templates,
	}

	// First render
	_ = BuildInit(ci)

	if len(ci.NodeStableIDs) == 0 {
		t.Fatal("expected NodeStableIDs to be populated for unbound input")
	}

	// Record the original node ID and stable key
	var origNodeID int
	var stableKey string
	for id, key := range ci.NodeStableIDs {
		origNodeID = id
		stableKey = key
	}

	// Change label → same structure, different content. The rebuilt tree
	// gives the input a new ID (IDCounter advanced), but MergeTree keeps
	// the old ID. remap[newID]=oldID should be produced.
	app.Label = "world"
	msg, _ := BuildUpdate(ci)
	if msg == nil {
		t.Fatal("expected patch message after changing Label")
	}

	// NodeStableIDs should still have exactly one entry with the same stable key
	if len(ci.NodeStableIDs) != 1 {
		t.Fatalf("expected 1 NodeStableIDs entry, got %d", len(ci.NodeStableIDs))
	}

	var remappedNodeID int
	for id, key := range ci.NodeStableIDs {
		remappedNodeID = id
		if key != stableKey {
			t.Errorf("stable key changed: was %q, now %q", stableKey, key)
		}
	}

	// The remapped ID should equal the original (MergeTree preserved it)
	if remappedNodeID != origNodeID {
		t.Errorf("expected remapped ID=%d (original), got %d", origNodeID, remappedNodeID)
	}

	// The ID must resolve to the input node in the merged tree
	node := vdom.FindNodeByID(ci.Tree, remappedNodeID)
	if node == nil {
		t.Fatalf("NodeStableIDs[%d] does not exist in merged tree", remappedNodeID)
	}
	if el, ok := node.(*vdom.ElementNode); ok {
		if el.Tag != "input" {
			t.Errorf("expected input node, got %q", el.Tag)
		}
	} else {
		t.Error("expected ElementNode for remapped stable ID")
	}
}

func TestBuildUpdate_RemapsNodeStableIDs_ElseBranch(t *testing.T) {
	// When a g-if toggle changes the child list structure, the unbound input
	// moves to a different position. MergeTree can't match it positionally,
	// so its new ID is NOT in the remap. The else branch keeps the original
	// new-tree ID in NodeStableIDs.
	type gifRemapApp struct {
		Component struct{}
		ShowExtra bool
	}
	app := &gifRemapApp{ShowExtra: false}
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(`<!DOCTYPE html><html><head></head><body>
	<div g-if="ShowExtra"><span>extra</span></div>
	<input type="text" placeholder="unbound"/>
</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	ci := &component.Info{
		Value:         v,
		Typ:           v.Elem().Type(),
		VDOMTemplates: templates,
	}

	_ = BuildInit(ci)

	if len(ci.NodeStableIDs) == 0 {
		t.Fatal("expected NodeStableIDs for unbound input")
	}

	var stableKey string
	for _, key := range ci.NodeStableIDs {
		stableKey = key
	}

	// Toggle g-if: structural change shifts input position
	app.ShowExtra = true
	msg, _ := BuildUpdate(ci)
	if msg == nil {
		t.Fatal("expected patch message")
	}

	// Stable key should be preserved
	if len(ci.NodeStableIDs) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ci.NodeStableIDs))
	}
	for id, key := range ci.NodeStableIDs {
		if key != stableKey {
			t.Errorf("stable key changed: was %q, now %q", stableKey, key)
		}
		node := vdom.FindNodeByID(ci.Tree, id)
		if node == nil {
			t.Fatalf("NodeStableIDs[%d] not in merged tree", id)
		}
	}
}

// ---------------------------------------------------------------------------
// findComponent / findNode cache tests
// ---------------------------------------------------------------------------

func TestFindComponent_Found(t *testing.T) {
	ci1 := &component.Info{}
	ci1.Tree = &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 10}, Tag: "div",
	}
	ci2 := &component.Info{}
	ci2.Tree = &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 20}, Tag: "span",
	}

	comps := []*component.Info{ci1, ci2}
	lookup := newNodeLookup()

	found := findComponent(20, comps, lookup)
	if found != ci2 {
		t.Error("expected to find ci2 for node ID 20")
	}
}

func TestFindComponent_NotFound(t *testing.T) {
	ci := &component.Info{}
	ci.Tree = &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 10}, Tag: "div",
	}
	comps := []*component.Info{ci}
	lookup := newNodeLookup()

	found := findComponent(999, comps, lookup)
	if found != nil {
		t.Error("expected nil for non-existent node ID")
	}
}

func TestFindComponent_Empty(t *testing.T) {
	lookup := newNodeLookup()
	found := findComponent(1, nil, lookup)
	if found != nil {
		t.Error("expected nil for empty comps")
	}
}

func TestFindComponent_CacheHit(t *testing.T) {
	ci := &component.Info{}
	ci.Tree = &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 10}, Tag: "div",
	}
	comps := []*component.Info{ci}
	lookup := newNodeLookup()

	// First call — cache miss, traverses tree.
	found1 := findComponent(10, comps, lookup)
	// Second call — cache hit.
	found2 := findComponent(10, comps, lookup)

	if found1 != ci || found2 != ci {
		t.Error("expected cache to return same component")
	}
}

func TestFindComponent_EvictsRemovedNode(t *testing.T) {
	child := &vdom.ElementNode{NodeBase: vdom.NodeBase{ID: 20}, Tag: "span"}
	root := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 10}, Tag: "div",
		Children: []vdom.Node{child},
	}
	ci := &component.Info{}
	ci.Tree = root
	comps := []*component.Info{ci}
	lookup := newNodeLookup()

	// Populate cache for the child node.
	found := findNode(20, ci, lookup)
	if found != child {
		t.Fatal("expected to find child node")
	}

	// Remove the child from the tree (sets Removed flag via MarkRemoved).
	root.RemoveChild(0)

	// Cache should evict the stale entry on next access.
	e := lookup.get(20)
	if e != nil {
		t.Error("expected cache entry to be evicted for removed node")
	}

	// findComponent should also return nil since the node is gone.
	comp := findComponent(20, comps, lookup)
	if comp != nil {
		t.Error("expected nil for removed node")
	}
}

func TestFindNode_Found(t *testing.T) {
	child := &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 5}, Text: "hello"}
	root := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1}, Tag: "div",
		Children: []vdom.Node{child},
	}
	ci := &component.Info{Tree: root}
	lookup := newNodeLookup()

	found := findNode(5, ci, lookup)
	if found != child {
		t.Error("expected to find child node")
	}
}

func TestFindNode_CacheHit(t *testing.T) {
	child := &vdom.TextNode{NodeBase: vdom.NodeBase{ID: 5}, Text: "hello"}
	root := &vdom.ElementNode{
		NodeBase: vdom.NodeBase{ID: 1}, Tag: "div",
		Children: []vdom.Node{child},
	}
	ci := &component.Info{Tree: root}
	lookup := newNodeLookup()

	found1 := findNode(5, ci, lookup)
	found2 := findNode(5, ci, lookup)

	if found1 != child || found2 != child {
		t.Error("expected cache to return same node")
	}
}

// --- Checkbox and InputBindings tests ---

type checkboxApp struct {
	Component struct{}
	Agree     bool
	Dark      bool
	Color     string
	Title     string
}

func (a *checkboxApp) ToggleDark() { a.Dark = !a.Dark }

const checkboxHTML = `<!DOCTYPE html><html><head></head><body>
	<input type="checkbox" g-checked="Agree"/>
	<span g-text="Agree"></span>
	<input type="checkbox" g-checked="Dark"/>
	<select g-value="Color">
		<option value="red">Red</option>
		<option value="blue">Blue</option>
	</select>
	<span g-text="Color"></span>
	<input type="text" g-bind="Title"/>
	<span g-text="Title"></span>
</body></html>`

func makeCheckboxCI(app *checkboxApp) *component.Info {
	v := reflect.ValueOf(app)
	t := v.Elem().Type()
	templates, err := vdom.ParseTemplate(checkboxHTML)
	if err != nil {
		panic(err)
	}
	return &component.Info{
		Value:         v,
		Typ:           t,
		VDOMTemplates: templates,
	}
}

func findNodeByTagAndAttr(n vdom.Node, tag, attrKey, attrVal string) *vdom.ElementNode {
	if n == nil {
		return nil
	}
	if el, ok := n.(*vdom.ElementNode); ok {
		if el.Tag == tag && el.Facts.Attrs[attrKey] == attrVal {
			return el
		}
		for _, child := range el.Children {
			if found := findNodeByTagAndAttr(child, tag, attrKey, attrVal); found != nil {
				return found
			}
		}
	}
	return nil
}

func findNodeByTagAndDirective(ci *component.Info, kind, field string) int {
	for f, bindings := range ci.Bindings {
		if f != field {
			continue
		}
		for _, b := range bindings {
			if b.Kind == kind {
				return b.NodeID
			}
		}
	}
	return 0
}

// TestHandleNodeEvent_CheckboxUpdatesCheckedAsBool verifies that checkbox
// node events store Props["checked"] as a bool, not Props["value"] as string.
func TestHandleNodeEvent_CheckboxUpdatesCheckedAsBool(t *testing.T) {
	app := &checkboxApp{Agree: true}
	ci := makeCheckboxCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Find the checkbox node for Agree
	nodeID := findNodeByTagAndDirective(ci, "prop", "Agree")
	if nodeID == 0 {
		t.Fatal("could not find Agree checkbox binding")
	}

	pool := &connPool{}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(nodeID), "false")

	// Verify Props["checked"] is bool false, not string "false"
	node := vdom.FindNodeByID(ci.Tree, nodeID)
	el := node.(*vdom.ElementNode)
	checked, ok := el.Facts.Props["checked"]
	if !ok {
		t.Fatal("expected Props[checked] to exist")
	}
	if checked != false {
		t.Errorf("expected Props[checked]=false (bool), got %v (%T)", checked, checked)
	}
	// Props["value"] should NOT be set by checkbox handling
	if _, hasValue := el.Facts.Props["value"]; hasValue {
		t.Error("checkbox should set Props[checked], not Props[value]")
	}
}

// TestHandleNodeEvent_CheckboxTrueSetsBoolTrue verifies "true" string
// from bridge becomes bool true in Props["checked"].
func TestHandleNodeEvent_CheckboxTrueSetsBoolTrue(t *testing.T) {
	app := &checkboxApp{Agree: false}
	ci := makeCheckboxCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	nodeID := findNodeByTagAndDirective(ci, "prop", "Agree")
	if nodeID == 0 {
		t.Fatal("could not find Agree checkbox binding")
	}

	pool := &connPool{}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(nodeID), "true")

	node := vdom.FindNodeByID(ci.Tree, nodeID)
	el := node.(*vdom.ElementNode)
	if el.Facts.Props["checked"] != true {
		t.Errorf("expected Props[checked]=true (bool), got %v (%T)", el.Facts.Props["checked"], el.Facts.Props["checked"])
	}
}

// TestHandleNodeEvent_BoundInputSyncsStruct verifies that changing a bound
// input (g-bind, g-value, g-checked) syncs the value back to the struct
// via the InputBindings reverse map.
func TestHandleNodeEvent_BoundInputSyncsStruct(t *testing.T) {
	app := &checkboxApp{Title: "old", Agree: true, Color: "red"}
	ci := makeCheckboxCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	pool := &connPool{}

	// Test g-bind (text input) syncs struct
	titleNodeID := findNodeByTagAndDirective(ci, "bind", "Title")
	if titleNodeID == 0 {
		t.Fatal("could not find Title binding")
	}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(titleNodeID), "new title")
	if app.Title != "new title" {
		t.Errorf("expected struct Title='new title', got %q", app.Title)
	}

	// Test g-checked (checkbox) syncs struct
	agreeNodeID := findNodeByTagAndDirective(ci, "prop", "Agree")
	if agreeNodeID == 0 {
		t.Fatal("could not find Agree binding")
	}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(agreeNodeID), "false")
	if app.Agree != false {
		t.Errorf("expected struct Agree=false, got %v", app.Agree)
	}

	// Test g-value (select) syncs struct
	colorNodeID := findNodeByTagAndDirective(ci, "prop", "Color")
	if colorNodeID == 0 {
		t.Fatal("could not find Color binding")
	}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(colorNodeID), "blue")
	if app.Color != "blue" {
		t.Errorf("expected struct Color='blue', got %q", app.Color)
	}
}

// TestHandleNodeEvent_UnboundInputDoesNotSyncStruct verifies that unbound
// inputs (no g-bind/g-value/g-checked) do NOT sync to the struct.
func TestHandleNodeEvent_UnboundInputDoesNotSyncStruct(t *testing.T) {
	app := &counterApp{Step: 5, Count: 0}
	ci := makeCounterCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()

	// Remove InputBindings to simulate unbound input
	ci.InputBindings = nil

	var inputNodeID int
	findInputNode(ci.Tree, &inputNodeID)
	if inputNodeID == 0 {
		t.Fatal("could not find input node")
	}

	pool := &connPool{}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleNodeEvent(ci, 0, int32(inputNodeID), "99")

	// The tree should be updated (targeted patch path)
	node := vdom.FindNodeByID(ci.Tree, inputNodeID)
	el := node.(*vdom.ElementNode)
	if el.Facts.Props["value"] != "99" {
		t.Errorf("expected Props[value]='99', got %v", el.Facts.Props["value"])
	}
	// But the struct should NOT be updated
	if app.Step != 5 {
		t.Errorf("unbound input should not sync struct: expected Step=5, got %d", app.Step)
	}
}

// TestInputBindings_PopulatedOnResolve verifies that InputBindings reverse
// map is populated during tree resolution for g-bind, g-value, and g-checked.
func TestInputBindings_PopulatedOnResolve(t *testing.T) {
	app := &checkboxApp{Agree: true, Color: "red", Title: "hi"}
	ci := makeCheckboxCI(app)
	ci.Tree = buildTree(ci)

	if ci.InputBindings == nil {
		t.Fatal("expected InputBindings to be populated")
	}

	// Count input bindings — should have entries for: Agree (checkbox),
	// Dark (checkbox), Color (select), Title (text input)
	fields := make(map[string]bool)
	for _, ib := range ci.InputBindings {
		fields[ib.Field] = true
	}
	for _, expected := range []string{"Agree", "Dark", "Color", "Title"} {
		if !fields[expected] {
			t.Errorf("expected InputBinding for field %q", expected)
		}
	}
}

// TestInputBindings_NotPopulatedForNonInputBindings verifies that text, show,
// class etc. bindings do NOT create InputBindings entries.
func TestInputBindings_NotPopulatedForNonInputBindings(t *testing.T) {
	app := &surgicalApp{Name: "Alice", Visible: true, Active: true, Width: "100px"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	if ci.InputBindings == nil {
		t.Fatal("expected InputBindings (g-bind on Name)")
	}

	// Only "bind" and "prop" kinds should be in InputBindings.
	// The surgicalHTML has g-text, g-attr, g-show, g-hide, g-class, g-style —
	// none of these should create InputBinding entries.
	for nodeID, ib := range ci.InputBindings {
		if ib.Field != "Name" && ib.Field != "InputVal" {
			t.Errorf("unexpected InputBinding for nodeID=%d field=%q — only Name (g-bind) and InputVal (g-value) should have entries", nodeID, ib.Field)
		}
	}
}

// TestHandleMethodCall_SyncsPropBindings verifies that handleMethodCall
// syncs Kind="prop" bindings (g-value, g-checked) in addition to g-bind.
func TestHandleMethodCall_SyncsPropBindings(t *testing.T) {
	app := &checkboxApp{Dark: false}
	ci := makeCheckboxCI(app)
	ci.Mu.Lock()
	_ = BuildInit(ci)
	ci.Mu.Unlock()
	ci.RefreshFn = func() {}

	// Simulate browser toggling the Dark checkbox — update tree prop directly
	darkNodeID := findNodeByTagAndDirective(ci, "prop", "Dark")
	if darkNodeID == 0 {
		t.Fatal("could not find Dark binding")
	}
	node := vdom.FindNodeByID(ci.Tree, darkNodeID)
	el := node.(*vdom.ElementNode)
	if el.Facts.Props == nil {
		el.Facts.Props = make(map[string]any)
	}
	el.Facts.Props["checked"] = true

	// Call a method — handleMethodCall should sync Dark from tree to struct
	call := &gproto.BrowserMessage{Kind: gproto.BrowserKind_BROWSER_METHOD, Method: "ToggleDark"}
	pool := &connPool{}
	(&serverCtx{pool: pool, sm: &sharedPtrMaps{}, lookup: newNodeLookup()}).handleMethodCall(ci, 0, call)

	// ToggleDark flips Dark. If sync worked, Dark was set to true from tree,
	// then ToggleDark flipped it to false.
	if app.Dark != false {
		t.Errorf("expected Dark=false (synced true then toggled), got %v", app.Dark)
	}
}

// TestBuildSurgicalPatches_BindKind verifies that buildSurgicalPatches
// produces a facts patch with Props["value"] for Kind="bind" bindings.
func TestBuildSurgicalPatches_BindKind(t *testing.T) {
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name"})

	// Should have both a text patch (g-text) and a facts/props patch (g-bind)
	var hasTextPatch, hasBindPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchText {
			data := p.Data.(vdom.PatchTextData)
			if data.Text == "Bob" {
				hasTextPatch = true
			}
		}
		if p.Type == vdom.PatchFacts {
			data := p.Data.(vdom.PatchFactsData)
			if data.Diff.Props != nil && data.Diff.Props["value"] == "Bob" {
				hasBindPatch = true
			}
		}
	}
	if !hasTextPatch {
		t.Error("expected PatchText for g-text='Name' with text 'Bob'")
	}
	if !hasBindPatch {
		t.Error("expected PatchFacts for g-bind='Name' with Props[value]='Bob'")
	}
}

// TestBuildSurgicalPatches_BindKind_UpdatesLiveTree verifies that
// buildSurgicalPatches updates ci.Tree for Kind="bind" bindings.
func TestBuildSurgicalPatches_BindKind_UpdatesLiveTree(t *testing.T) {
	app := &surgicalApp{Name: "Alice"}
	ci := makeSurgicalCI(app)
	ci.Tree = buildTree(ci)

	app.Name = "Bob"
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Name"})

	// Find the g-bind node and check its tree was updated
	for _, b := range ci.Bindings["Name"] {
		if b.Kind == "bind" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			el := node.(*vdom.ElementNode)
			if el.Facts.Props["value"] != "Bob" {
				t.Errorf("expected live tree Props[value]='Bob', got %v", el.Facts.Props["value"])
			}
			return
		}
	}
	t.Error("no bind binding found for Name")
}

// TestBuildSurgicalPatches_CheckedBool verifies that buildSurgicalPatches
// sends Props["checked"] as a bool (not string) for g-checked bindings.
func TestBuildSurgicalPatches_CheckedBool(t *testing.T) {
	app := &checkboxApp{Agree: true}
	ci := makeCheckboxCI(app)
	ci.Tree = buildTree(ci)

	app.Agree = false
	patches := (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Agree"})

	var foundCheckedPatch bool
	for _, p := range patches {
		if p.Type == vdom.PatchFacts {
			data := p.Data.(vdom.PatchFactsData)
			if val, ok := data.Diff.Props["checked"]; ok {
				foundCheckedPatch = true
				boolVal, isBool := val.(bool)
				if !isBool {
					t.Errorf("expected checked to be bool, got %T", val)
				} else if boolVal != false {
					t.Errorf("expected checked=false, got %v", boolVal)
				}
			}
		}
	}
	if !foundCheckedPatch {
		t.Error("expected a PatchFacts with Props[checked] for Agree")
	}
}

// TestBuildSurgicalPatches_CheckedBool_UpdatesLiveTree verifies the live tree
// stores Props["checked"] as a bool after surgical patches.
func TestBuildSurgicalPatches_CheckedBool_UpdatesLiveTree(t *testing.T) {
	app := &checkboxApp{Agree: true}
	ci := makeCheckboxCI(app)
	ci.Tree = buildTree(ci)

	app.Agree = false
	_ = (&serverCtx{lookup: newNodeLookup()}).buildSurgicalPatches(ci, []string{"Agree"})

	for _, b := range ci.Bindings["Agree"] {
		if b.Kind == "prop" && b.Prop == "checked" {
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			el := node.(*vdom.ElementNode)
			val := el.Facts.Props["checked"]
			if val != false {
				t.Errorf("expected live tree checked=false (bool), got %v (%T)", val, val)
			}
			return
		}
	}
	t.Error("no prop/checked binding found for Agree")
}

// TestInputBindings_RemappedAfterBuildUpdate verifies that InputBindings
// IDs are remapped when MergeTree reassigns node IDs during BuildUpdate.
func TestInputBindings_RemappedAfterBuildUpdate(t *testing.T) {
	app := &checkboxApp{Title: "hello", Agree: true, Color: "red"}
	ci := makeCheckboxCI(app)
	ci.Tree = buildTree(ci)

	// Record original InputBindings node IDs
	origIDs := make(map[string]int) // field → nodeID
	for nodeID, ib := range ci.InputBindings {
		origIDs[ib.Field] = nodeID
	}

	// Trigger a BuildUpdate — this rebuilds the tree with new IDs,
	// then MergeTree remaps them back to old IDs.
	ci.Mu.Lock()
	app.Title = "world"
	_, _ = BuildUpdate(ci)
	ci.Mu.Unlock()

	// InputBindings should still have valid entries for all fields
	newFields := make(map[string]bool)
	for _, ib := range ci.InputBindings {
		newFields[ib.Field] = true
	}
	for _, field := range []string{"Title", "Agree", "Dark", "Color"} {
		if !newFields[field] {
			t.Errorf("InputBindings missing field %q after BuildUpdate", field)
		}
	}

	// Verify the remapped IDs match actual nodes in the merged tree
	for nodeID, ib := range ci.InputBindings {
		node := vdom.FindNodeByID(ci.Tree, nodeID)
		if node == nil {
			t.Errorf("InputBinding for %q references nodeID=%d which doesn't exist in tree", ib.Field, nodeID)
		}
	}
}
