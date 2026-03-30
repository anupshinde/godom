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
	"reflect"
	"runtime"
	"strings"

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
	Comps     []*component.Info
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

// serverCtx holds shared state used by event processors and handlers.
// Avoids threading pool, sm, lookup, and comps through every function.
type serverCtx struct {
	pool   *connPool
	sm     *sharedPtrMaps
	lookup *nodeLookup
	comps  []*component.Info
}

// sharedPtrMaps holds the pointer-sharing relationships between components.
// Built once at startup, used to propagate refreshes to sibling components
// that share embedded pointer fields (e.g. *CounterState).
type sharedPtrMaps struct {
	ptrToCompIdx map[uintptr][]int // pointer address → component indices sharing it
	compIdxToPtr map[int][]uintptr // component index → pointer addresses it holds
	comps        []*component.Info
	pool         *connPool
}

// buildSharedPtrMaps walks all component structs to find embedded pointer fields
// and groups components that share the same pointer address.
func buildSharedPtrMaps(comps []*component.Info) *sharedPtrMaps {
	sm := &sharedPtrMaps{
		ptrToCompIdx: make(map[uintptr][]int),
		compIdxToPtr: make(map[int][]uintptr),
		comps:        comps,
	}
	for idx, ci := range comps {
		v := ci.Value.Elem() // the struct value
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.Anonymous || f.Type.Kind() != reflect.Ptr {
				continue
			}
			fv := v.Field(i)
			if fv.IsNil() {
				continue
			}
			ptr := fv.Pointer()
			sm.ptrToCompIdx[ptr] = append(sm.ptrToCompIdx[ptr], idx)
			sm.compIdxToPtr[idx] = append(sm.compIdxToPtr[idx], ptr)
		}
	}
	// Remove entries where only one component holds the pointer (no sharing).
	for ptr, idxs := range sm.ptrToCompIdx {
		if len(idxs) <= 1 {
			delete(sm.ptrToCompIdx, ptr)
			for _, idx := range idxs {
				sm.compIdxToPtr[idx] = removePtr(sm.compIdxToPtr[idx], ptr)
				if len(sm.compIdxToPtr[idx]) == 0 {
					delete(sm.compIdxToPtr, idx)
				}
			}
		}
	}
	return sm
}

func removePtr(ptrs []uintptr, target uintptr) []uintptr {
	result := ptrs[:0]
	for _, p := range ptrs {
		if p != target {
			result = append(result, p)
		}
	}
	return result
}

// refreshSharedComponents triggers surgical refresh on all other components
// that share an embedded pointer with the given component, using the changed
// field names extracted from the original component's patches.
func (sm *sharedPtrMaps) refreshSharedComponents(compIdx int, changedFields []string) {
	if sm == nil || len(changedFields) == 0 {
		return
	}
	ptrs := sm.compIdxToPtr[compIdx]
	if len(ptrs) == 0 {
		return
	}
	seen := map[int]bool{compIdx: true} // skip self
	for _, ptr := range ptrs {
		for _, sibIdx := range sm.ptrToCompIdx[ptr] {
			if seen[sibIdx] {
				continue
			}
			seen[sibIdx] = true
			sib := sm.comps[sibIdx]
			sib.AddMarkedFields(changedFields...)
			sib.RefreshFn()
		}
	}
}

