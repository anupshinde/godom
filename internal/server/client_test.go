package server

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// add() must give every connection a Client with a back-reference and a
// non-empty, unique, per-socket id. "Per-socket" means a reconnecting tab gets a
// brand-new id — ids are never reused, so stale references can't alias a new tab.
func TestConnPool_AddAssignsUniquePerSocketClientIDs(t *testing.T) {
	pool := &connPool{}
	wc1 := pool.add(nil)
	wc2 := pool.add(nil)
	wc3 := pool.add(nil)

	all := []*wsConn{wc1, wc2, wc3}
	seen := map[string]bool{}
	for i, wc := range all {
		if wc.client == nil {
			t.Fatalf("conn %d: nil client", i)
		}
		if wc.client.wc != wc {
			t.Errorf("conn %d: client.wc back-reference is wrong", i)
		}
		if wc.client.ID() == "" {
			t.Errorf("conn %d: empty client id", i)
		}
		if seen[wc.client.ID()] {
			t.Errorf("conn %d: duplicate id %q", i, wc.client.ID())
		}
		seen[wc.client.ID()] = true
	}

	// Remove one and re-add: the new connection must NOT reuse the old id.
	old := wc1.client.ID()
	pool.remove(wc1)
	wc4 := pool.add(nil)
	if wc4.client.ID() == old {
		t.Errorf("re-added connection reused id %q; client identity must be per-socket", old)
	}
}

// Clients() must mirror the live roster across add/remove and must hand back a
// fresh copy (mutating the result must not corrupt the pool).
func TestConnPool_ClientsRosterReflectsAddRemove(t *testing.T) {
	pool := &connPool{}
	if got := pool.Clients(); len(got) != 0 {
		t.Fatalf("empty pool: expected 0 clients, got %d", len(got))
	}

	wc1 := pool.add(nil)
	wc2 := pool.add(nil)

	got := pool.Clients()
	if len(got) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(got))
	}

	// The snapshot is a copy: mutating it must not change the pool's roster.
	got[0] = nil
	if len(pool.Clients()) != 2 {
		t.Errorf("Clients() must return a copy, not the live slice")
	}

	// Roster contains exactly the two live clients.
	want := map[*Client]bool{wc1.client: true, wc2.client: true}
	for _, c := range pool.Clients() {
		if !want[c] {
			t.Errorf("unexpected client %q in roster", c.ID())
		}
		delete(want, c)
	}
	if len(want) != 0 {
		t.Errorf("roster missing %d expected client(s)", len(want))
	}

	pool.remove(wc1)
	got = pool.Clients()
	if len(got) != 1 || got[0] != wc2.client {
		t.Errorf("after removing wc1, roster should be exactly [wc2]; got %d client(s)", len(got))
	}
}

// sendTo must be genuinely targeted: the addressed client receives the frame and
// the other connected client does NOT. This is the property that distinguishes
// send-to-one from broadcast, so it's tested with real sockets.
func TestConnPool_SendToReachesExactlyOneClient(t *testing.T) {
	var mu sync.Mutex
	var serverConns []*websocket.Conn
	srv := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	c0, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c0.Close()
	c1, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()

	// Wait until the server has accepted both connections.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(serverConns)
		mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server accepted %d/2 connections", n)
		}
		time.Sleep(10 * time.Millisecond)
	}

	pool := &connPool{}
	mu.Lock()
	wc0 := pool.add(serverConns[0])
	pool.add(serverConns[1])
	mu.Unlock()

	if err := pool.sendTo(wc0.client, []byte{0xAA}); err != nil {
		t.Fatalf("sendTo: %v", err)
	}

	// Targeted client receives exactly the sent frame.
	c0.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := c0.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || len(data) != 1 || data[0] != 0xAA {
		t.Fatalf("targeted client should have received [0xAA]: mt=%d data=%v err=%v", mt, data, err)
	}

	// The other client must receive nothing (read times out).
	c1.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := c1.ReadMessage(); err == nil {
		t.Error("non-targeted client received a frame from a send-to-one")
	}
}
