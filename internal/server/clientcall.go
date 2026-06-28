package server

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	gproto "github.com/anupshinde/godom/internal/proto"
	"google.golang.org/protobuf/proto"
)

// clientCapMethod is the reserved godom.call method name the bridge uses to
// advertise a per-client module capability. It rides the BROWSER_METHOD channel
// (like the env handshake), can be sent any time a module initializes, and is
// re-sent on reconnect.
const clientCapMethod = "__godom_capability__"

// applyClientCapability records a capability the bridge advertised for a client.
// Runs on the connection read-loop goroutine.
func applyClientCapability(client *Client, args [][]byte) {
	if client == nil || len(args) == 0 {
		return
	}
	var name string
	if err := json.Unmarshal(args[0], &name); err != nil || name == "" {
		return
	}
	client.addCapability(name)
}

// ClientsWith returns the clients from the given set that have advertised the
// named capability. Engine.ClientsWith is a thin public wrapper over it.
func ClientsWith(clients []*Client, capability string) []*Client {
	var out []*Client
	for _, c := range clients {
		if c != nil && c.Has(capability) {
			out = append(out, c)
		}
	}
	return out
}

// --- per-client pending-call registry -------------------------------------

func (c *Client) nextCallID() int32 {
	c.callMu.Lock()
	defer c.callMu.Unlock()
	c.callID-- // negative space, distinct from island ExecJS (positive)
	return c.callID
}

func (c *Client) registerCall(id int32, cb func(result []byte, errMsg string)) {
	c.callMu.Lock()
	if c.pending == nil {
		c.pending = make(map[int32]func(result []byte, errMsg string))
	}
	c.pending[id] = cb
	c.callMu.Unlock()
}

// handleResult delivers a JS result to the matching pending callback. Returns
// false if no call with that id is pending (e.g. it was a broadcast ExecJS
// reply, or already cancelled).
func (c *Client) handleResult(id int32, result []byte, errMsg string) bool {
	c.callMu.Lock()
	cb, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.callMu.Unlock()
	if ok && cb != nil {
		cb(result, errMsg)
	}
	return ok
}

// cancelPending fails every in-flight call with errMsg. Called when the socket
// drops so a blocked Call returns instead of hanging.
func (c *Client) cancelPending(errMsg string) {
	c.callMu.Lock()
	pend := c.pending
	c.pending = nil
	c.callMu.Unlock()
	for _, cb := range pend {
		if cb != nil {
			cb(nil, errMsg)
		}
	}
}

// --- targeted JS eval and typed module calls ------------------------------

// Eval runs a JavaScript expression on this one client (unlike ExecJS, which
// broadcasts to every tab) and invokes cb once with the JSON-encoded result and
// an error string. cb may be nil for fire-and-forget. Safe to call from a
// handler — it does not block.
func (c *Client) Eval(expr string, cb func(result []byte, errMsg string)) {
	id := c.nextCallID()
	if cb != nil {
		c.registerCall(id, cb)
	}
	msg := &gproto.ServerMessage{Kind: gproto.ServerKind_SERVER_JSCALL, CallId: id, Expr: expr}
	data, err := proto.Marshal(msg)
	if err != nil {
		c.handleResult(id, nil, "godom: marshal jscall: "+err.Error())
		return
	}
	if err := c.send(data); err != nil {
		c.handleResult(id, nil, "godom: send to client: "+err.Error())
	}
}

// moduleCallExpr builds the JS expression that invokes a registered client
// module's function: "widget.render" + args → godom.modules.widget.render(args).
func moduleCallExpr(method string, argsJSON []byte) string {
	return "window.godom.modules." + method + "(" + string(argsJSON) + ")"
}

// CallAsync invokes a registered client module function on this client with
// JSON-marshaled args and delivers the JSON-encoded reply (or a JS error) to cb.
// Non-blocking — safe from a handler.
func (c *Client) CallAsync(method string, args any, cb func(result []byte, errMsg string)) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		if cb != nil {
			cb(nil, "godom: marshal args: "+err.Error())
		}
		return
	}
	c.Eval(moduleCallExpr(method, argsJSON), cb)
}

// Call invokes a registered client module function and blocks until the reply,
// unmarshaling it into out (which may be nil) and surfacing a JS-side throw as a
// Go error. Because it blocks on a browser round-trip, it MUST be called off the
// island event loop — e.g. inside a Task closure. If the client disconnects
// mid-call it returns a "client disconnected" error rather than hanging.
func (c *Client) Call(method string, args any, out any) error {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("call %s: marshal args: %w", method, err)
	}
	type reply struct {
		data   []byte
		errMsg string
	}
	ch := make(chan reply, 1)
	c.Eval(moduleCallExpr(method, argsJSON), func(data []byte, errMsg string) {
		ch <- reply{data: data, errMsg: errMsg}
	})
	r := <-ch
	if r.errMsg != "" {
		return fmt.Errorf("call %s: %s", method, r.errMsg)
	}
	if out != nil && len(r.data) > 0 {
		if err := json.Unmarshal(r.data, out); err != nil {
			return fmt.Errorf("call %s: unmarshal result: %w", method, err)
		}
	}
	return nil
}

// --- module bundle injection ----------------------------------------------

// clientModulesJS builds the JS that ships registered client modules to the
// browser: it ensures the godom.modules namespace exists, then appends each
// module's script (which assigns itself onto godom.modules). Returns "" when
// there are no modules. Names are emitted in sorted order for a stable bundle.
func clientModulesJS(modules map[string]string) string {
	if len(modules) == 0 {
		return ""
	}
	names := make([]string, 0, len(modules))
	for n := range modules {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(";(function(){var g=window[window.GODOM_NS||'godom']=window[window.GODOM_NS||'godom']||{};g.modules=g.modules||{};})();\n")
	for _, n := range names {
		b.WriteString(modules[n])
		b.WriteString("\n")
	}
	return b.String()
}
