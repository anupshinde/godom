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
	"github.com/anupshinde/godom/internal/env"
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

	// Wire up Refresh:
	// If fields were marked via MarkRefresh(), do a surgical update.
	// Otherwise, full refresh — re-send the entire tree.
	ci.RefreshFn = func() {
		ci.Mu.Lock()
		fields := ci.MarkedFields
		ci.MarkedFields = nil
		if len(fields) > 0 {
			patches := buildSurgicalPatches(ci, fields)
			if len(patches) > 0 {
				ci.Mu.Unlock()
				msg := render.EncodePatchMessage(patches)
				data, _ := proto.Marshal(msg)
				pool.broadcast(data)
				return
			}
			// No patches produced (fields had no bindings) — fall through to full rebuild.
		}
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
			if msgType != websocket.BinaryMessage || len(data) < 2 {
				continue
			}

			tag := data[0]
			payload := data[1:]

			switch tag {
			case 1: // NodeEvent (Layer 1)
				evt := &gproto.NodeEvent{}
				if err := proto.Unmarshal(payload, evt); err != nil {
					log.Printf("godom: node event unmarshal error: %v", err)
					continue
				}
				handleNodeEvent(ci, evt.NodeId, evt.Value, pool)

			case 2: // MethodCall (Layer 2)
				call := &gproto.MethodCall{}
				if err := proto.Unmarshal(payload, call); err != nil {
					log.Printf("godom: method call unmarshal error: %v", err)
					continue
				}
				handleMethodCall(ci, call, pool)
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
		fmt.Printf("godom running at\n%s\n", url)
		printQR(url)
	}

	if !cfg.NoBrowser {
		openBrowser(url)
	}

	return http.Serve(ln, mux)
}

// --- VDOM orchestration ---

// BuildInit builds the initial VDomMessage for a client connection.
// If a live tree exists (from prior connections or node events), it encodes
// that tree as-is so new clients see the current state.
func BuildInit(ci *component.Info) *gproto.VDomMessage {
	if ci.Tree == nil {
		ci.Tree = buildTree(ci)
	}

	msg, err := render.EncodeInitTreeMessage(ci.Tree)
	if err != nil {
		panic("EncodeInitTreeMessage: " + err.Error())
	}
	return msg
}

// BuildUpdate rebuilds the tree from templates, diffs against Tree, and
// returns a patch message. Returns nil if no changes.
func BuildUpdate(ci *component.Info) *gproto.VDomMessage {
	newTree := buildTree(ci)

	if ci.Tree == nil {
		ci.Tree = newTree
		msg, err := render.EncodeInitTreeMessage(newTree)
		if err != nil {
			panic("EncodeInitTreeMessage: " + err.Error())
		}
		return msg
	}

	patches := vdom.Diff(ci.Tree, newTree)
	vdom.MergeTree(ci.Tree, newTree)

	if len(patches) == 0 {
		return nil
	}

	return render.EncodePatchMessage(patches)
}

func buildTree(ci *component.Info) *vdom.ElementNode {
	if ci.IDCounter == nil {
		ci.IDCounter = &vdom.IDCounter{}
	}
	ctx := &vdom.ResolveContext{
		State: ci.Value,
		Vars:  make(map[string]any),
		IDs:   ci.IDCounter,
	}
	children := vdom.ResolveTree(ci.VDOMTemplates, ctx)
	root := &vdom.ElementNode{NodeBase: vdom.NodeBase{ID: ci.IDCounter.Next()}, Tag: "body", Children: children}
	vdom.ComputeDescendants(root)

	// Update bindings on every resolve (node IDs change on rebuild)
	if ctx.Bindings != nil {
		ci.Bindings = ctx.Bindings
	}

	return root
}

