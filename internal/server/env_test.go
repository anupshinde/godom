package server

import (
	"reflect"
	"testing"
	"time"

	"github.com/anupshinde/godom/internal/island"
	"github.com/anupshinde/godom/internal/vdom"
)

type envApp struct {
	Island struct{}
	TZ     string
	W      int
}

func (a *envApp) OnConnect(c *Client) {
	e := c.Env()
	a.TZ = e.TimeZone
	a.W = e.Viewport.W
}

const envHTML = `<!DOCTYPE html><html><head></head><body><span g-text="TZ"></span></body></html>`

// A delivered env payload is stored on the client and OnConnect is dispatched on
// the island loop, where it can seed state from the connection's environment.
func TestApplyClientEnv_SetsEnvAndRunsOnConnect(t *testing.T) {
	app := &envApp{}
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(envHTML)
	if err != nil {
		t.Fatal(err)
	}
	ci := &island.Info{Value: v, Typ: v.Elem().Type(), VDOMTemplates: templates}
	ci.EventCh = make(chan island.Event, 64)
	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*island.Info{ci},
	}
	done := make(chan struct{})
	go func() { ctx.processEvents(ci, 0); close(done) }()
	defer func() { close(ci.EventCh); <-done }()

	// Construct the client directly so it is NOT in the broadcast pool (an empty
	// pool means the OnConnect-triggered refresh has nothing to write to).
	client := &Client{id: "test"}
	envJSON := []byte(`{"timeZone":"Europe/Berlin","locale":"en-GB","viewport":{"w":1280,"h":800}}`)
	applyClientEnv(client, [][]byte{envJSON}, ctx.comps)

	// Stored on the client immediately (read-loop side).
	if got := client.Env(); got.TimeZone != "Europe/Berlin" || got.Locale != "en-GB" || got.Viewport.W != 1280 || got.Viewport.H != 800 {
		t.Fatalf("client env not set correctly: %+v", got)
	}

	// OnConnect ran on the loop and seeded state from the env.
	deadline := time.Now().Add(2 * time.Second)
	for snapshotOnLoop(ci, func() string { return app.TZ }) != "Europe/Berlin" {
		if time.Now().After(deadline) {
			t.Fatal("OnConnect did not run / seed TZ")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if w := snapshotOnLoop(ci, func() int { return app.W }); w != 1280 {
		t.Errorf("OnConnect seeded viewport W=%d, want 1280", w)
	}
}

// An island that does not implement EnvAware gets no OnConnect event, but the
// client env is still recorded.
func TestApplyClientEnv_SkipsNonEnvAware(t *testing.T) {
	ci := makeCounterCI(&counterApp{})
	ci.EventCh = make(chan island.Event, 4)
	pool := &connPool{}
	wc := pool.add(nil)

	applyClientEnv(wc.client, [][]byte{[]byte(`{"timeZone":"UTC"}`)}, []*island.Info{ci})

	if wc.client.Env().TimeZone != "UTC" {
		t.Errorf("env should be recorded even with no EnvAware island")
	}
	select {
	case <-ci.EventCh:
		t.Error("a non-EnvAware island must not receive an OnConnect event")
	default:
	}
}

// Malformed or missing input is a safe no-op (no panic, env left zero).
func TestApplyClientEnv_MalformedIsSafeNoOp(t *testing.T) {
	pool := &connPool{}
	wc := pool.add(nil)

	applyClientEnv(wc.client, [][]byte{[]byte("not json")}, nil)
	if wc.client.Env() != (Env{}) {
		t.Errorf("malformed payload should leave env zero, got %+v", wc.client.Env())
	}
	applyClientEnv(wc.client, nil, nil)    // no args
	applyClientEnv(nil, [][]byte{{}}, nil) // nil client — must not panic
}

// Client.Env is the zero value until a payload is delivered.
func TestClient_EnvZeroByDefault(t *testing.T) {
	pool := &connPool{}
	wc := pool.add(nil)
	if wc.client.Env() != (Env{}) {
		t.Errorf("a fresh client should have a zero Env, got %+v", wc.client.Env())
	}
}
