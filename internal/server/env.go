package server

import (
	"encoding/json"

	"github.com/anupshinde/godom/internal/island"
)

// clientEnvMethod is the reserved godom.call method name the bridge uses to
// deliver a connection's browser environment right after connect. It rides the
// existing BROWSER_METHOD channel, so no new wire message is needed.
const clientEnvMethod = "__godom_env__"

// Viewport is the browser viewport size in CSS pixels.
type Viewport struct {
	W int
	H int
}

// Env is the connection-derived browser environment — the data Go cannot derive
// on its own. It is captured once, just after the socket connects.
type Env struct {
	TimeZone string // IANA timezone, e.g. "Europe/Berlin"
	Locale   string // BCP-47 locale, e.g. "en-GB"
	Viewport Viewport
}

// EnvAware is implemented by an island that wants to seed state from a new
// connection's environment. OnConnect runs as an ordinary event on the island's
// loop (serialized with handlers and refreshes), once per connecting client.
//
// Note: an island has one shared VDOM replicated to every client, so seeding a
// shared field from c.Env() is last-writer-wins across clients — correct for the
// single-environment case (one local user, possibly multiple same-machine
// windows). For divergent per-client environments, scope by page or engine.
type EnvAware interface {
	OnConnect(c *Client)
}

// applyClientEnv decodes the env payload, stores it on the client, and dispatches
// OnConnect to every EnvAware island. Runs on the connection's read-loop
// goroutine; the per-island OnConnect is marshaled onto each island's event loop.
func applyClientEnv(client *Client, args [][]byte, comps []*island.Info) {
	if client == nil || len(args) == 0 {
		return
	}
	var p struct {
		TimeZone string `json:"timeZone"`
		Locale   string `json:"locale"`
		Viewport struct {
			W int `json:"w"`
			H int `json:"h"`
		} `json:"viewport"`
	}
	if err := json.Unmarshal(args[0], &p); err != nil {
		return
	}
	client.setEnv(Env{
		TimeZone: p.TimeZone,
		Locale:   p.Locale,
		Viewport: Viewport{W: p.Viewport.W, H: p.Viewport.H},
	})

	for _, ci := range comps {
		ea, ok := ci.Value.Interface().(EnvAware)
		if !ok || ci.EventCh == nil {
			continue
		}
		ci, ea := ci, ea
		// Unfenced apply: runs OnConnect on the island loop, then the ApplyKind
		// handler refreshes so the seeded state is rendered.
		ci.EventCh <- island.Event{Kind: island.ApplyKind, Apply: func() { ea.OnConnect(client) }}
	}
}
