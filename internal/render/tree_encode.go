package render

import (
	"encoding/json"
	"strings"

	"github.com/anupshinde/godom/internal/vdom"

	gproto "github.com/anupshinde/godom/internal/proto"
)

// WireNode is the JSON structure sent to the bridge for building DOM directly.
// The bridge creates DOM nodes from this and maintains nodeMap[id] → DOM node.
type WireNode struct {
	ID       int               `json:"id"`
	Type     string            `json:"t"`              // "text", "el", "keyed"
	Tag      string            `json:"tag,omitempty"`  // element tag name
	NS       string            `json:"ns,omitempty"`   // namespace (SVG)
	Text     string            `json:"x,omitempty"`    // text content (text nodes)
	Props    map[string]any    `json:"p,omitempty"`    // DOM properties
	Attrs    map[string]string `json:"a,omitempty"`    // HTML attributes
	AttrsNS  map[string]wireNSAttr `json:"an,omitempty"` // namespaced attributes
	Styles   map[string]string `json:"s,omitempty"`    // CSS styles
	Events   []*WireNodeEvent  `json:"ev,omitempty"`   // event listeners
	Children []*WireNode       `json:"c,omitempty"`    // child nodes
	Keys     []string          `json:"k,omitempty"`    // keys for keyed children (parallel to children)
	Plugin   string            `json:"plug,omitempty"` // plugin name
	PlugData any               `json:"pd,omitempty"`   // plugin data
}

// WireNodeEvent describes an event listener for the bridge to register.
type WireNodeEvent struct {
	On     string   `json:"on"`              // event type: "click", "input", etc.
	Key    string   `json:"key,omitempty"`   // key filter for keydown
	Method string   `json:"method"`          // Go method name (or "__bind__" for g-bind)
	Args   [][]byte `json:"args,omitempty"`  // JSON-encoded arguments
	SP     bool     `json:"sp,omitempty"`    // stopPropagation
	PD     bool     `json:"pd,omitempty"`    // preventDefault
}

// EncodeTree converts a vdom.Node tree to a WireNode tree suitable for JSON transport.
func EncodeTree(n vdom.Node) *WireNode {
	if n == nil {
		return nil
	}

	switch n := n.(type) {
	case *vdom.TextNode:
		return &WireNode{
			ID:   n.ID,
			Type: "text",
			Text: n.Text,
		}

	case *vdom.ElementNode:
		wn := &WireNode{
			ID:   n.ID,
			Type: "el",
			Tag:  n.Tag,
			NS:   n.Namespace,
		}
		encodeFacts(&n.Facts, wn)
		for _, child := range n.Children {
			wn.Children = append(wn.Children, EncodeTree(child))
		}
		return wn

	case *vdom.KeyedElementNode:
		wn := &WireNode{
			ID:   n.ID,
			Type: "keyed",
			Tag:  n.Tag,
			NS:   n.Namespace,
		}
		encodeFacts(&n.Facts, wn)
		for _, kc := range n.Children {
			wn.Keys = append(wn.Keys, kc.Key)
			wn.Children = append(wn.Children, EncodeTree(kc.Node))
		}
		return wn

	case *vdom.PluginNode:
		wn := &WireNode{
			ID:     n.ID,
			Type:   "el",
			Tag:    n.Tag,
			NS:     "",
			Plugin: n.Name,
		}
		encodeFacts(&n.Facts, wn)
		if n.Data != nil {
			wn.PlugData = n.Data
		}
		return wn

	case *vdom.LazyNode:
		if n.Cached != nil {
			return EncodeTree(n.Cached)
		}
		return nil
	}

	return nil
}

// encodeFacts populates the WireNode's props/attrs/styles/events from Facts.
func encodeFacts(f *vdom.Facts, wn *WireNode) {
	if len(f.Props) > 0 {
		wn.Props = f.Props
	}
	if len(f.Attrs) > 0 {
		wn.Attrs = f.Attrs
	}
	if len(f.AttrsNS) > 0 {
		wn.AttrsNS = make(map[string]wireNSAttr, len(f.AttrsNS))
		for k, v := range f.AttrsNS {
			wn.AttrsNS[k] = wireNSAttr{NS: v.Namespace, Val: v.Value}
		}
	}
	if len(f.Styles) > 0 {
		wn.Styles = f.Styles
	}
	// Layer 2: encode event handlers for the bridge to register.
	if len(f.Events) > 0 {
		for key, eh := range f.Events {
			eventType := key
			keyFilter := ""
			if idx := strings.Index(key, ":"); idx != -1 {
				eventType = key[:idx]
				keyFilter = key[idx+1:]
			}
			if keyFilter == "" {
				keyFilter = eh.Options.Key
			}
			var argBytes [][]byte
			for _, arg := range eh.Args {
				b, _ := json.Marshal(arg)
				argBytes = append(argBytes, b)
			}
			wn.Events = append(wn.Events, &WireNodeEvent{
				On:     eventType,
				Key:    keyFilter,
				Method: eh.Handler,
				Args:   argBytes,
				SP:     eh.Options.StopPropagation,
				PD:     eh.Options.PreventDefault,
			})
		}
	}
}

// EncodeTreeJSON serializes a vdom.Node tree to JSON bytes for the init message.
func EncodeTreeJSON(n vdom.Node) ([]byte, error) {
	wn := EncodeTree(n)
	if wn == nil {
		return []byte("null"), nil
	}
	return json.Marshal(wn)
}

// EncodeInitTreeMessage builds a ServerMessage for init using a tree description
// instead of HTML. The bridge builds DOM directly from this.
func EncodeInitTreeMessage(root vdom.Node) (*gproto.ServerMessage, error) {
	treeJSON, err := EncodeTreeJSON(root)
	if err != nil {
		return nil, err
	}
	return &gproto.ServerMessage{
		Kind: "init",
		Tree: treeJSON,
	}, nil
}