// Run starts the HTTP server, opens the browser, and blocks forever.
func Run(cfg Config) error {
	pool := &connPool{}
	var token string
	if !cfg.NoAuth {
		if cfg.Token != "" {
			token = cfg.Token
		} else {
			token = generateToken()
		}
	}

	// All components share a single IDCounter so node IDs are globally
	// unique across the bridge's nodeMap.
	sharedIDCounter := &vdom.IDCounter{}
	for _, ci := range cfg.Comps {
		ci.IDCounter = sharedIDCounter
	}

	// Wire up Refresh for each component.
	for _, ci := range cfg.Comps {
		ci := ci // capture for closure
		wireRefresh(ci)
	}

	// Build shared pointer maps for auto-refreshing sibling components.
	sm := buildSharedPtrMaps(cfg.Comps)
	sm.pool = pool

	ctx := &serverCtx{
		pool:  pool,
		sm:    sm,
		lookup: newNodeLookup(),
		comps: cfg.Comps,
	}

	// Start event queue processor for each component.
	for idx, ci := range cfg.Comps {
		ci.EventCh = make(chan component.Event, 64)
		idx, ci := idx, ci // capture for closure
		go ctx.processEvents(ci, idx)
	}

	mux := http.NewServeMux()

	// Build the JS bundle once: protobuf, protocol, plugins, bridge.
	var bundleJS string
	bundleJS += cfg.ProtobufMinJS + "\n" + cfg.ProtocolJS + "\n"
	if len(cfg.Plugins) > 0 {
		bundleJS += "var godom=window[window.GODOM_NS||'godom']=window[window.GODOM_NS||'godom']||{};godom._plugins=godom._plugins||{};godom.register=function(n,h){godom._plugins[n]=h};\n"
		for _, pluginScripts := range cfg.Plugins {
			for _, js := range pluginScripts {
				bundleJS += js + "\n"
			}
		}
	}
	bundleJS += cfg.BridgeJS

	// Serve as external script at /godom.js.
	mux.HandleFunc("/godom.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, bundleJS)
	})

	// The root component (first in Comps, mounted via Mount) provides the page HTML.
	pageHTML := strings.Replace(cfg.Comps[0].HTMLBody, "</body>", "<script src=\"/godom.js\"></script>\n</body>", 1)

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

		// Send init for each component in mount order (root first, children after).
		for _, ci := range cfg.Comps {
			if err := handleInit(wc, ci, ci.SlotName); err != nil {
				log.Printf("godom: failed to compute init for slot %q: %v", ci.SlotName, err)
				pool.remove(wc)
				conn.Close()
				return
			}
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
				if ci := findComponent(int(evt.NodeId), ctx.comps, ctx.lookup); ci != nil {
					e := component.Event{Kind: component.NodeEventKind, NodeID: evt.NodeId, Value: evt.Value}
					if shouldEnqueue(e) {
						ci.EventCh <- e
					}
				}

			case 2: // MethodCall (Layer 2)
				call := &gproto.MethodCall{}
				if err := proto.Unmarshal(payload, call); err != nil {
					log.Printf("godom: method call unmarshal error: %v", err)
					continue
				}
				if ci := findComponent(int(call.NodeId), ctx.comps, ctx.lookup); ci != nil {
					e := component.Event{Kind: component.MethodCallKind, Call: call}
					if shouldEnqueue(e) {
						ci.EventCh <- e
					}
				}
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

	err = http.Serve(ln, mux)

	// Clean shutdown: close event channels so processor goroutines exit.
	for _, ci := range cfg.Comps {
		if ci.EventCh != nil {
			close(ci.EventCh)
		}
	}

	return err
}

// wireRefresh sets up the RefreshFn for a mounted component.
// RefreshFn sends a RefreshKind event to the component's event queue,
// ensuring all refreshes are serialized through the processor goroutine.
// The actual refresh logic lives in executeRefresh.
func wireRefresh(ci *component.Info) {
	ci.RefreshFn = func() {
		if ci.EventCh != nil {
			ci.EventCh <- component.Event{Kind: component.RefreshKind}
		}
	}
}

// executeRefresh performs the actual refresh: drain marked fields for surgical
// patches, or fall back to a full BuildUpdate + diff. Called only from
// processEvents to ensure serialized access — no lock needed.
func (s *serverCtx) executeRefresh(ci *component.Info) {
	fields := ci.DrainMarkedFields()
	if len(fields) > 0 {
		patches := s.buildSurgicalPatches(ci, fields)
		if len(patches) > 0 {
			msg := render.EncodePatchMessage(patches)
			msg.TargetName = ci.SlotName
			data, _ := proto.Marshal(msg)
			s.pool.broadcast(data)
			return
		}
	}
	msg, changedFields := BuildUpdate(ci)
	s.lookup.evictRemoved()
	ci.LastChangedFields = changedFields
	if msg != nil {
		msg.TargetName = ci.SlotName
		data, _ := proto.Marshal(msg)
		s.pool.broadcast(data)
	}
}

// shouldEnqueue decides whether an event should be placed on the channel.
// Returns true to enqueue, false to drop. Currently allows all events.
func shouldEnqueue(_ component.Event) bool {
	return true
}

// shouldProcess decides whether an event should be processed after being
// dequeued. Returns true to process, false to skip. Currently allows all events.
func shouldProcess(_ component.Event) bool {
	return true
}

