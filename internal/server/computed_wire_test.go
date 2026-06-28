package server

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/anupshinde/godom/internal/island"
	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/anupshinde/godom/internal/vdom"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type computedWireApp struct {
	Island struct{}
	Count  int
	Label  string // computed = "LBL-<Count>"
}

const computedWireHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Label"></span></body></html>`

// End-to-end: marking an input must recompute the dependent computed AND deliver
// the new value in the surgical patch broadcast over the real socket. This closes
// the §1 chain (mark → expand → recompute → surgical patch → wire), beyond the
// unit tests that stop at "the field was recomputed".
func TestComputed_RecomputedValueReachesPatchOnWire(t *testing.T) {
	app := &computedWireApp{}
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(computedWireHTML)
	if err != nil {
		t.Fatal(err)
	}
	ci := &island.Info{Value: v, Typ: v.Elem().Type(), VDOMTemplates: templates}
	if err := ci.RegisterComputed([]island.ComputedDef{
		{Name: "Label", Fn: func() any { return fmt.Sprintf("LBL-%d", app.Count) }, Deps: []string{"Count"}},
	}); err != nil {
		t.Fatal(err)
	}
	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*island.Info{ci},
	}

	// Connect a real browser socket and register its server end in the pool so
	// the broadcast actually goes out over the wire.
	var serverConn *websocket.Conn
	ready := make(chan struct{})
	clientWS, cleanup := wsServer(t, func(c *websocket.Conn) {
		serverConn = c
		close(ready)
		time.Sleep(2 * time.Second)
	})
	defer cleanup()
	<-ready
	ctx.pool.add(serverConn)

	// Change the input and trigger a surgical refresh.
	app.Count = 42
	ci.AddMarkedFields("Count")
	ctx.executeRefresh(ci)

	// The client must receive a surgical PATCH carrying the recomputed value.
	clientWS.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, data, err := clientWS.ReadMessage()
	if err != nil {
		t.Fatalf("reading broadcast patch: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected a binary message, got type %d", mt)
	}
	sm := &gproto.ServerMessage{}
	if err := proto.Unmarshal(data, sm); err != nil {
		t.Fatalf("unmarshal server message: %v", err)
	}
	if sm.Kind != gproto.ServerKind_SERVER_PATCH {
		t.Fatalf("expected a surgical PATCH (not a full rebuild), got kind %v", sm.Kind)
	}
	// The recomputed computed value must be present in the patch on the wire.
	if !bytes.Contains(data, []byte("LBL-42")) {
		t.Errorf("recomputed computed value did not reach the patch on the wire (no LBL-42)")
	}
	// Sanity: the stale value must not be what we ship.
	if bytes.Contains(data, []byte("LBL-0")) {
		t.Errorf("patch carried the stale computed value LBL-0")
	}
}
