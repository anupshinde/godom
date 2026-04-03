package render

import (
	"encoding/json"
	"strings"

	gproto "github.com/anupshinde/godom/internal/proto"
	"github.com/anupshinde/godom/internal/vdom"
)

// Patch op names sent over the wire — bridge.js dispatches on these.
const (
	OpRedraw     = "redraw"
	OpText       = "text"
	OpFacts      = "facts"
	OpAppend     = "append"
	OpRemoveLast = "remove-last"
	OpRemove     = "remove"
	OpReorder    = "reorder"
	OpPlugin     = "plugin"
	OpLazy       = "lazy"
)

// EncodePatchMessage builds a VDomMessage with patches from a diff.
func EncodePatchMessage(patches []vdom.Patch) *gproto.ServerMessage {
	msg := &gproto.ServerMessage{Kind: "patch"}
	for _, p := range patches {
		dp := encodePatch(p)
		if dp != nil {
			msg.Patches = append(msg.Patches, dp)
		}
	}
	return msg
}

// encodePatch converts a Go Patch to a protobuf DomPatch.
func encodePatch(p vdom.Patch) *gproto.DomPatch {
	dp := &gproto.DomPatch{
		NodeId: int32(p.NodeID),
	}

	switch p.Type {
	case vdom.PatchRedraw:
		dp.Op = OpRedraw
		data := p.Data.(vdom.PatchRedrawData)
		treeJSON, _ := json.Marshal(EncodeTree(data.Node))
		dp.TreeContent = treeJSON

	case vdom.PatchText:
		dp.Op = OpText
		data := p.Data.(vdom.PatchTextData)
		dp.Text = data.Text

	case vdom.PatchFacts:
		dp.Op = OpFacts
		data := p.Data.(vdom.PatchFactsData)
		factsJSON, _ := json.Marshal(encodeFactsDiff(&data.Diff))
		dp.Facts = factsJSON

	case vdom.PatchAppend:
		dp.Op = OpAppend
		data := p.Data.(vdom.PatchAppendData)
		// Encode each appended node as a tree description
		trees := make([]*WireNode, len(data.Nodes))
		for i, n := range data.Nodes {
			trees[i] = EncodeTree(n)
		}
		treeJSON, _ := json.Marshal(trees)
		dp.TreeContent = treeJSON

	case vdom.PatchRemoveLast:
		dp.Op = OpRemoveLast
		data := p.Data.(vdom.PatchRemoveLastData)
		dp.Count = int32(data.Count)

	case vdom.PatchRemove:
		dp.Op = OpRemove

	case vdom.PatchReorder:
		dp.Op = OpReorder
		data := p.Data.(vdom.PatchReorderData)
		reorderJSON, _ := json.Marshal(encodeReorderData(&data))
		dp.Reorder = reorderJSON
		// Sub-patches for changed keyed children
		for _, sp := range data.Patches {
			sub := encodePatch(sp)
			if sub != nil {
				dp.SubPatches = append(dp.SubPatches, sub)
			}
		}

	case vdom.PatchPlugin:
		dp.Op = OpPlugin
		data := p.Data.(vdom.PatchPluginData)
		pluginJSON, _ := json.Marshal(data.Data)
		dp.PluginData = pluginJSON

	case vdom.PatchLazy:
		dp.Op = OpLazy
		data := p.Data.(vdom.PatchLazyData)
		for _, sp := range data.Patches {
			sub := encodePatch(sp)
			if sub != nil {
				dp.SubPatches = append(dp.SubPatches, sub)
			}
		}

	default:
		return nil
	}

	return dp
}

// ---------------------------------------------------------------------------
// FactsDiff encoding for JSON transport
// ---------------------------------------------------------------------------

// WireFactsDiff is the JSON structure sent to the bridge.
type WireFactsDiff struct {
	Props   map[string]any        `json:"p,omitempty"`  // properties
	Attrs   map[string]string     `json:"a,omitempty"`  // attributes
	AttrsNS map[string]wireNSAttr `json:"an,omitempty"` // namespaced attributes
	Styles  map[string]string     `json:"s,omitempty"`  // styles
	Events  map[string]*wireEvent `json:"e,omitempty"`  // events
}

type wireNSAttr struct {
	NS  string `json:"ns"`
	Val string `json:"v"`
}

type wireEvent struct {
	On     string   `json:"on"`              // event type
	Key    string   `json:"key,omitempty"`   // key filter
	Method string   `json:"method"`          // Go method name
	Args   [][]byte `json:"args,omitempty"`  // JSON-encoded arguments
	SP     bool     `json:"sp,omitempty"`    // stopPropagation
	PD     bool     `json:"pd,omitempty"`    // preventDefault
}

func encodeFactsDiff(d *vdom.FactsDiff) *WireFactsDiff {
	w := &WireFactsDiff{}

	if len(d.Props) > 0 {
		w.Props = d.Props
	}
	if len(d.Attrs) > 0 {
		w.Attrs = d.Attrs
	}
	if len(d.AttrsNS) > 0 {
		w.AttrsNS = make(map[string]wireNSAttr, len(d.AttrsNS))
		for k, v := range d.AttrsNS {
			w.AttrsNS[k] = wireNSAttr{NS: v.Namespace, Val: v.Value}
		}
	}
	if len(d.Styles) > 0 {
		w.Styles = d.Styles
	}
	if len(d.Events) > 0 {
		w.Events = make(map[string]*wireEvent, len(d.Events))
		for k, v := range d.Events {
			if v == nil {
				w.Events[k] = nil // removal
			} else {
				eventType := k
				keyFilter := v.Options.Key
				if idx := strings.Index(k, ":"); idx != -1 {
					eventType = k[:idx]
					if keyFilter == "" {
						keyFilter = k[idx+1:]
					}
				}
				var argBytes [][]byte
				for _, arg := range v.Args {
					b, _ := json.Marshal(arg)
					argBytes = append(argBytes, b)
				}
				w.Events[k] = &wireEvent{
					On:     eventType,
					Key:    keyFilter,
					Method: v.Handler,
					Args:   argBytes,
					SP:     v.Options.StopPropagation,
					PD:     v.Options.PreventDefault,
				}
			}
		}
	}

	return w
}

// ---------------------------------------------------------------------------
// Reorder encoding
// ---------------------------------------------------------------------------

type wireReorder struct {
	Inserts []wireReorderInsert `json:"ins"`
	Removes []wireReorderRemove `json:"rem"`
}

type wireReorderInsert struct {
	Index int       `json:"i"`
	Key   string    `json:"k"`
	Tree  *WireNode `json:"tree,omitempty"` // tree description for new nodes
}

type wireReorderRemove struct {
	Index int    `json:"i"`
	Key   string `json:"k"`
}

func encodeReorderData(d *vdom.PatchReorderData) *wireReorder {
	w := &wireReorder{}
	for _, ins := range d.Inserts {
		wi := wireReorderInsert{Index: ins.Index, Key: ins.Key}
		if ins.Node != nil {
			wi.Tree = EncodeTree(ins.Node)
		}
		w.Inserts = append(w.Inserts, wi)
	}
	for _, rem := range d.Removes {
		w.Removes = append(w.Removes, wireReorderRemove{Index: rem.Index, Key: rem.Key})
	}
	return w
}
