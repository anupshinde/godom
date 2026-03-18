package godom

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
)

// renderToHTML renders a Node tree to an HTML string.
// Used for the init message — the bridge receives this as innerHTML.
// Each element with events or bindings gets a data-gid attribute
// assigned from the gidCounter, enabling the bridge to target them.
func renderToHTML(nodes []Node, gid *gidCounter) string {
	var sb strings.Builder
	for _, n := range nodes {
		renderNode(&sb, n, gid)
	}
	return sb.String()
}

// gidCounter assigns sequential data-gid values during rendering.
type gidCounter struct {
	seq int
}

func (g *gidCounter) next() string {
	g.seq++
	return fmt.Sprintf("g%d", g.seq)
}

// renderNode writes a single node's HTML to the builder.
func renderNode(sb *strings.Builder, n Node, gid *gidCounter) {
	switch n := n.(type) {
	case *TextNode:
		sb.WriteString(html.EscapeString(n.Text))

	case *ElementNode:
		renderElement(sb, n.Tag, n.Namespace, &n.Facts, n.Children, nil, gid)

	case *KeyedElementNode:
		// Render keyed children in order
		children := make([]Node, len(n.Children))
		for i, kc := range n.Children {
			children[i] = kc.Node
		}
		renderElement(sb, n.Tag, n.Namespace, &n.Facts, children, nil, gid)

	case *ComponentNode:
		if n.SubTree != nil {
			renderNode(sb, n.SubTree, gid)
		}

	case *PluginNode:
		renderElement(sb, n.Tag, "", &n.Facts, nil, n, gid)

	case *LazyNode:
		if n.Cached != nil {
			renderNode(sb, n.Cached, gid)
		}
	}
}

// renderElement writes an HTML element with its facts and children.
func renderElement(sb *strings.Builder, tag, namespace string, facts *Facts, children []Node, plugin *PluginNode, gid *gidCounter) {
	sb.WriteByte('<')
	sb.WriteString(tag)

	// Assign data-gid if the element has events, bindings, or is a plugin
	needsGID := facts != nil && (len(facts.Events) > 0 || len(facts.Props) > 0 || len(facts.Styles) > 0 || plugin != nil)
	if needsGID {
		id := gid.next()
		sb.WriteString(` data-gid="`)
		sb.WriteString(id)
		sb.WriteByte('"')
	}

	// Plugin marker — bridge uses this to find the plugin handler
	if plugin != nil {
		sb.WriteString(` data-g-plugin="`)
		sb.WriteString(html.EscapeString(plugin.Name))
		sb.WriteByte('"')
		// Embed plugin init data so the bridge can init on first render
		if plugin.Data != nil {
			dataJSON, _ := json.Marshal(plugin.Data)
			sb.WriteString(` data-g-plugin-init="`)
			sb.WriteString(html.EscapeString(string(dataJSON)))
			sb.WriteByte('"')
		}
	}

	// Render properties that map to HTML attributes
	if facts != nil {
		renderFactsAsAttrs(sb, facts)
	}

	sb.WriteByte('>')

	// Void elements (no closing tag)
	if isVoidElement(tag) {
		return
	}

	// Children
	for _, c := range children {
		renderNode(sb, c, gid)
	}

	sb.WriteString("</")
	sb.WriteString(tag)
	sb.WriteByte('>')
}

// renderFactsAsAttrs writes Facts as HTML attributes.
func renderFactsAsAttrs(sb *strings.Builder, f *Facts) {
	// Properties → HTML attributes
	if f.Props != nil {
		// Sort keys for deterministic output
		keys := sortedKeys(f.Props)
		for _, k := range keys {
			v := f.Props[k]
			htmlAttr := propToHTMLAttr(k)
			if htmlAttr == "" {
				continue // skip non-renderable properties
			}
			switch val := v.(type) {
			case bool:
				if val {
					sb.WriteByte(' ')
					sb.WriteString(htmlAttr)
				}
			case string:
				sb.WriteByte(' ')
				sb.WriteString(htmlAttr)
				sb.WriteString(`="`)
				sb.WriteString(html.EscapeString(val))
				sb.WriteByte('"')
			default:
				sb.WriteByte(' ')
				sb.WriteString(htmlAttr)
				sb.WriteString(`="`)
				sb.WriteString(html.EscapeString(fmt.Sprint(val)))
				sb.WriteByte('"')
			}
		}
	}

	// Attributes
	if f.Attrs != nil {
		keys := sortedStringKeys(f.Attrs)
		for _, k := range keys {
			sb.WriteByte(' ')
			sb.WriteString(k)
			sb.WriteString(`="`)
			sb.WriteString(html.EscapeString(f.Attrs[k]))
			sb.WriteByte('"')
		}
	}

	// Namespaced attributes
	if f.AttrsNS != nil {
		for k, ns := range f.AttrsNS {
			sb.WriteByte(' ')
			sb.WriteString(k)
			sb.WriteString(`="`)
			sb.WriteString(html.EscapeString(ns.Value))
			sb.WriteByte('"')
			_ = ns.Namespace // namespace applied via setAttributeNS in bridge, not in HTML
		}
	}

	// Styles → style attribute
	if len(f.Styles) > 0 {
		sb.WriteString(` style="`)
		keys := sortedStringKeys(f.Styles)
		for i, k := range keys {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(html.EscapeString(f.Styles[k]))
		}
		sb.WriteByte('"')
	}
}

