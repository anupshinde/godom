package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/anupshinde/godom/internal/component"
	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/anupshinde/godom/internal/render"
	"github.com/anupshinde/godom/internal/vdom"
	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
	"google.golang.org/protobuf/proto"
)

// Config holds everything the server needs to run.
type Config struct {
	Comp      *component.Info
	Plugins   map[string][]string
	StaticFS  fs.FS
	Port      int
	Host      string
	NoAuth    bool
	Token     string
	NoBrowser bool
	Quiet     bool

	// Embedded JS scripts (passed from root via //go:embed)
	BridgeJS      string
	ProtobufMinJS string
	ProtocolJS    string
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Run starts the HTTP server, opens the browser, and blocks forever.
func Run(cfg Config) error {
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

	// Wire up Refresh: allow Go code to push state to all browsers.
	ci.RefreshFn = func() {
		ci.Mu.Lock()
		msg := BuildUpdate(ci)
		ci.Mu.Unlock()
		if msg != nil {
			data, _ := proto.Marshal(msg)
			pool.broadcast(data)
		}
	}

	mux := http.NewServeMux()

	// Build injected scripts: protobuf library, protocol definitions,
	// plugin registration + scripts, then bridge (last).
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

	// Serve static assets (CSS, images, etc.) from the embedded UI filesystem.
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
			log.Printf("godom: websocket upgrade error: %v", err)
			return
		}

		wc := pool.add(conn)

		if err := handleInit(wc, ci); err != nil {
			log.Printf("godom: failed to compute init: %v", err)
			pool.remove(wc)
			conn.Close()
			return
		}

		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("panic: %v", r)
				log.Printf("godom: %s", msg)
				reason := msg
				if len(reason) > 123 {
					reason = reason[:120] + "..."
				}
				closeMsg := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, reason)
				pool.broadcastClose(closeMsg)
				os.Exit(1)
			}
			pool.remove(wc)
			conn.Close()
		}()
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType != websocket.BinaryMessage {
				continue
			}

			env := &gproto.Envelope{}
			if err := proto.Unmarshal(data, env); err != nil {
				log.Printf("godom: envelope unmarshal error: %v", err)
				continue
			}

			wsMsg := &gproto.WSMessage{}
			if err := proto.Unmarshal(env.Msg, wsMsg); err != nil {
				log.Printf("godom: wsmessage unmarshal error: %v", err)
				continue
			}

			switch wsMsg.Type {
			case "call":
				handleCall(ci, wsMsg, env.Args, env.Value, pool)
			case "bind":
				handleBind(ci, wsMsg, env.Value, pool)
			}
		}
	})

	host := cfg.Host
	if host == "" {
		host = "localhost"
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, cfg.Port))
	if err != nil {
		return fmt.Errorf("godom: failed to listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	urlHost := host
	if host == "0.0.0.0" {
		if ip := localIP(); ip != "" {
			urlHost = ip
		} else {
			urlHost = "localhost"
		}
	}
	url := fmt.Sprintf("http://%s:%d", urlHost, port)
	if !cfg.NoAuth {
		url += "?token=" + token
	}
	if !cfg.Quiet {
		fmt.Printf("godom running at %s\n", url)
		printQR(url)
	}

	if !cfg.NoBrowser {
		openBrowser(url)
	}

	return http.Serve(ln, mux)
}

// --- VDOM orchestration ---

// BuildInit builds the initial VDomMessage for a new client connection.
func BuildInit(ci *component.Info) *gproto.VDomMessage {
	tree := buildTree(ci)
	ci.PrevTree = tree

	msg, err := render.EncodeInitTreeMessage(tree)
	if err != nil {
		// Should never happen with well-formed trees
		panic("EncodeInitTreeMessage: " + err.Error())
	}
	return msg
}

// BuildUpdate rebuilds the tree, diffs, and returns patches. Returns nil if no changes.
func BuildUpdate(ci *component.Info) *gproto.VDomMessage {
	newTree := buildTree(ci)

	if ci.PrevTree == nil {
		ci.PrevTree = newTree
		msg, err := render.EncodeInitTreeMessage(newTree)
		if err != nil {
			panic("EncodeInitTreeMessage: " + err.Error())
		}
		return msg
	}

	patches := vdom.Diff(ci.PrevTree, newTree)
	if len(patches) == 0 {
		return nil
	}

	msg := render.EncodePatchMessage(patches)
	ci.PrevTree = newTree
	return msg
}

func buildTree(ci *component.Info) *vdom.ElementNode {
	ctx := &vdom.ResolveContext{
		State: ci.Value,
		Vars:  make(map[string]any),
	}
	children := vdom.ResolveTree(ci.VDOMTemplates, ctx)
	root := &vdom.ElementNode{Tag: "body", Children: children}
	vdom.ComputeDescendants(root)
	return root
}

// --- WebSocket helpers ---

type wsConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func (wc *wsConn) writeBinary(data []byte) error {
	wc.wmu.Lock()
	defer wc.wmu.Unlock()
	return wc.conn.WriteMessage(websocket.BinaryMessage, data)
}

type connPool struct {
	mu    sync.RWMutex
	conns []*wsConn
}

func (p *connPool) add(conn *websocket.Conn) *wsConn {
	wc := &wsConn{conn: conn}
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
}

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

// --- Message handlers ---

func handleInit(wc *wsConn, ci *component.Info) error {
	ci.Mu.Lock()
	msg := BuildInit(ci)
	ci.Mu.Unlock()
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	return wc.writeBinary(data)
}

func handleCall(ci *component.Info, wsMsg *gproto.WSMessage, envArgs []float64, envValue []byte, pool *connPool) {
	ci.Mu.Lock()

	for _, a := range envArgs {
		jsonArg, _ := json.Marshal(a)
		wsMsg.Args = append(wsMsg.Args, jsonArg)
	}

	if len(envValue) > 0 {
		var extraArgs []json.RawMessage
		if err := json.Unmarshal(envValue, &extraArgs); err == nil {
			for _, arg := range extraArgs {
				wsMsg.Args = append(wsMsg.Args, arg)
			}
		}
	}

	jsonArgs := make([]json.RawMessage, len(wsMsg.Args))
	for i, a := range wsMsg.Args {
		jsonArgs[i] = json.RawMessage(a)
	}

	if err := ci.CallMethod(wsMsg.Method, jsonArgs); err != nil {
		log.Printf("godom: %v", err)
		ci.Mu.Unlock()
		return
	}

	msg := BuildUpdate(ci)
	ci.Mu.Unlock()
	if msg != nil {
		data, _ := proto.Marshal(msg)
		pool.broadcast(data)
	}
}

func handleBind(ci *component.Info, wsMsg *gproto.WSMessage, value []byte, pool *connPool) {
	ci.Mu.Lock()

	if err := ci.SetField(wsMsg.Field, json.RawMessage(value)); err != nil {
		log.Printf("godom: bind error: %v", err)
	}

	msg := BuildUpdate(ci)
	ci.Mu.Unlock()
	if msg != nil {
		data, _ := proto.Marshal(msg)
		pool.broadcast(data)
	}
}

// --- Helpers ---

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("godom: failed to generate auth token: %v", err)
	}
	return hex.EncodeToString(b)
}

func checkAuth(token string, w http.ResponseWriter, r *http.Request) bool {
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

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func printQR(url string) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return
	}
	bitmap := qr.Bitmap()
	n := len(bitmap)
	for y := 0; y < n; y += 2 {
		for x := 0; x < n; x++ {
			top := bitmap[y][x]
			bot := false
			if y+1 < n {
				bot = bitmap[y+1][x]
			}
			switch {
			case top && bot:
				fmt.Print("█")
			case top:
				fmt.Print("▀")
			case bot:
				fmt.Print("▄")
			default:
				fmt.Print(" ")
			}
		}
		fmt.Println()
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
