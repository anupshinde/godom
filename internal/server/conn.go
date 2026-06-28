package server

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client is an addressable handle to one connected replica — a single browser
// tab / WebSocket. It is the foundation that per-connection features build on:
// connection-scoped environment (Env), targeted JS, and roster/governance.
//
// A Client is per-socket: a reconnecting tab is a *new* Client, not a revived
// one. Its ID is stable for the life of the connection and unique within the
// process. Application code obtains Clients from the engine roster; it never
// constructs them.
type Client struct {
	id    string
	wc    *wsConn
	envMu sync.RWMutex
	env   Env

	// Targeted-call state (§4). Pending callbacks are keyed by a per-client,
	// negative call id (the island broadcast ExecJS path uses positive ids, so
	// the read loop can route replies by sign without collision).
	callMu  sync.Mutex
	callID  int32
	pending map[int32]func(result []byte, errMsg string)

	// Per-client module capabilities (§4). A capability is a runtime, per-tab
	// fact — the module's JS loaded and its prerequisites are present — declared
	// by the bridge (godom.declareCapability) and re-advertised on reconnect.
	capMu sync.RWMutex
	caps  map[string]bool
}

// Has reports whether this client has advertised the named module capability.
// Safe to call from any goroutine.
func (c *Client) Has(capability string) bool {
	c.capMu.RLock()
	defer c.capMu.RUnlock()
	return c.caps[capability]
}

func (c *Client) addCapability(name string) {
	c.capMu.Lock()
	if c.caps == nil {
		c.caps = make(map[string]bool)
	}
	c.caps[name] = true
	c.capMu.Unlock()
}

// ID returns the connection's stable, process-unique identifier.
func (c *Client) ID() string { return c.id }

// Env returns a snapshot of the connection's browser environment (timezone,
// locale, viewport). It is zero until the bridge delivers it just after connect;
// safe to read from any goroutine.
func (c *Client) Env() Env {
	c.envMu.RLock()
	defer c.envMu.RUnlock()
	return c.env
}

func (c *Client) setEnv(e Env) {
	c.envMu.Lock()
	c.env = e
	c.envMu.Unlock()
}

// send writes a single binary frame to this client's connection only
// (send-to-one), in contrast to connPool.broadcast which writes to all.
func (c *Client) send(data []byte) error { return c.wc.writeBinary(data) }

// wsConn wraps a WebSocket connection with a write mutex for safe
// serialization of binary messages from concurrent goroutines.
type wsConn struct {
	conn   *websocket.Conn
	wmu    sync.Mutex
	client *Client
}

func (wc *wsConn) writeBinary(data []byte) error {
	wc.wmu.Lock()
	defer wc.wmu.Unlock()
	return wc.conn.WriteMessage(websocket.BinaryMessage, data)
}

// connPool manages active WebSocket connections and provides thread-safe
// broadcast to all connected clients and send-to-one to a single client.
type connPool struct {
	mu     sync.RWMutex
	conns  []*wsConn
	nextID atomic.Uint64
}

func (p *connPool) add(conn *websocket.Conn) *wsConn {
	wc := &wsConn{conn: conn}
	// Each connection gets a fresh, process-unique, monotonic id. A reconnecting
	// tab is therefore a new Client with a new id — Client identity is per-socket.
	wc.client = &Client{id: "c" + strconv.FormatUint(p.nextID.Add(1), 10), wc: wc}
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
	// Fail any in-flight targeted calls so a blocked Call unblocks instead of
	// hanging when the tab goes away.
	if wc.client != nil {
		wc.client.cancelPending("client disconnected")
	}
}

// Clients returns a snapshot of the currently connected clients. The slice is a
// fresh copy; the *Client values are the stable per-connection handles.
func (p *connPool) Clients() []*Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Client, len(p.conns))
	for i, wc := range p.conns {
		out[i] = wc.client
	}
	return out
}

// sendTo writes a binary frame to exactly one client (send-to-one).
func (p *connPool) sendTo(c *Client, data []byte) error { return c.send(data) }

func (p *connPool) broadcast(data []byte) {
	p.mu.RLock()
	snapshot := make([]*wsConn, len(p.conns))
	copy(snapshot, p.conns)
	p.mu.RUnlock()

	for _, wc := range snapshot {
		wc.writeBinary(data)
	}
}

func (p *connPool) broadcastClose(closeMsg []byte) {
	p.mu.RLock()
	snapshot := make([]*wsConn, len(p.conns))
	copy(snapshot, p.conns)
	p.mu.RUnlock()

	for _, wc := range snapshot {
		wc.conn.WriteMessage(websocket.CloseMessage, closeMsg)
		wc.conn.Close()
	}
}

// ClientSource exposes the live connection roster. The server's connPool
// implements it, and the public Engine reads through it for Engine.Clients().
type ClientSource interface {
	Clients() []*Client
}