// propToHTMLAttr maps a DOM property name to its HTML attribute name.
// Returns "" for properties that have no HTML attribute equivalent.
func propToHTMLAttr(prop string) string {
	switch prop {
	case "className":
		return "class"
	case "htmlFor":
		return "for"
	case "value":
		return "value"
	case "checked":
		return "checked"
	case "disabled":
		return "disabled"
	case "id":
		return "id"
	case "draggable":
		return "draggable"
	case "style":
		return "" // handled separately
	default:
		// data-* and other pass-through attributes
		if strings.HasPrefix(prop, "data-") {
			return prop
		}
		return prop
	}
}

func isVoidElement(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------------------
// Rendering with event collection — used by VDOM init and patch encoding
// ---------------------------------------------------------------------------

// renderToHTMLWithEvents renders nodes to HTML and collects EventSetup entries
// for all elements that have event handlers. The EventSetup entries include
// pre-built WSMessage bytes so the bridge can send them back on event fire.
func renderToHTMLWithEvents(nodes []Node, gid *gidCounter) (string, []*EventSetup) {
	var sb strings.Builder
	var events []*EventSetup
	for _, n := range nodes {
		renderNodeWithEvents(&sb, n, gid, &events)
	}
	return sb.String(), events
}

// renderNodeWithEvents is like renderNode but also collects EventSetup entries.
func renderNodeWithEvents(sb *strings.Builder, n Node, gid *gidCounter, events *[]*EventSetup) {
	switch n := n.(type) {
	case *TextNode:
		sb.WriteString(html.EscapeString(n.Text))

	case *ElementNode:
		renderElementWithEvents(sb, n.Tag, n.Namespace, &n.Facts, n.Children, nil, gid, events)

	case *KeyedElementNode:
		children := make([]Node, len(n.Children))
		for i, kc := range n.Children {
			children[i] = kc.Node
		}
		renderElementWithEvents(sb, n.Tag, n.Namespace, &n.Facts, children, nil, gid, events)

	case *ComponentNode:
		if n.SubTree != nil {
			renderNodeWithEvents(sb, n.SubTree, gid, events)
		}

	case *PluginNode:
		renderElementWithEvents(sb, n.Tag, "", &n.Facts, nil, n, gid, events)

	case *LazyNode:
		if n.Cached != nil {
			renderNodeWithEvents(sb, n.Cached, gid, events)
		}
	}
}

// renderElementWithEvents writes an HTML element and collects events.
func renderElementWithEvents(sb *strings.Builder, tag, namespace string, facts *Facts, children []Node, plugin *PluginNode, gid *gidCounter, events *[]*EventSetup) {
	sb.WriteByte('<')
	sb.WriteString(tag)

	// Assign data-gid if the element has events, bindings, or is a plugin
	needsGID := facts != nil && (len(facts.Events) > 0 || len(facts.Props) > 0 || len(facts.Styles) > 0 || plugin != nil)
	var assignedGID string
	if needsGID {
		assignedGID = gid.next()
		sb.WriteString(` data-gid="`)
		sb.WriteString(assignedGID)
		sb.WriteByte('"')
	}

	// Plugin marker — bridge uses this to find the plugin handler
	if plugin != nil {
		sb.WriteString(` data-g-plugin="`)
		sb.WriteString(html.EscapeString(plugin.Name))
		sb.WriteByte('"')
		// Embed plugin init data so the bridge can init on first render
		if plugin.Data != nil {
			dataJSON, _ := json.Marshal(plugin.Data)
			sb.WriteString(` data-g-plugin-init="`)
			sb.WriteString(html.EscapeString(string(dataJSON)))
			sb.WriteByte('"')
		}
	}

	// Render properties that map to HTML attributes
	if facts != nil {
		renderFactsAsAttrs(sb, facts)
	}

	sb.WriteByte('>')

	// Collect events for this element
	if assignedGID != "" && facts != nil && len(facts.Events) > 0 {
		for eventKey, handler := range facts.Events {
			// Parse composite event key (e.g. "keydown:ArrowUp" → event="keydown", key="ArrowUp")
			eventName := eventKey
			keyFilter := handler.Options.Key
			if idx := strings.Index(eventKey, ":"); idx != -1 {
				eventName = eventKey[:idx]
				if keyFilter == "" {
					keyFilter = eventKey[idx+1:]
				}
			}

			es := &EventSetup{
				Gid:             assignedGID,
				Event:           eventName,
				Key:             keyFilter,
				StopPropagation: handler.Options.StopPropagation,
				PreventDefault:  handler.Options.PreventDefault,
				Msg:             buildWSMessageBytes(handler),
			}
			*events = append(*events, es)
		}
	}

	// Void elements (no closing tag)
	if isVoidElement(tag) {
		return
	}

	// Children
	for _, c := range children {
		renderNodeWithEvents(sb, c, gid, events)
	}

	sb.WriteString("</")
	sb.WriteString(tag)
	sb.WriteByte('>')
}

// buildWSMessageBytes builds a pre-encoded WSMessage for an event handler.
// For bind handlers: WSMessage{type:"bind", field:"FieldName"}
// For call handlers: WSMessage{type:"call", method:"MethodName", args:[...]}
func buildWSMessageBytes(handler EventHandler) []byte {
	if handler.Handler == "__bind__" {
		fieldName, _ := handler.Args[0].(string)
		msg := &WSMessage{Type: "bind", Field: fieldName}
		data, _ := proto.Marshal(msg)
		return data
	}

	msg := &WSMessage{Type: "call", Method: handler.Handler}
	for _, arg := range handler.Args {
		jsonArg, _ := json.Marshal(arg)
		msg.Args = append(msg.Args, jsonArg)
	}
	data, _ := proto.Marshal(msg)
	return data
}
