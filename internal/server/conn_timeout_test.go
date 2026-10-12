package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWriteBinaryTimesOutOnStalledClient: a client that never reads must not
// block writeBinary (and so broadcast / the island event loops) forever. Once its
// TCP buffers fill, a write fails within writeTimeout and the connection is
// closed so the read loop can remove it.
func TestWriteBinaryTimesOutOnStalledClient(t *testing.T) {
	old := writeTimeout
	writeTimeout = 200 * time.Millisecond
	defer func() { writeTimeout = old }()

	srvConn := make(chan *websocket.Conn, 1)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		srvConn <- c
	}))
	defer srv.Close()

	// The client connects and then never reads.
	cli, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()

	wc := &wsConn{conn: <-srvConn}
	frame := make([]byte, 1<<20) // 1 MiB frames fill the socket buffers quickly

	done := make(chan error, 1)
	go func() {
		for i := 0; i < 1000; i++ {
			if err := wc.writeBinary(frame); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a write to fail against a client that never reads")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writeBinary blocked on a stalled client (no write deadline)")
	}

	// The connection was closed: further writes fail immediately.
	start := time.Now()
	if err := wc.writeBinary([]byte("x")); err == nil {
		t.Fatal("expected write on closed connection to fail")
	}
	if time.Since(start) > time.Second {
		t.Fatal("write on a closed connection should fail fast")
	}
}