// processEvents is the single goroutine per component that processes events
// sequentially from the component's event queue. This eliminates race
// conditions between concurrent event sources (multiple WS connections,
// background goroutines).
func (s *serverCtx) processEvents(ci *component.Info, compIdx int) {
	for evt := range ci.EventCh {
		if !shouldProcess(evt) {
			continue
		}
		switch evt.Kind {
		case component.NodeEventKind:
			s.handleNodeEvent(ci, compIdx, evt.NodeID, evt.Value)
		case component.MethodCallKind:
			s.handleMethodCall(ci, compIdx, evt.Call)
		case component.RefreshKind:
			s.executeRefresh(ci)
		}
	}
}



// --- VDOM orchestration ---

// BuildInit builds the initial VDomMessage for a client connection.
// On first call (no tree yet), it builds from scratch.
// On subsequent calls (reconnect), it re-resolves from the live struct and
// merges into the existing tree to preserve node IDs for other connections.
func BuildInit(ci *component.Info) *gproto.VDomMessage {
	if ci.Tree == nil {
		// First connection: build from scratch. We must not rebuild ci.Tree
		// from scratch after this point — node IDs are baked into the bridge's
		// nodeMap for all connected browsers. A fresh tree would assign new IDs,
		// causing subsequent patches to reference IDs that existing connections
		// don't recognize.
		ci.Tree = buildTree(ci)
	} else {
		// Re-resolve from live struct to pick up state changes that
		// didn't trigger a BuildUpdate for this component (e.g. shared state).
		// Use BuildUpdate (not just MergeTree) so that Bindings, InputBindings,
		// and NodeStableIDs are remapped correctly after the merge.
		_, _ = BuildUpdate(ci)
	}

	msg, err := render.EncodeInitTreeMessage(ci.Tree)
	if err != nil {
		panic("EncodeInitTreeMessage: " + err.Error())
	}
	return msg
}

// BuildUpdate rebuilds the tree from templates, diffs against Tree, and
// returns a patch message. Returns nil if no changes.
func BuildUpdate(ci *component.Info) (*gproto.VDomMessage, []string) {
	// Keep the IDCounter alive across renders so IDs only increase.
	// Resetting to zero would cause new subtrees (from g-if transitions)
	// to get IDs that collide with existing nodes elsewhere in the tree,
	// corrupting the bridge's nodeMap.
	if ci.IDCounter == nil {
		ci.IDCounter = &vdom.IDCounter{}
	}
	newTree := buildTree(ci)

	if ci.Tree == nil {
		ci.Tree = newTree
		msg, err := render.EncodeInitTreeMessage(newTree)
		if err != nil {
			panic("EncodeInitTreeMessage: " + err.Error())
		}
		return msg, nil
	}

	patches := vdom.Diff(ci.Tree, newTree)

	// MergeTree keeps dst's node IDs at structurally matching positions.
	// It returns a map from src (new) IDs → dst (old) IDs so we can remap
	// bindings and NodeStableIDs that reference new-tree IDs.
	remap := vdom.MergeTree(ci.Tree, newTree)

	// Remap bindings: replace new-tree IDs with merged-tree IDs.
	if len(remap) > 0 {
		for field, bindings := range ci.Bindings {
			for i, b := range bindings {
				if mergedID, ok := remap[b.NodeID]; ok {
					ci.Bindings[field][i].NodeID = mergedID
				}
			}
		}
		if ci.NodeStableIDs != nil {
			remapped := make(map[int]string, len(ci.NodeStableIDs))
			for nodeID, key := range ci.NodeStableIDs {
				if mergedID, ok := remap[nodeID]; ok {
					remapped[mergedID] = key
				} else {
					remapped[nodeID] = key
				}
			}
			ci.NodeStableIDs = remapped
		}
		if ci.InputBindings != nil {
			remappedIB := make(map[int]vdom.InputBinding, len(ci.InputBindings))
			for nodeID, ib := range ci.InputBindings {
				if mergedID, ok := remap[nodeID]; ok {
					remappedIB[mergedID] = ib
				} else {
					remappedIB[nodeID] = ib
				}
			}
			ci.InputBindings = remappedIB
		}
	}

	if len(patches) == 0 {
		return nil, nil
	}

	return render.EncodePatchMessage(patches), changedFieldsFromPatches(patches, ci.Bindings)
}

