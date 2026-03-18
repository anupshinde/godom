package godom

import (
	"encoding/json"

	gproto "github.com/anupshinde/godom/proto"
	"github.com/anupshinde/godom/vdom"
)

// Patch op names sent over the wire — bridge.js dispatches on these.
const (
	opRedraw     = "redraw"
	opText       = "text"
	opFacts      = "facts"
	opAppend     = "append"
	opRemoveLast = "remove-last"
	opRemove     = "remove"
	opReorder    = "reorder"
	opPlugin     = "plugin"
	opLazy       = "lazy"
)

// encodeInitMessage builds a VDomMessage for the initial full render.
func encodeInitMessage(htmlContent string, events []*gproto.EventSetup) *gproto.VDomMessage {
	return &gproto.VDomMessage{
		Type:   "init",
		Html:   []byte(htmlContent),
		Events: events,
	}
}

// encodePatchMessage builds a VDomMessage with patches from a diff.
func encodePatchMessage(patches []vdom.Patch, gid *gidCounter) *gproto.VDomMessage {
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
func encodePatch(p vdom.Patch, gid *gidCounter) *gproto.DomPatch {
	dp := &gproto.DomPatch{
		Index: int32(p.Index),
	}

	switch p.Type {
	case vdom.PatchRedraw:
		dp.Op = opRedraw
		data := p.Data.(vdom.PatchRedrawData)
		// Render the new node to HTML and collect events
		html, events := renderToHTMLWithEvents([]vdom.Node{data.Node}, gid)
		dp.HtmlContent = []byte(html)
		dp.PatchEvents = events

	case vdom.PatchText:
		dp.Op = opText
		data := p.Data.(vdom.PatchTextData)
		dp.Text = data.Text

	case vdom.PatchFacts:
		dp.Op = opFacts
		data := p.Data.(vdom.PatchFactsData)
		factsJSON, _ := json.Marshal(encodeFactsDiff(&data.Diff))
		dp.Facts = factsJSON

	case vdom.PatchAppend:
		dp.Op = opAppend
		data := p.Data.(vdom.PatchAppendData)
		html, events := renderToHTMLWithEvents(data.Nodes, gid)
		dp.HtmlContent = []byte(html)
		dp.PatchEvents = events

	case vdom.PatchRemoveLast:
		dp.Op = opRemoveLast
		data := p.Data.(vdom.PatchRemoveLastData)
		dp.Count = int32(data.Count)

	case vdom.PatchRemove:
		dp.Op = opRemove

	case vdom.PatchReorder:
		dp.Op = opReorder
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
		dp.Op = opPlugin
		data := p.Data.(vdom.PatchPluginData)
		pluginJSON, _ := json.Marshal(data.Data)
		dp.PluginData = pluginJSON

	case vdom.PatchLazy:
		dp.Op = opLazy
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

// wireFactsDiff is the JSON structure sent to the bridge.
type wireFactsDiff struct {
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

func encodeFactsDiff(d *vdom.FactsDiff) *wireFactsDiff {
	w := &wireFactsDiff{}

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
					// Msg and Gid are filled in when we have the full context
					// (gid assignment happens during renderToHTML)
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
			wi.HTML = renderToHTML([]vdom.Node{ins.Node}, &gidCounter{})
		}
		w.Inserts = append(w.Inserts, wi)
	}
	for _, rem := range d.Removes {
		w.Removes = append(w.Removes, wireReorderRemove{Index: rem.Index, Key: rem.Key})
	}
	return w
}

// Event collection is now integrated into renderToHTMLWithEvents in render_html.go.
// Events are collected during HTML rendering so that gid assignment and event
// registration happen in a single pass.
