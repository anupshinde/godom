package server

import (
	"strings"
	"testing"
	"time"

	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestClient_PendingRegistry(t *testing.T) {
	c := &Client{}
	id1, id2 := c.nextCallID(), c.nextCallID()
	if id1 >= 0 || id2 >= 0 || id1 == id2 {
		t.Fatalf("call ids must be negative and unique, got %d and %d", id1, id2)
	}

	var got string
	c.registerCall(id1, func(r []byte, e string) { got = string(r) + "|" + e })
	if !c.handleResult(id1, []byte("ok"), "") {
		t.Error("handleResult should find the pending call")
	}
	if got != "ok|" {
		t.Errorf("callback got %q", got)
	}
	if c.handleResult(id1, nil, "") {
		t.Error("a delivered call must be consumed (second handleResult → false)")
	}
	if c.handleResult(-999, nil, "") {
		t.Error("unknown id must return false")
	}
}

func TestClient_CancelPending(t *testing.T) {
	c := &Client{}
	id := c.nextCallID()
	var gotErr string
	c.registerCall(id, func(r []byte, e string) { gotErr = e })
	c.cancelPending("gone")
	if gotErr != "gone" {
		t.Errorf("cancelPending should fire the callback with the error, got %q", gotErr)
	}
	if c.handleResult(id, nil, "") {
		t.Error("pending calls must be cleared after cancel")
	}
}

func TestModuleCallExpr(t *testing.T) {
	got := moduleCallExpr("widget.render", []byte(`{"w":8}`))
	want := `window.godom.modules.widget.render({"w":8})`
	if got != want {
		t.Errorf("moduleCallExpr = %q, want %q", got, want)
	}
}

func TestClientModulesJS(t *testing.T) {
	if clientModulesJS(nil) != "" {
		t.Error("no modules should produce empty string")
	}
	js := clientModulesJS(map[string]string{"bbb": "B();", "aaa": "A();"})
	if !strings.Contains(js, "g.modules=g.modules||{}") {
		t.Error("missing godom.modules namespace preamble")
	}
	if strings.Index(js, "A();") > strings.Index(js, "B();") {
		t.Error("module scripts should be emitted in sorted (stable) order")
	}
}

// clientWithWS returns a Client backed by a real server-side socket, plus the
// browser-side socket to read JSCALLs from. The read loop isn't running, so
// tests simulate its reply routing by calling client.handleResult directly.
func clientWithWS(t *testing.T) (*Client, *websocket.Conn, *connPool, *wsConn, func()) {
	t.Helper()
	var serverConn *websocket.Conn
	ready := make(chan struct{})
	clientWS, cleanup := wsServer(t, func(c *websocket.Conn) {
		serverConn = c
		close(ready)
		time.Sleep(2 * time.Second)
	})
	<-ready
	pool := &connPool{}
	wc := pool.add(serverConn)
	return wc.client, clientWS, pool, wc, cleanup
}

func readJSCall(t *testing.T, ws *websocket.Conn) *gproto.ServerMessage {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read jscall: %v", err)
	}
	sm := &gproto.ServerMessage{}
	if err := proto.Unmarshal(data, sm); err != nil {
		t.Fatalf("unmarshal jscall: %v", err)
	}
	return sm
}

// Eval sends a JSCALL to exactly one client with a negative (targeted) id, and
// the routed reply reaches the callback.
func TestClient_EvalTargetsOneClient(t *testing.T) {
	client, ws, _, _, cleanup := clientWithWS(t)
	defer cleanup()

	done := make(chan string, 1)
	client.Eval("1+1", func(result []byte, errMsg string) { done <- string(result) + "|" + errMsg })

	sm := readJSCall(t, ws)
	if sm.Kind != gproto.ServerKind_SERVER_JSCALL || sm.Expr != "1+1" {
		t.Fatalf("unexpected jscall: kind=%v expr=%q", sm.Kind, sm.Expr)
	}
	if sm.CallId >= 0 {
		t.Errorf("targeted call id should be negative, got %d", sm.CallId)
	}

	client.handleResult(sm.CallId, []byte("2"), "") // simulate read-loop routing
	if got := <-done; got != "2|" {
		t.Errorf("callback got %q, want \"2|\"", got)
	}
}

// Call marshals args into a module-function expression, blocks for the reply,
// and unmarshals it into out.
func TestClient_CallBlockingRoundtrip(t *testing.T) {
	client, ws, _, _, cleanup := clientWithWS(t)
	defer cleanup()

	go func() {
		sm := readJSCall(t, ws)
		if !strings.HasPrefix(sm.Expr, "window.godom.modules.mod.fn(") {
			t.Errorf("Call expr = %q, want a module call", sm.Expr)
		}
		client.handleResult(sm.CallId, []byte(`{"x":5}`), "")
	}()

	var out struct{ X int }
	if err := client.Call("mod.fn", map[string]int{"a": 1}, &out); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.X != 5 {
		t.Errorf("Call unmarshaled out.X=%d, want 5", out.X)
	}
}

// A JS-side throw surfaces as a Go error.
func TestClient_CallSurfacesJSError(t *testing.T) {
	client, ws, _, _, cleanup := clientWithWS(t)
	defer cleanup()

	go func() {
		sm := readJSCall(t, ws)
		client.handleResult(sm.CallId, nil, "boom")
	}()

	err := client.Call("mod.fn", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Call should surface the JS error, got %v", err)
	}
}

// A disconnect mid-call unblocks Call with an error instead of hanging.
func TestClient_CallUnblocksOnDisconnect(t *testing.T) {
	client, ws, _, _, cleanup := clientWithWS(t)
	defer cleanup()

	go func() {
		readJSCall(t, ws)                           // receive the call, but never reply
		client.cancelPending("client disconnected") // what pool.remove does on close
	}()

	err := client.Call("mod.fn", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Errorf("a disconnect should unblock Call with an error, got %v", err)
	}
}

// connPool.remove must cancel a client's in-flight calls so nothing hangs.
func TestConnPool_RemoveCancelsPendingCalls(t *testing.T) {
	pool := &connPool{}
	wc := pool.add(nil)
	var gotErr string
	wc.client.registerCall(wc.client.nextCallID(), func(r []byte, e string) { gotErr = e })

	pool.remove(wc)

	if gotErr != "client disconnected" {
		t.Errorf("remove should cancel pending calls; callback err = %q", gotErr)
	}
}