// changedFieldsFromPatches reverse-looks up patch NodeIDs in the bindings map
// to determine which field names were affected by the update.
func changedFieldsFromPatches(patches []vdom.Patch, bindings map[string][]vdom.Binding) []string {
	if len(bindings) == 0 {
		return nil
	}
	changedIDs := make(map[int]bool, len(patches))
	for _, p := range patches {
		changedIDs[p.NodeID] = true
	}
	var fields []string
	for field, bs := range bindings {
		for _, b := range bs {
			if changedIDs[b.NodeID] {
				fields = append(fields, field)
				break
			}
		}
	}
	return fields
}

func buildTree(ci *component.Info) *vdom.ElementNode {
	if ci.IDCounter == nil {
		ci.IDCounter = &vdom.IDCounter{}
	}
	nodeStableIDs := make(map[int]string)
	ctx := &vdom.ResolveContext{
		State:         ci.Value,
		Vars:          make(map[string]any),
		IDs:           ci.IDCounter,
		UnboundValues: ci.UnboundValues,
		NodeStableIDs: nodeStableIDs,
	}
	children := vdom.ResolveTree(ci.VDOMTemplates, ctx)
	root := &vdom.ElementNode{NodeBase: vdom.NodeBase{ID: ci.IDCounter.Next()}, Tag: "body", Children: children}
	vdom.ComputeDescendants(root)

	// Update bindings on every resolve (node IDs change on rebuild)
	if ctx.Bindings != nil {
		ci.Bindings = ctx.Bindings
	}
	ci.InputBindings = ctx.InputBindings
	ci.NodeStableIDs = nodeStableIDs

	return root
}

