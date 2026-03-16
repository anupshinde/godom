package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// termConn wraps a WebSocket connection with a write mutex.
type termConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (tc *termConn) writeBinary(data []byte) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.conn.WriteMessage(websocket.BinaryMessage, data)
}

// termSession manages a PTY and its connected WebSocket clients.
// When the shell exits, it automatically respawns a new one.
type termSession struct {
	shell string
	mu    sync.Mutex
	ptmx  *os.File
	conns []*termConn
}

func newTermSession() *termSession {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	ts := &termSession{shell: shell}
	ts.spawn()
	return ts
}

// spawn starts a new shell process with a PTY and begins
// broadcasting its output. Called on startup and after shell exit.
func (ts *termSession) spawn() {
	cmd := exec.Command(ts.shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("terminal: failed to start PTY: %v", err)
	}

	ts.mu.Lock()
	ts.ptmx = ptmx
	ts.mu.Unlock()

	// Read PTY output and broadcast to all connected browsers.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				// Shell exited. Notify browsers and respawn.
				msg := []byte("\r\n\x1b[1;33m[shell exited — this is a browser-based terminal powered by godom, starting new session...]\x1b[0m\r\n\x1b[0;33m[to exit the terminal, close this tab or terminate the godom process]\x1b[0m\r\n\r\n")
				ts.mu.Lock()
				for _, tc := range ts.conns {
					tc.writeBinary(msg)
				}
				ts.mu.Unlock()

				ptmx.Close()
				ts.spawn()
				return
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			ts.mu.Lock()
			for _, tc := range ts.conns {
				tc.writeBinary(data)
			}
			ts.mu.Unlock()
		}
	}()
}

func (ts *termSession) addConn(tc *termConn) {
	ts.mu.Lock()
	ts.conns = append(ts.conns, tc)
	ts.mu.Unlock()
}

func (ts *termSession) removeConn(tc *termConn) {
	ts.mu.Lock()
	for i, c := range ts.conns {
		if c == tc {
			ts.conns = append(ts.conns[:i], ts.conns[i+1:]...)
			break
		}
	}
	ts.mu.Unlock()
}

func (ts *termSession) writeToPTY(data []byte) {
	ts.mu.Lock()
	ptmx := ts.ptmx
	ts.mu.Unlock()
	ptmx.Write(data)
}

func (ts *termSession) resize(cols, rows int) {
	ts.mu.Lock()
	ptmx := ts.ptmx
	ts.mu.Unlock()
	pty.Setsize(ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// startTerminalServer spawns a shell with a PTY and serves a WebSocket
// that pipes raw I/O between the browser and the PTY.
// When the shell exits, a new session is started automatically.
// Returns the port the server is listening on.
func startTerminalServer(authToken string) int {
	ts := newTermSession()

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("terminal: failed to listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/terminal", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != authToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("terminal: websocket upgrade: %v", err)
			return
		}

		tc := &termConn{conn: conn}
		ts.addConn(tc)

		defer func() {
			ts.removeConn(tc)
			conn.Close()
		}()

		// Read from WebSocket, write to PTY.
		// Binary messages = terminal input.
		// Text messages = control commands (resize).
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				var resize struct {
					Cols int `json:"cols"`
					Rows int `json:"rows"`
				}
				if json.Unmarshal(data, &resize) == nil && resize.Cols > 0 && resize.Rows > 0 {
					ts.resize(resize.Cols, resize.Rows)
				}
			} else {
				ts.writeToPTY(data)
			}
		}
	})

	go http.Serve(ln, mux)
	return port
}
