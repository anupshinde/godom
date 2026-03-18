package render

import (
	"encoding/json"

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

// EncodeInitMessage builds a VDomMessage for the initial full render.
func EncodeInitMessage(htmlContent string, events []*gproto.EventSetup) *gproto.VDomMessage {
	return &gproto.VDomMessage{
		Type:   "init",
		Html:   []byte(htmlContent),
		Events: events,
	}
}

// EncodePatchMessage builds a VDomMessage with patches from a diff.
func EncodePatchMessage(patches []vdom.Patch, gid *GIDCounter) *gproto.VDomMessage {
	msg := &gproto.VDomMessage{Type: "patch"}
	for _, p := range patches {
		dp := encodePatch(p, gid)
		if dp != nil {
			msg.Patches = append(msg.Patches, dp)
		}
	}
	return msg
}

// encodePatch converts a Go Patch to a protobuf DomPatch.
func encodePatch(p vdom.Patch, gid *GIDCounter) *gproto.DomPatch {
	dp := &gproto.DomPatch{
		Index: int32(p.Index),
	}

	switch p.Type {
	case vdom.PatchRedraw:
		dp.Op = OpRedraw
		data := p.Data.(vdom.PatchRedrawData)
		// Render the new node to HTML and collect events
		html, events := RenderToHTMLWithEvents([]vdom.Node{data.Node}, gid)
		dp.HtmlContent = []byte(html)
		dp.PatchEvents = events

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
		html, events := RenderToHTMLWithEvents(data.Nodes, gid)
		dp.HtmlContent = []byte(html)
		dp.PatchEvents = events

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
			sub := encodePatch(sp, gid)
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
			sub := encodePatch(sp, gid)
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
	Props   map[string]any         `json:"p,omitempty"` // properties
	Attrs   map[string]string      `json:"a,omitempty"` // attributes
	AttrsNS map[string]wireNSAttr  `json:"an,omitempty"` // namespaced attributes
	Styles  map[string]string      `json:"s,omitempty"` // styles
	Events  map[string]*wireEvent  `json:"e,omitempty"` // events
}

type wireNSAttr struct {
	NS  string `json:"ns"`
	Val string `json:"v"`
}

type wireEvent struct {
	Gid string `json:"gid"`          // data-gid of the element
	On  string `json:"on"`           // event type
	Key string `json:"key,omitempty"` // key filter
	Msg []byte `json:"msg"`          // pre-built WSMessage bytes
	SP  bool   `json:"sp,omitempty"` // stopPropagation
	PD  bool   `json:"pd,omitempty"` // preventDefault
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
				w.Events[k] = &wireEvent{
					On:  k,
					Key: v.Options.Key,
					SP:  v.Options.StopPropagation,
					PD:  v.Options.PreventDefault,
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
	Index int    `json:"i"`
	Key   string `json:"k"`
	HTML  string `json:"h,omitempty"` // rendered HTML for new nodes
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
			wi.HTML = RenderToHTML([]vdom.Node{ins.Node}, &GIDCounter{})
		}
		w.Inserts = append(w.Inserts, wi)
	}
	for _, rem := range d.Removes {
		w.Removes = append(w.Removes, wireReorderRemove{Index: rem.Index, Key: rem.Key})
	}
	return w
}