// buildSurgicalPatches reads the current field values and produces targeted
// patches for only the nodes bound to those fields. No tree rebuild, no diff.
func (s *serverCtx) buildSurgicalPatches(ci *component.Info, fields []string) []vdom.Patch {
	var patches []vdom.Patch

	for _, field := range fields {
		bindings, ok := ci.Bindings[field]
		if !ok {
			continue
		}

		for _, b := range bindings {
			expr := b.Expr
			if expr == "" {
				expr = field
			}
			val := vdom.ResolveExpr(expr, &vdom.ResolveContext{State: ci.Value})
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
				var propVal any = strVal
				if b.Prop == "checked" {
					propVal = truthy
				}
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Props: map[string]any{b.Prop: propVal}}},
				})
			case "attr":
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Attrs: map[string]string{b.Prop: strVal}}},
				})
			case "bind":
				patches = append(patches, vdom.Patch{
					NodeID: b.NodeID,
					Type:   vdom.PatchFacts,
					Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Props: map[string]any{"value": strVal}}},
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
				node := findNode(b.NodeID, ci, s.lookup)
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
			node := findNode(b.NodeID, ci, s.lookup)
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
					var pv any = strVal
					if b.Prop == "checked" {
						pv = truthy
					}
					el.Facts.Props[b.Prop] = pv
				}
			case "bind":
				if el, ok := node.(*vdom.ElementNode); ok {
					if el.Facts.Props == nil {
						el.Facts.Props = make(map[string]any)
					}
					el.Facts.Props["value"] = strVal
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

// --- Message handlers ---

// handleInit builds and sends the init tree to a specific connection.
// Not routed through the event queue because init must be synchronous —
// the browser needs the root DOM before child components can render into
// their g-component targets (mount order). Even if we sent this through
// the channel, we'd need to block until it completes — so a lock is
// the simpler and more direct way to achieve the same thing. Uses ci.Mu
// to prevent races with the processor goroutine that writes ci.Tree.
func handleInit(wc *wsConn, ci *component.Info, targetName string) error {
	ci.Mu.Lock()
	msg := BuildInit(ci)
	msg.TargetName = targetName
	ci.Mu.Unlock()
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	return wc.writeBinary(data)
}

// handleNodeEvent processes a Layer 1 node event: find the node in the live
// tree by ID, update its Props["value"] (or Props["checked"] for checkboxes),
// and broadcast a facts patch to all clients.
func (s *serverCtx) handleNodeEvent(ci *component.Info, compIdx int, nodeID int32, value string) {
	// No lock needed — called only from processEvents (serialized).

	node := findNode(int(nodeID), ci, s.lookup)
	if node == nil {
		log.Printf("godom: node %d not found in tree", nodeID)
		return
	}

	el, ok := node.(*vdom.ElementNode)
	if !ok {
		log.Printf("godom: node %d is not an element", nodeID)
		return
	}

	// Checkboxes send "true"/"false" and use Props["checked"] (bool)
	isCheckbox := el.Tag == "input" && el.Facts.Attrs["type"] == "checkbox"
	propKey := "value"
	var propVal any = value
	if isCheckbox {
		propKey = "checked"
		propVal = value == "true"
	}

	// If this node has an input binding (g-bind, g-value, g-checked),
	// sync the value to the struct and trigger a full refresh so all
	// nodes referencing that field get updated (including computed methods).
	// Don't update ci.Tree directly — BuildUpdate will rebuild from the
	// struct, diff against the old tree, and produce proper patches.
	if ib, ok := ci.InputBindings[int(nodeID)]; ok {
		raw, err := json.Marshal(propVal)
		if err == nil {
			setPath := ib.Field
			if ib.Expr != "" {
				setPath = ib.Expr
			}
			ci.SetField(setPath, json.RawMessage(raw))
		}
		s.executeRefresh(ci)
		changedFields := ci.LastChangedFields
		ci.LastChangedFields = nil
		s.sm.refreshSharedComponents(compIdx, changedFields)
		return
	}

	// Unbound input: update tree directly and broadcast targeted patch
	if el.Facts.Props == nil {
		el.Facts.Props = make(map[string]any)
	}
	el.Facts.Props[propKey] = propVal

	// Store unbound input value so it survives tree rebuilds
	if stableKey, ok := ci.NodeStableIDs[int(nodeID)]; ok {
		if ci.UnboundValues == nil {
			ci.UnboundValues = make(map[string]any)
		}
		ci.UnboundValues[stableKey] = propVal
	}

	// Build a targeted facts patch — no need for a full diff
	patch := vdom.Patch{
		NodeID: int(nodeID),
		Type:   vdom.PatchFacts,
		Data:   vdom.PatchFactsData{Diff: vdom.FactsDiff{Props: map[string]any{propKey: propVal}}},
	}
	msg := render.EncodePatchMessage([]vdom.Patch{patch})
	msg.TargetName = ci.SlotName

	data, _ := proto.Marshal(msg)
	s.pool.broadcast(data)
}

// handleMethodCall processes a Layer 2 method call: call the method on the
// component, then rebuild the tree and broadcast to all clients.
// If the component shares embedded pointers with siblings, their changed
// fields are surgically refreshed via MarkRefresh.
func (s *serverCtx) handleMethodCall(ci *component.Info, compIdx int, call *gproto.MethodCall) {
	// No lock needed — called only from processEvents (serialized).

	// Convert protobuf [][]byte to []json.RawMessage
	args := make([]json.RawMessage, len(call.Args))
	for i, a := range call.Args {
		args[i] = json.RawMessage(a)
	}

	// Sync g-bind and g-value/g-checked values from the live tree back to the struct.
	// Layer 1 keeps Tree props in sync with browser input values,
	// so we read from Tree and write to the struct before the method runs.
	if ci.Tree != nil && ci.Bindings != nil {
		for field, bindings := range ci.Bindings {
			for _, b := range bindings {
				if b.Kind != "bind" && b.Kind != "prop" {
					continue
				}
				node := findNode(b.NodeID, ci, s.lookup)
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
				propKey := "value"
				if b.Kind == "prop" && b.Prop != "" {
					propKey = b.Prop
				}
				val, exists := el.Facts.Props[propKey]
				if !exists {
					continue
				}
				raw, err := json.Marshal(val)
				if err != nil {
					continue
				}
				setPath := field
				if b.Expr != "" {
					setPath = b.Expr
				}
				ci.SetField(setPath, json.RawMessage(raw))
			}
		}
	}

	if env.Debug {
		log.Printf("godom: method call %q args=%v", call.Method, args)
	}

	// Call the method, then refresh the component and any siblings
	// that share embedded pointer state.
	if err := ci.CallMethod(call.Method, args); err != nil {
		log.Printf("godom: method call %q error: %v", call.Method, err)
		return
	}

	// Refresh the calling component (BuildUpdate + broadcast).
	s.executeRefresh(ci)

	// Surgically refresh siblings that share embedded pointer state,
	// using the changed fields captured during BuildUpdate above.
	changedFields := ci.LastChangedFields
	ci.LastChangedFields = nil
	s.sm.refreshSharedComponents(compIdx, changedFields)
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