// buildSurgicalPatches reads the current field values and produces targeted
// patches for only the nodes bound to those fields. No tree rebuild, no diff.
func buildSurgicalPatches(ci *component.Info, fields []string) []vdom.Patch {
	var patches []vdom.Patch

	for _, field := range fields {
		bindings, ok := ci.Bindings[field]
		if !ok {
			continue
		}

		val := vdom.ResolveExpr(field, &vdom.ResolveContext{State: ci.Value})

		for _, b := range bindings {
			truthy := vdom.IsTruthy(val)
			strVal := fmt.Sprint(val)

			switch b.Kind {
			case "style":
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Styles: map[string]string{b.Prop: strVal}}},
				})
			case "prop":
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Props: map[string]any{b.Prop: strVal}}},
				})
			case "attr":
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Attrs: map[string]string{b.Prop: strVal}}},
				})
			case "text":
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchText,
					Data:   vdom.PatchTextData{Text: strVal},
				})
			case "show":
				display := ""
				if !truthy {
					display = "none"
				}
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Styles: map[string]string{"display": display}}},
				})
			case "hide":
				display := ""
				if truthy {
					display = "none"
				}
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Styles: map[string]string{"display": display}}},
				})
			case "class":
				// For class toggling, we need to add/remove the class name
				// This is simplified — a full implementation would track existing classes
				node := vdom.FindNodeByID(ci.Tree, b.NodeID)
				if el, ok := node.(*vdom.ElementNode); ok {
					existing, _ := el.Facts.Props["className"].(string)
					if truthy && !strings.Contains(existing, b.Prop) {
						if existing != "" {
							existing += " " + b.Prop
						} else {
							existing = b.Prop
						}
					} else if !truthy {
						existing = strings.TrimSpace(strings.ReplaceAll(existing, b.Prop, ""))
					}
					patches = append(patches, vdom.Patch{
						NodeID: b.NodeID,
						Type:   vdom.PatchFacts,
						Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Props: map[string]any{"className": existing}}},
					})
				}
			}

			// Also update the live tree so it stays in sync
			node := vdom.FindNodeByID(ci.Tree, b.NodeID)
			switch b.Kind {
			case "style":
				if el, ok := node.(*vdom.ElementNode); ok {
					if el.Facts.Styles == nil {
						el.Facts.Styles = make(map[string]string)
					}
					el.Facts.Styles[b.Prop] = strVal
				}
			case "prop":
				if el, ok := node.(*vdom.ElementNode); ok {
					if el.Facts.Props == nil {
						el.Facts.Props = make(map[string]any)
					}
					el.Facts.Props[b.Prop] = strVal
				}
			case "attr":
				if el, ok := node.(*vdom.ElementNode); ok {
					if el.Facts.Attrs == nil {
						el.Facts.Attrs = make(map[string]string)
					}
					el.Facts.Attrs[b.Prop] = strVal
				}
			case "show":
				if el, ok := node.(*vdom.ElementNode); ok {
					if el.Facts.Styles == nil {
						el.Facts.Styles = make(map[string]string)
					}
					if !truthy {
						el.Facts.Styles["display"] = "none"
					} else {
						delete(el.Facts.Styles, "display")
					}
				}
			case "hide":
				if el, ok := node.(*vdom.ElementNode); ok {
					if el.Facts.Styles == nil {
						el.Facts.Styles = make(map[string]string)
					}
					if truthy {
						el.Facts.Styles["display"] = "none"
					} else {
						delete(el.Facts.Styles, "display")
					}
				}
			case "text":
				if tn, ok := node.(*vdom.TextNode); ok {
					tn.Text = strVal
				}
			}
		}
	}

	return patches
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

// handleNodeEvent processes a Layer 1 node event: find the node in the live
// tree by ID, update its Props["value"], and broadcast a facts patch to all clients.
func handleNodeEvent(ci *component.Info, nodeID int32, value string, pool *connPool) {
	ci.Mu.Lock()

	node := vdom.FindNodeByID(ci.Tree, int(nodeID))
	if node == nil {
		log.Printf("godom: node %d not found in tree", nodeID)
		ci.Mu.Unlock()
		return
	}

	el, ok := node.(*vdom.ElementNode)
	if !ok {
		log.Printf("godom: node %d is not an element", nodeID)
		ci.Mu.Unlock()
		return
	}

	// Update the live tree in place
	if el.Facts.Props == nil {
		el.Facts.Props = make(map[string]any)
	}
	el.Facts.Props["value"] = value

	// Build a targeted facts patch — no need for a full diff
	patch := vdom.Patch{
		NodeID: int(nodeID),
		Type:   vdom.PatchFacts,
		Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Props: map[string]any{"value": value}}},
	}
	msg := render.EncodePatchMessage([]vdom.Patch{patch})
	ci.Mu.Unlock()

	data, _ := proto.Marshal(msg)
	pool.broadcast(data)
}

// handleMethodCall processes a Layer 2 method call: call the method on the
// component, then rebuild the tree and broadcast init to all clients.
func handleMethodCall(ci *component.Info, call *gproto.MethodCall, pool *connPool) {
	ci.Mu.Lock()

	// Convert protobuf [][]byte to []json.RawMessage
	args := make([]json.RawMessage, len(call.Args))
	for i, a := range call.Args {
		args[i] = json.RawMessage(a)
	}

	// Sync g-bind values from the live tree back to the struct.
	// Layer 1 keeps Tree props in sync with browser input values,
	// so we read from Tree and write to the struct before the method runs.
	if ci.Tree != nil && ci.Bindings != nil {
		for field, bindings := range ci.Bindings {
			for _, b := range bindings {
				if b.Kind != "bind" {
					continue
				}
				node := vdom.FindNodeByID(ci.Tree, b.NodeID)
				if node == nil {
					continue
				}
				el, ok := node.(*vdom.ElementNode)
				if !ok {
					continue
				}
				if el.Facts.Props == nil {
					continue
				}
				val, exists := el.Facts.Props["value"]
				if !exists {
					continue
				}
				raw, err := json.Marshal(val)
				if err != nil {
					continue
				}
				ci.SetField(field, json.RawMessage(raw))
			}
		}
	}

	if env.Debug {
		log.Printf("godom: method call %q args=%v", call.Method, args)
	}

	// Release the lock before calling the user method so that Refresh()
	// (which also acquires ci.Mu) can run without deadlocking.
	ci.Mu.Unlock()

	// Call the method, then refresh all clients.
	if err := ci.CallMethod(call.Method, args); err != nil {
		log.Printf("godom: method call %q error: %v", call.Method, err)
		return
	}

	ci.RefreshFn()
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
