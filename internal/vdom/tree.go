package vdom

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ---------------------------------------------------------------------------
// Template tree — parsed once at Mount() time, reused on every render.
// ---------------------------------------------------------------------------

// TemplateNode represents one node in the parsed template tree.
type TemplateNode struct {
	// For element nodes
	Tag       string
	Namespace string
	Attrs     []html.Attribute // static HTML attributes (non-directive)

	// Directives (extracted from g-* attributes)
	Directives []Directive

	// Children (for elements) or nil (for text)
	Children []*TemplateNode

	// For text nodes
	IsText    bool
	TextParts []TextPart // static text + {expr} interpolations

	// For g-for nodes
	IsFor    bool
	ForItem  string // loop variable name, e.g. "todo"
	ForIndex string // index variable name, e.g. "i" (empty if unused)
	ForList  string // list field, e.g. "Todos"
	ForKey   string // key expression, e.g. "todo.ID" (empty = positional)
	ForBody  []*TemplateNode // template for each item

	// For plugin nodes
	IsPlugin   bool
	PluginName string // plugin name from g-plugin:name
	PluginExpr string // data expression

	// For g-slot nodes
	IsSlot   bool
	SlotExpr string // instance name expression (e.g. "counter" or "{{slot.Name}}")
	SlotType string // type attribute (e.g. "component:Counter"), required for static names

	// StableID is a UUID assigned at parse time to unbound form inputs.
	// Used to preserve input values across tree rebuilds.
	StableID string
}

// Directive represents a single g-* directive on an element.
type Directive struct {
	Type string // "text", "html", "bind", "value", "checked", "if", "show", "hide", "class", "attr", "style", "prop",
	           // "click", "keydown", "mousedown", "mousemove", "mouseup", "wheel", "scroll", "drop",
	           // "draggable", "dropzone"
	Name string // modifier name: class name, attr name, style property, key filter, etc.
	Expr string // expression: field name, method call, etc.
}

// TextPart represents a segment of text content.
type TextPart struct {
	Static bool
	Value  string // literal text if Static, expression string if not
}

// ---------------------------------------------------------------------------
// HTML → Template tree parser
// ---------------------------------------------------------------------------

// ParseTemplate parses HTML into a template tree.
func ParseTemplate(htmlStr string) ([]*TemplateNode, error) {
	// Go's html.Parse doesn't treat custom elements as void/self-closing.
	// <g-slot .../> is parsed as <g-slot ...> (open tag), which
	// swallows all subsequent siblings as children. Expand to explicit
	// closing tags so the parser produces correct sibling structure.
	htmlStr = expandSelfClosingGSlot(htmlStr)

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil { // unreachable: html.Parse never errors, but kept as defensive check
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	body := findBody(doc)
	if body == nil { // unreachable: html.Parse always synthesizes <body>, but kept as defensive check
		return nil, fmt.Errorf("no <body> found in HTML")
	}

	var nodes []*TemplateNode
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if tn := htmlToTemplate(c); tn != nil {
			nodes = append(nodes, tn)
		}
	}
	if err := checkDuplicateSlots(nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// checkDuplicateSlots walks the template tree and returns an error if any
// static slot name appears more than once.
func checkDuplicateSlots(nodes []*TemplateNode) error {
	seen := map[string]bool{}
	var walk func([]*TemplateNode) error
	walk = func(nodes []*TemplateNode) error {
		for _, n := range nodes {
			if n.IsSlot {
				if n.SlotExpr == "" {
					return fmt.Errorf("<g-slot> requires a non-empty 'instance' attribute")
				}
				isInterpolated := strings.Contains(n.SlotExpr, "{")
				if isInterpolated {
					// Interpolated names are dynamic — skip duplicate and type checks
				} else {
					// Static name — must be a valid identifier, type attribute is required
					if !IsValidIdentifier(n.SlotExpr) {
						return fmt.Errorf("<g-slot instance=%q> must be a valid identifier (letters, digits, underscores; cannot start with a digit)", n.SlotExpr)
					}
					if n.SlotType == "" {
						return fmt.Errorf("<g-slot instance=%q> requires a 'type' attribute for static names", n.SlotExpr)
					}
					if !strings.HasPrefix(n.SlotType, "component:") || strings.TrimPrefix(n.SlotType, "component:") == "" {
						return fmt.Errorf("<g-slot instance=%q> type must be \"component:TypeName\", got %q", n.SlotExpr, n.SlotType)
					}
					if seen[n.SlotExpr] {
						return fmt.Errorf("duplicate <g-slot> instance %q in template", n.SlotExpr)
					}
					seen[n.SlotExpr] = true
				}
			}
			if err := walk(n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(nodes)
}

func htmlToTemplate(n *html.Node) *TemplateNode {
	switch n.Type {
	case html.TextNode:
		text := n.Data
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return &TemplateNode{IsText: true, TextParts: ParseTextInterpolations(text)}

	case html.ElementNode:
		return htmlElementToTemplate(n)

	case html.CommentNode:
		return nil

	default:
		return nil
	}
}

func htmlElementToTemplate(n *html.Node) *TemplateNode {
	tag := n.Data

	if forExpr := getAttrVal(n, "g-for"); forExpr != "" {
		return parseForTemplate(n, forExpr)
	}

	// <g-slot type="component:Counter" instance="counter">
	// <g-slot type="component:Counter" instance="counter"/>
	// <g-slot instance="{{slot.Name}}"/>  — interpolated, type optional
	if tag == "g-slot" {
		instance := getAttrVal(n, "instance")
		slotType := getAttrVal(n, "type")
		tn := &TemplateNode{IsSlot: true, SlotExpr: instance, SlotType: slotType}
		return tn
	}

	pluginName, pluginExpr := extractPluginDirective(n)
	if pluginName != "" {
		tn := &TemplateNode{
			Tag:        tag,
			IsPlugin:   true,
			PluginName: pluginName,
			PluginExpr: pluginExpr,
		}
		tn.Attrs, tn.Directives = extractAttrsAndDirectives(n)
		return tn
	}

	tn := &TemplateNode{Tag: tag}
	tn.Attrs, tn.Directives = extractAttrsAndDirectives(n)

	// Assign a stable ID to unbound form inputs so their values survive rebuilds.
	if isFormInput(tag) && !hasBind(tn.Directives) {
		tn.StableID = genUUID()
	}

	if tag == "svg" || n.Namespace == "svg" {
		tn.Namespace = "http://www.w3.org/2000/svg"
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := htmlToTemplate(c); child != nil {
			if tn.Namespace != "" && !child.IsText {
				child.Namespace = tn.Namespace
			}
			tn.Children = append(tn.Children, child)
		}
	}

	return tn
}

func parseForTemplate(n *html.Node, forExpr string) *TemplateNode {
	item, index, list := ParseForExpr(forExpr)

	keyExpr := getAttrVal(n, "g-key")

	tn := &TemplateNode{
		IsFor:    true,
		ForItem:  item,
		ForIndex: index,
		ForList:  list,
		ForKey:   keyExpr,
		Tag:      n.Data,
	}

	itemTemplate := &TemplateNode{Tag: n.Data}
	itemTemplate.Attrs, itemTemplate.Directives = extractAttrsAndDirectives(n)

	if n.Data == "svg" || n.Namespace == "svg" {
		itemTemplate.Namespace = "http://www.w3.org/2000/svg"
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := htmlToTemplate(c); child != nil {
			if itemTemplate.Namespace != "" && !child.IsText {
				child.Namespace = itemTemplate.Namespace
			}
			itemTemplate.Children = append(itemTemplate.Children, child)
		}
	}

	tn.ForBody = []*TemplateNode{itemTemplate}
	return tn
}

// ---------------------------------------------------------------------------
// Attribute / directive extraction
// ---------------------------------------------------------------------------

func extractAttrsAndDirectives(n *html.Node) ([]html.Attribute, []Directive) {
	var attrs []html.Attribute
	var dirs []Directive

	for _, a := range n.Attr {
		switch {
		case a.Key == "g-for", a.Key == "g-key":
			continue
		case strings.HasPrefix(a.Key, ":"):
			continue

		case a.Key == "g-text":
			dirs = append(dirs, Directive{Type: "text", Expr: a.Val})
		case a.Key == "g-html":
			dirs = append(dirs, Directive{Type: "html", Expr: a.Val})
		case a.Key == "g-bind":
			dirs = append(dirs, Directive{Type: "bind", Expr: a.Val})
		case a.Key == "g-value":
			dirs = append(dirs, Directive{Type: "value", Expr: a.Val})
		case a.Key == "g-checked":
			dirs = append(dirs, Directive{Type: "checked", Expr: a.Val})
		case a.Key == "g-if":
			dirs = append(dirs, Directive{Type: "if", Expr: a.Val})
		case a.Key == "g-show":
			dirs = append(dirs, Directive{Type: "show", Expr: a.Val})
		case a.Key == "g-hide":
			dirs = append(dirs, Directive{Type: "hide", Expr: a.Val})

		case strings.HasPrefix(a.Key, "g-class:"):
			dirs = append(dirs, Directive{Type: "class", Name: a.Key[len("g-class:"):], Expr: a.Val})
		case strings.HasPrefix(a.Key, "g-attr:"):
			dirs = append(dirs, Directive{Type: "attr", Name: a.Key[len("g-attr:"):], Expr: a.Val})
		case strings.HasPrefix(a.Key, "g-style:"):
			dirs = append(dirs, Directive{Type: "style", Name: a.Key[len("g-style:"):], Expr: a.Val})
		case strings.HasPrefix(a.Key, "g-prop:"):
			dirs = append(dirs, Directive{Type: "prop", Name: kebabToCamel(a.Key[len("g-prop:"):]), Expr: a.Val})

		case a.Key == "g-click":
			dirs = append(dirs, Directive{Type: "click", Expr: a.Val})
		case a.Key == "g-keydown":
			for _, part := range strings.Split(a.Val, ";") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				key, method := "", part
				if idx := strings.Index(part, ":"); idx != -1 {
					key = part[:idx]
					method = part[idx+1:]
				}
				dirs = append(dirs, Directive{Type: "keydown", Name: key, Expr: method})
			}
		case a.Key == "g-mousedown":
			dirs = append(dirs, Directive{Type: "mousedown", Expr: a.Val})
		case a.Key == "g-mousemove":
			dirs = append(dirs, Directive{Type: "mousemove", Expr: a.Val})
		case a.Key == "g-mouseup":
			dirs = append(dirs, Directive{Type: "mouseup", Expr: a.Val})
		case a.Key == "g-wheel":
			dirs = append(dirs, Directive{Type: "wheel", Expr: a.Val})
		case a.Key == "g-scroll":
			dirs = append(dirs, Directive{Type: "scroll", Expr: a.Val})
		case a.Key == "g-drop" || strings.HasPrefix(a.Key, "g-drop:"):
			group := ""
			if strings.HasPrefix(a.Key, "g-drop:") {
				group = a.Key[len("g-drop:"):]
			}
			dirs = append(dirs, Directive{Type: "drop", Name: group, Expr: a.Val})

		case a.Key == "g-draggable" || strings.HasPrefix(a.Key, "g-draggable:"):
			group := ""
			if strings.HasPrefix(a.Key, "g-draggable:") {
				group = a.Key[len("g-draggable:"):]
			}
			dirs = append(dirs, Directive{Type: "draggable", Name: group, Expr: a.Val})
		case a.Key == "g-dropzone":
			dirs = append(dirs, Directive{Type: "dropzone", Expr: a.Val})

		case strings.HasPrefix(a.Key, "g-plugin:"):
			continue

		default:
			attrs = append(attrs, a)
		}
	}

	return attrs, dirs
}

func extractPluginDirective(n *html.Node) (name, expr string) {
	for _, a := range n.Attr {
		if strings.HasPrefix(a.Key, "g-plugin:") {
			return a.Key[len("g-plugin:"):], a.Val
		}
	}
	return "", ""
}

// ---------------------------------------------------------------------------
// Text interpolation
// ---------------------------------------------------------------------------

// ParseTextInterpolations splits text containing {{expr}} into parts.
func ParseTextInterpolations(text string) []TextPart {
	var parts []TextPart
	for {
		start := strings.Index(text, "{{")
		if start == -1 {
			if text != "" {
				parts = append(parts, TextPart{Static: true, Value: text})
			}
			break
		}
		end := strings.Index(text[start:], "}}")
		if end == -1 {
			parts = append(parts, TextPart{Static: true, Value: text})
			break
		}
		end += start

		if start > 0 {
			parts = append(parts, TextPart{Static: true, Value: text[:start]})
		}
		expr := strings.TrimSpace(text[start+2 : end])
		if expr != "" {
			parts = append(parts, TextPart{Static: false, Value: expr})
		}
		text = text[end+2:]
	}
	if len(parts) == 0 {
		return []TextPart{{Static: true, Value: ""}}
	}
	return parts
}

// ---------------------------------------------------------------------------
// g-for expression parsing
// ---------------------------------------------------------------------------

// ParseForExpr parses "todo in Todos" or "todo, i in Todos".
func ParseForExpr(expr string) (item, index, list string) {
	inIdx := strings.Index(expr, " in ")
	if inIdx == -1 {
		return "", "", expr
	}
	lhs := strings.TrimSpace(expr[:inIdx])
	list = strings.TrimSpace(expr[inIdx+4:])

	if commaIdx := strings.Index(lhs, ","); commaIdx != -1 {
		item = strings.TrimSpace(lhs[:commaIdx])
		index = strings.TrimSpace(lhs[commaIdx+1:])
	} else {
		item = lhs
	}
	return
}

// ---------------------------------------------------------------------------
// Template tree → resolved Node tree
// ---------------------------------------------------------------------------

// IDCounter assigns monotonically increasing node IDs.
// It must persist across renders (never reset) so that
// existing IDs in the bridge's node map remain valid.
type IDCounter struct {
	Seq int
}

func (c *IDCounter) Next() int {
	c.Seq++
	return c.Seq
}

// Binding records a dependency: "field X affects node Y's property Z."
// Used for surgical updates — when a field changes, only the bound nodes are patched.
type Binding struct {
	NodeID int
	Kind   string // "style", "prop", "attr", "text"
	Prop   string // property/style/attr name (empty for text)
	Expr   string // original expression (e.g., "Inputs[first]") — used by g-bind to write back
}

// InputBinding is the reverse lookup: nodeID → field info for input nodes.
type InputBinding struct {
	Field string // struct field name (binding key)
	Expr  string // original expression (e.g., "Inputs[first]")
	Prop  string // "value" or "checked"
}

// ResolveContext holds the state and loop variables available during tree resolution.
type ResolveContext struct {
	State    reflect.Value        // the component struct (or pointer to it)
	Vars     map[string]any       // loop variables: {todo: item, i: index}
	IDs      *IDCounter           // node ID allocator (must persist across renders)
	Bindings map[string][]Binding // field name → bindings (built during resolve)

	// Reverse lookup: nodeID → field for input bindings (g-bind, g-value, g-checked)
	InputBindings map[int]InputBinding

	// Unbound input support
	UnboundValues map[string]any    // stableKey → stored value (passed in from component.Info)
	NodeStableIDs map[int]string    // nodeID → stableKey (built during resolve, read by server)
	ForIndices    []int             // current g-for loop index stack (for composite stable keys)

	// baseEnv is the expr-lang environment built from struct fields + methods.
	// Built once per render on first use, reused for every ResolveExpr call.
	baseEnv map[string]any
}

// addBinding records a dependency from a field expression to a node.
func (ctx *ResolveContext) addBinding(expr string, nodeID int, kind, prop string) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "!") {
		return
	}
	// Skip loop variables
	root := expr
	if dotIdx := strings.Index(root, "."); dotIdx != -1 {
		root = root[:dotIdx]
	}
	if _, isVar := ctx.Vars[root]; isVar {
		return
	}
	// Use the root field name as the binding key
	bindKey := root
	if field, _, ok := ParseMapAccess(expr); ok {
		bindKey = field
	}
	if ctx.Bindings == nil {
		ctx.Bindings = make(map[string][]Binding)
	}
	ctx.Bindings[bindKey] = append(ctx.Bindings[bindKey], Binding{NodeID: nodeID, Kind: kind, Prop: prop, Expr: expr})

	// Build reverse lookup for input bindings (g-bind, g-value, g-checked).
	// Only form-input properties ("value", "checked") participate —
	// arbitrary g-prop: bindings (e.g. scrollTop) are Go→browser only.
	if kind == "bind" || (kind == "prop" && (prop == "value" || prop == "checked")) {
		if ctx.InputBindings == nil {
			ctx.InputBindings = make(map[int]InputBinding)
		}
		ctx.InputBindings[nodeID] = InputBinding{Field: bindKey, Expr: expr, Prop: prop}
	}
}

// ResolveTree resolves a list of template nodes into concrete Nodes.
func ResolveTree(templates []*TemplateNode, ctx *ResolveContext) []Node {
	var nodes []Node
	for _, t := range templates {
		resolved := ResolveTemplateNode(t, ctx)
		nodes = append(nodes, resolved...)
	}
	return MergeAdjacentText(nodes)
}

// MergeAdjacentText collapses consecutive TextNode entries into one and
// removes empty TextNodes.
func MergeAdjacentText(nodes []Node) []Node {
	if len(nodes) == 0 {
		return nodes
	}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		tn, isText := n.(*TextNode)
		if isText {
			if tn.Text == "" {
				continue
			}
			if len(out) > 0 {
				if prev, prevIsText := out[len(out)-1].(*TextNode); prevIsText {
					prev.Text += tn.Text
					continue
				}
			}
		}
		out = append(out, n)
	}
	return out
}

// ResolveTemplateNode resolves a single template node into zero or more Nodes.
func ResolveTemplateNode(t *TemplateNode, ctx *ResolveContext) []Node {
	if t.IsText {
		return resolveTextNode(t, ctx)
	}
	if t.IsFor {
		return resolveForNode(t, ctx)
	}

	for _, d := range t.Directives {
		if d.Type == "if" {
			val := ResolveExpr(d.Expr, ctx)
			if !IsTruthy(val) {
				return nil
			}
		}
	}

	if t.IsSlot {
		return []Node{resolveSlotNode(t, ctx)}
	}

	if t.IsPlugin {
		return []Node{resolvePluginNode(t, ctx)}
	}

	return []Node{resolveElementNode(t, ctx)}
}

// nextID returns the next node ID from the context, or 0 if no counter is set.
func nextID(ctx *ResolveContext) int {
	if ctx.IDs != nil {
		return ctx.IDs.Next()
	}
	return 0
}

func resolveTextNode(t *TemplateNode, ctx *ResolveContext) []Node {
	if len(t.TextParts) == 1 && t.TextParts[0].Static {
		return []Node{&TextNode{NodeBase: NodeBase{ID: nextID(ctx)}, Text: t.TextParts[0].Value}}
	}

	var sb strings.Builder
	var dynamicCount int
	var singleExpr string
	for _, p := range t.TextParts {
		if p.Static {
			sb.WriteString(p.Value)
		} else {
			val := ResolveExpr(p.Value, ctx)
			sb.WriteString(fmt.Sprint(val))
			dynamicCount++
			singleExpr = p.Value
		}
	}
	id := nextID(ctx)
	// Only register a text binding when the node is a single {{field}} with
	// no surrounding static text. Mixed content like "Step size: {{Step}}"
	// can't be surgically patched (the static prefix would be lost), so it
	// relies on the full BuildUpdate/diff path instead.
	if dynamicCount == 1 && len(t.TextParts) == 1 {
		ctx.addBinding(singleExpr, id, "text", "")
	}
	return []Node{&TextNode{NodeBase: NodeBase{ID: id}, Text: sb.String()}}
}

func resolveElementNode(t *TemplateNode, ctx *ResolveContext) Node {
	id := nextID(ctx)
	facts := resolveFacts(t, ctx, id)

	for _, d := range t.Directives {
		if d.Type == "text" {
			el := &ElementNode{
				NodeBase:  NodeBase{ID: id},
				Tag:       t.Tag,
				Namespace: t.Namespace,
				Facts:     facts,
			}
			val := ResolveExpr(d.Expr, ctx)
			text := fmt.Sprint(val)
			textID := nextID(ctx)
			el.Children = []Node{&TextNode{NodeBase: NodeBase{ID: textID}, Text: text}}
			ctx.addBinding(d.Expr, textID, "text", "")
			return el
		}
		if d.Type == "html" {
			val := ResolveExpr(d.Expr, ctx)
			html := fmt.Sprint(val)
			if facts.Props == nil {
				facts.Props = make(map[string]any)
			}
			facts.Props["innerHTML"] = html
			el := &ElementNode{
				NodeBase:  NodeBase{ID: id},
				Tag:       t.Tag,
				Namespace: t.Namespace,
				Facts:     facts,
			}
			ctx.addBinding(d.Expr, id, "prop", "innerHTML")
			return el
		}
	}

	// Check if any child template is a keyed g-for. If so, produce a
	// KeyedElementNode so the diff uses keyed reordering (DOM node moves)
	// instead of positional patching (in-place updates).
	if keyedFor := findKeyedFor(t.Children); keyedFor != nil {
		kel := &KeyedElementNode{
			NodeBase:  NodeBase{ID: id},
			Tag:       t.Tag,
			Namespace: t.Namespace,
			Facts:     facts,
		}
		// Resolve non-for children before the keyed for
		// and after it as regular nodes isn't supported — keyed element
		// expects all children to be keyed. So we resolve only the for.
		kel.Children = resolveKeyedForNode(keyedFor, ctx)
		return kel
	}

	el := &ElementNode{
		NodeBase:  NodeBase{ID: id},
		Tag:       t.Tag,
		Namespace: t.Namespace,
		Facts:     facts,
	}
	el.Children = ResolveTree(t.Children, ctx)
	return el
}

// findKeyedFor returns the first keyed g-for child template, or nil.
func findKeyedFor(children []*TemplateNode) *TemplateNode {
	for _, c := range children {
		if c.IsFor && c.ForKey != "" {
			return c
		}
	}
	return nil
}

// resolveKeyedForNode resolves a g-for with g-key into KeyedChild entries.
func resolveKeyedForNode(t *TemplateNode, ctx *ResolveContext) []KeyedChild {
	listVal := ResolveExpr(t.ForList, ctx)
	rv := reflect.ValueOf(listVal)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return nil
	}

	var children []KeyedChild
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()

		childCtx := &ResolveContext{
			State:         ctx.State,
			Vars:          CopyVars(ctx.Vars),
			IDs:           ctx.IDs,
			UnboundValues: ctx.UnboundValues,
			NodeStableIDs: ctx.NodeStableIDs,
			ForIndices:    append(append([]int{}, ctx.ForIndices...), i),
			baseEnv:       ctx.baseEnv, // share parent's cached base env
		}
		childCtx.Vars[t.ForItem] = item
		if t.ForIndex != "" {
			childCtx.Vars[t.ForIndex] = i
		}

		key := fmt.Sprint(ResolveExpr(t.ForKey, childCtx))

		for _, bodyTmpl := range t.ForBody {
			resolved := ResolveTemplateNode(bodyTmpl, childCtx)
			for _, node := range resolved {
				children = append(children, KeyedChild{Key: key, Node: node})
			}
		}
	}

	return children
}

func resolveForNode(t *TemplateNode, ctx *ResolveContext) []Node {
	listVal := ResolveExpr(t.ForList, ctx)
	rv := reflect.ValueOf(listVal)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return nil
	}

	var nodes []Node
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()

		childCtx := &ResolveContext{
			State:         ctx.State,
			Vars:          CopyVars(ctx.Vars),
			IDs:           ctx.IDs,
			UnboundValues: ctx.UnboundValues,
			NodeStableIDs: ctx.NodeStableIDs,
			ForIndices:    append(append([]int{}, ctx.ForIndices...), i),
			baseEnv:       ctx.baseEnv, // share parent's cached base env
		}
		childCtx.Vars[t.ForItem] = item
		if t.ForIndex != "" {
			childCtx.Vars[t.ForIndex] = i
		}

		for _, bodyTmpl := range t.ForBody {
			resolved := ResolveTemplateNode(bodyTmpl, childCtx)
			nodes = append(nodes, resolved...)
		}
	}

	return nodes
}

func resolveSlotNode(t *TemplateNode, ctx *ResolveContext) Node {
	id := nextID(ctx)

	// Resolve instance name — supports {{expr}} interpolation like all attributes.
	// instance="counter" → "counter", instance="{{slot.Name}}" → resolved value.
	name := resolveAttrValue(t.SlotExpr, ctx)

	// Create a plain div placeholder. Slot metadata lives on the VDOM node
	// (IsSlot/SlotName), not in DOM attributes. The bridge targets this node
	// via its VDOM ID (nodeMap lookup), not via getElementById.
	el := &ElementNode{
		NodeBase: NodeBase{ID: id},
		Tag:      "div",
		IsSlot:   true,
		SlotName: name,
	}
	return el
}

func resolvePluginNode(t *TemplateNode, ctx *ResolveContext) Node {
	id := nextID(ctx)
	data := ResolveExpr(t.PluginExpr, ctx)
	data = DeepCopyJSON(data)
	return &PluginNode{
		NodeBase: NodeBase{ID: id},
		Tag:      t.Tag,
		Name:     t.PluginName,
		Facts:    resolveFacts(t, ctx, id),
		Data:     data,
	}
}

// ---------------------------------------------------------------------------
// Facts resolution
// ---------------------------------------------------------------------------

// resolveAttrValue resolves {{expr}} interpolations in an attribute value.
// If the value contains no interpolations, it is returned as-is.
func resolveAttrValue(val string, ctx *ResolveContext) string {
	if !strings.Contains(val, "{{") {
		return val
	}
	parts := ParseTextInterpolations(val)
	var b strings.Builder
	for _, p := range parts {
		if p.Static {
			b.WriteString(p.Value)
		} else {
			b.WriteString(fmt.Sprint(ResolveExpr(p.Value, ctx)))
		}
	}
	return b.String()
}

func resolveFacts(t *TemplateNode, ctx *ResolveContext, nodeID int) Facts {
	var f Facts

	for _, a := range t.Attrs {
		val := resolveAttrValue(a.Val, ctx)
		if a.Key == "class" || a.Key == "style" || a.Key == "id" {
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			if a.Key == "class" {
				f.Props["className"] = val
			} else {
				f.Props[a.Key] = val
			}
		} else {
			if f.Attrs == nil {
				f.Attrs = make(map[string]string)
			}
			f.Attrs[a.Key] = val
		}
	}

	for _, d := range t.Directives {
		switch d.Type {
		case "text":
			continue
		case "if":
			continue

		case "bind":
			val := ResolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["value"] = fmt.Sprint(val)
			ctx.addBinding(d.Expr, nodeID, "bind", "value")

		case "value":
			val := ResolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["value"] = fmt.Sprint(val)
			ctx.addBinding(d.Expr, nodeID, "prop", "value")

		case "checked":
			val := ResolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["checked"] = IsTruthy(val)
			ctx.addBinding(d.Expr, nodeID, "prop", "checked")

		case "show":
			val := ResolveExpr(d.Expr, ctx)
			if !IsTruthy(val) {
				if f.Styles == nil {
					f.Styles = make(map[string]string)
				}
				f.Styles["display"] = "none"
			}
			ctx.addBinding(d.Expr, nodeID, "show", "")

		case "hide":
			val := ResolveExpr(d.Expr, ctx)
			if IsTruthy(val) {
				if f.Styles == nil {
					f.Styles = make(map[string]string)
				}
				f.Styles["display"] = "none"
			}
			ctx.addBinding(d.Expr, nodeID, "hide", "")

		case "class":
			val := ResolveExpr(d.Expr, ctx)
			if IsTruthy(val) {
				if f.Props == nil {
					f.Props = make(map[string]any)
				}
				existing, _ := f.Props["className"].(string)
				if existing != "" {
					f.Props["className"] = existing + " " + d.Name
				} else {
					f.Props["className"] = d.Name
				}
			}
			ctx.addBinding(d.Expr, nodeID, "class", d.Name)

		case "attr":
			val := ResolveExpr(d.Expr, ctx)
			if f.Attrs == nil {
				f.Attrs = make(map[string]string)
			}
			f.Attrs[d.Name] = fmt.Sprint(val)
			ctx.addBinding(d.Expr, nodeID, "attr", d.Name)

		case "style":
			val := ResolveExpr(d.Expr, ctx)
			if f.Styles == nil {
				f.Styles = make(map[string]string)
			}
			f.Styles[d.Name] = fmt.Sprint(val)
			ctx.addBinding(d.Expr, nodeID, "style", d.Name)

		case "prop":
			val := ResolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props[d.Name] = val
			ctx.addBinding(d.Expr, nodeID, "prop", d.Name)

		case "click", "mousedown", "mousemove", "mouseup", "wheel", "scroll":
			method, args := ParseMethodCall(d.Expr)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			resolvedArgs := resolveArgs(args, ctx)
			f.Events[d.Type] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
			}

		case "drop":
			method, args := ParseMethodCall(d.Expr)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			resolvedArgs := resolveArgs(args, ctx)
			f.Events["drop"] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
			}
			if d.Name != "" {
				if f.Attrs == nil {
					f.Attrs = make(map[string]string)
				}
				f.Attrs["data-drop-group"] = d.Name
			}

		case "keydown":
			method, args := ParseMethodCall(d.Expr)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			resolvedArgs := resolveArgs(args, ctx)
			eventKey := "keydown"
			if d.Name != "" {
				eventKey = "keydown:" + d.Name
			}
			f.Events[eventKey] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
				Options: EventOptions{Key: d.Name},
			}

		case "draggable":
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["draggable"] = true
			if f.Attrs == nil {
				f.Attrs = make(map[string]string)
			}
			val := ResolveExpr(d.Expr, ctx)
			f.Attrs["data-drag-value"] = fmt.Sprint(val)
			if d.Name != "" {
				f.Attrs["data-drag-group"] = d.Name
			}

		case "dropzone":
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			method, args := ParseMethodCall(d.Expr)
			resolvedArgs := resolveArgs(args, ctx)
			f.Events["drop"] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
			}
		}
	}

	// Inject unbound input values: if the template node has a StableID,
	// look up the stored value and set it as Props["value"].
	if t.StableID != "" {
		key := unboundKey(t.StableID, ctx.ForIndices)
		if ctx.UnboundValues != nil {
			if val, ok := ctx.UnboundValues[key]; ok {
				if f.Props == nil {
					f.Props = make(map[string]any)
				}
				f.Props["value"] = fmt.Sprint(val)
			}
		}
		if ctx.NodeStableIDs != nil {
			ctx.NodeStableIDs[nodeID] = key
		}
	}

	return f
}

// isFormInput returns true for tags whose value should be preserved across rebuilds.
func isFormInput(tag string) bool {
	return tag == "input" || tag == "textarea" || tag == "select"
}

// hasBind returns true if any directive is a "bind" directive.
func hasBind(directives []Directive) bool {
	for _, d := range directives {
		if d.Type == "bind" || d.Type == "value" || d.Type == "checked" {
			return true
		}
	}
	return false
}

// genUUID generates a random UUID v4 string.
func genUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// unboundKey computes the storage key for an unbound input.
// Inside g-for loops, it combines the StableID with the loop indices.
func unboundKey(stableID string, forIndices []int) string {
	if len(forIndices) == 0 {
		return stableID
	}
	parts := make([]string, len(forIndices))
	for i, idx := range forIndices {
		parts[i] = fmt.Sprintf("%d", idx)
	}
	return stableID + ":" + strings.Join(parts, ",")
}

// ---------------------------------------------------------------------------
// Expression resolution
// ---------------------------------------------------------------------------

// BuildExprEnv constructs the environment map for expr-lang evaluation.
// It includes all exported struct fields, zero-arg single-return methods,
// and any loop variables from the context.
// buildBaseEnv builds the expr-lang environment from struct fields and methods.
// Called once per render and cached on the ResolveContext.
func buildBaseEnv(ctx *ResolveContext) map[string]any {
	env := make(map[string]any)

	v := ctx.State
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			break
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.IsExported() {
				fv := v.Field(i)
				for fv.Kind() == reflect.Ptr {
					if fv.IsNil() {
						break
					}
					fv = fv.Elem()
				}
				if fv.IsValid() {
					env[f.Name] = fv.Interface()
				}
			}
		}

		// Add methods as callable functions.
		// Methods are bound to the receiver pointer, so they see current state.
		sv := ctx.State
		st := sv.Type()
		for i := 0; i < st.NumMethod(); i++ {
			m := st.Method(i)
			name := m.Name
			if _, exists := env[name]; !exists {
				method := sv.Method(i)
				env[name] = method.Interface()
			}
		}
	}

	return env
}

// BuildExprEnv returns the expr-lang environment for the current context.
// The base env (struct fields + methods) is built once per render and cached.
// Loop variables are merged on top each call.
func BuildExprEnv(ctx *ResolveContext) map[string]any {
	if ctx.baseEnv == nil {
		ctx.baseEnv = buildBaseEnv(ctx)
	}

	// No loop variables — return base env directly (hot path for non-loop expressions)
	if len(ctx.Vars) == 0 {
		return ctx.baseEnv
	}

	// Merge loop variables on top of base env
	env := make(map[string]any, len(ctx.baseEnv)+len(ctx.Vars))
	for k, v := range ctx.baseEnv {
		env[k] = v
	}
	for k, v := range ctx.Vars {
		env[k] = v
	}
	return env
}

// exprCache caches compiled expr-lang programs keyed by expression string.
// Expression strings are fixed at template parse time, so each unique expression
// is compiled once and reused on every render.
var exprCache sync.Map // map[string]*vm.Program

// isSimpleExpr returns true for expressions that can be resolved with direct
// reflection instead of expr-lang: identifiers ("Score"), dotted paths
// ("brick.Left"), and negated versions ("!Visible"). These make up the vast
// majority of expressions in typical templates and are much faster to resolve.
func isSimpleExpr(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '!' {
			continue
		}
		return false
	}
	return len(s) > 0
}

// resolveSimpleExpr resolves a simple field/variable expression using direct
// reflection. Returns (value, true) on success, (nil, false) if the expression
// can't be resolved on the fast path.
func resolveSimpleExpr(exprStr string, ctx *ResolveContext) (any, bool) {
	negated := false
	if exprStr[0] == '!' {
		negated = true
		exprStr = exprStr[1:]
		if exprStr == "" {
			return nil, false
		}
	}

	var val any
	root := exprStr
	if dotIdx := strings.IndexByte(exprStr, '.'); dotIdx != -1 {
		root = exprStr[:dotIdx]
	}

	// Check loop variables first (they shadow struct fields)
	if ctx.Vars != nil {
		if lv, exists := ctx.Vars[root]; exists {
			if root == exprStr {
				val = lv
			} else {
				val = resolveFieldPath(lv, exprStr[len(root)+1:])
			}
			if negated {
				return !IsTruthy(val), true
			}
			return val, true
		}
	}

	// Try struct field
	result := resolveStructField(ctx.State, exprStr)
	if result != nil {
		if negated {
			return !IsTruthy(result), true
		}
		return result, true
	}

	// Not found — let caller decide (may be a method call, literal, etc.)
	return nil, false
}

// ResolveExpr resolves an expression string against the context.
// Simple field/variable access uses direct reflection (fast path).
// Complex expressions (comparisons, operators, function calls) use expr-lang.
func ResolveExpr(exprStr string, ctx *ResolveContext) any {
	exprStr = strings.TrimSpace(exprStr)
	if exprStr == "" {
		return nil
	}

	// Fast path: simple field or dotted path (e.g. "Score", "brick.Left", "!Visible").
	// This covers the vast majority of expressions and avoids expr-lang overhead.
	if isSimpleExpr(exprStr) {
		if val, ok := resolveSimpleExpr(exprStr, ctx); ok {
			return val
		}
	}

	// Fast path: zero-arg method call like "ComputedName()" or "!ShowSaved()".
	inner := exprStr
	methodNegated := false
	if inner[0] == '!' {
		methodNegated = true
		inner = inner[1:]
	}
	if len(inner) > 2 && inner[len(inner)-2:] == "()" && isSimpleExpr(inner[:len(inner)-2]) {
		name := inner[:len(inner)-2]
		if result := callMethod(ctx.State, name); result != nil {
			if methodNegated {
				return !IsTruthy(result)
			}
			return result
		}
	}

	// Convert single-quoted strings to double-quoted for expr-lang.
	exprStr = convertQuotes(exprStr)

	// Handle bracket map access: "Field[key]" — expr-lang uses different syntax
	// so we resolve these directly.
	if field, key, ok := ParseMapAccess(exprStr); ok {
		val := resolveStructField(ctx.State, field+"["+key+"]")
		if val != nil {
			return val
		}
		if ctx.Vars != nil {
			if lv, exists := ctx.Vars[field]; exists {
				rv := reflect.ValueOf(lv)
				for rv.Kind() == reflect.Ptr {
					rv = rv.Elem()
				}
				if rv.Kind() == reflect.Map {
					mapVal := rv.MapIndex(reflect.ValueOf(key))
					if mapVal.IsValid() {
						return mapVal.Interface()
					}
				}
			}
		}
		return ""
	}

	env := BuildExprEnv(ctx)

	// Look up cached compiled program, or compile and cache on first use.
	var program *vm.Program
	if cached, ok := exprCache.Load(exprStr); ok {
		program = cached.(*vm.Program)
	} else {
		compiled, err := expr.Compile(exprStr, expr.Env(env), expr.AllowUndefinedVariables())
		if err != nil {
			return nil
		}
		program = compiled
		exprCache.Store(exprStr, program)
	}

	result, err := expr.Run(program, env)
	if err != nil {
		return nil
	}
	return result
}

// convertQuotes converts single-quoted strings to double-quoted for expr-lang.
// 'hello' → "hello", but leaves strings without matching quotes unchanged.
func convertQuotes(s string) string {
	// Simple case: entire expression is a single-quoted string
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' && !strings.Contains(s[1:len(s)-1], "'") {
		return "\"" + s[1:len(s)-1] + "\""
	}
	// Complex case: expression contains single-quoted strings mixed with operators
	// e.g., "Status == 'active'" → "Status == \"active\""
	if strings.Contains(s, "'") {
		var b strings.Builder
		inQuote := false
		for i := 0; i < len(s); i++ {
			if s[i] == '\'' {
				b.WriteByte('"')
				inQuote = !inQuote
			} else {
				b.WriteByte(s[i])
			}
		}
		return b.String()
	}
	return s
}

// callMethod calls a zero-arg, single-return method on v by name.
// Returns nil if the method doesn't exist or has the wrong signature.
func callMethod(v reflect.Value, name string) any {
	m := v.MethodByName(name)
	if !m.IsValid() {
		return nil
	}
	mt := m.Type()
	if mt.NumIn() != 0 || mt.NumOut() != 1 {
		return nil
	}
	return m.Call(nil)[0].Interface()
}

// callMethodWithArgs calls a method with pre-resolved arguments and returns its result.
// Returns nil if the method doesn't exist or has no return value.
func callMethodWithArgs(v reflect.Value, name string, args []any) any {
	m := v.MethodByName(name)
	if !m.IsValid() {
		return nil
	}
	mt := m.Type()
	if mt.NumOut() != 1 {
		return nil
	}
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		if a == nil {
			in[i] = reflect.Zero(mt.In(i))
		} else {
			in[i] = reflect.ValueOf(a)
		}
	}
	return m.Call(in)[0].Interface()
}

// ParseMapAccess parses "Field[key]" into ("Field", "key", true).
// Returns ("", "", false) if the expression doesn't match bracket syntax.
func ParseMapAccess(expr string) (field, key string, ok bool) {
	bracketIdx := strings.Index(expr, "[")
	if bracketIdx == -1 || !strings.HasSuffix(expr, "]") {
		return "", "", false
	}
	field = expr[:bracketIdx]
	key = expr[bracketIdx+1 : len(expr)-1]
	return field, key, true
}

func resolveStructField(v reflect.Value, path string) any {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	// Handle bracket map access: "Inputs[first]"
	if field, key, ok := ParseMapAccess(path); ok {
		if v.Kind() != reflect.Struct {
			return nil
		}
		fv := v.FieldByName(field)
		if !fv.IsValid() {
			return nil
		}
		for fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				return nil
			}
			fv = fv.Elem()
		}
		if fv.Kind() != reflect.Map {
			return nil
		}
		mapVal := fv.MapIndex(reflect.ValueOf(key))
		if !mapVal.IsValid() {
			return ""
		}
		return mapVal.Interface()
	}

	parts := strings.Split(path, ".")
	for _, part := range parts {
		if v.Kind() != reflect.Struct {
			return nil
		}
		v = v.FieldByName(part)
		if !v.IsValid() {
			return nil
		}
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
	}
	return v.Interface()
}

func resolveFieldPath(val any, path string) any {
	v := reflect.ValueOf(val)
	return resolveStructField(v, path)
}

func resolveArgs(argExprs []string, ctx *ResolveContext) []any {
	if len(argExprs) == 0 {
		return nil
	}
	args := make([]any, len(argExprs))
	for i, expr := range argExprs {
		args[i] = ResolveExpr(expr, ctx)
	}
	return args
}

// IsValidIdentifier checks that a name looks like a Go/JS identifier:
// letters, digits, underscores; cannot start with a digit.
func IsValidIdentifier(name string) bool {
	for i, c := range name {
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			continue
		}
		if c >= '0' && c <= '9' && i > 0 {
			continue
		}
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Method call parsing
// ---------------------------------------------------------------------------

// ParseMethodCall parses "Save", "Toggle(i)", "Remove(i, todo.ID)".
func ParseMethodCall(expr string) (method string, args []string) {
	expr = strings.TrimSpace(expr)
	parenIdx := strings.Index(expr, "(")
	if parenIdx == -1 {
		return expr, nil
	}
	method = expr[:parenIdx]
	argStr := strings.TrimSuffix(expr[parenIdx+1:], ")")
	if argStr == "" {
		return method, nil
	}
	for _, a := range strings.Split(argStr, ",") {
		args = append(args, strings.TrimSpace(a))
	}
	return method, args
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// IsTruthy returns whether a value is considered true for g-if/g-show/g-class.
func IsTruthy(val any) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	default:
		rv := reflect.ValueOf(val)
		switch rv.Kind() {
		case reflect.Slice, reflect.Map:
			return rv.Len() > 0
		default:
			return true
		}
	}
}

// kebabToCamel converts kebab-case to camelCase (e.g. "scroll-top" → "scrollTop").
// HTML parsers lowercase attribute names, so g-prop:scrollTop arrives as g-prop:scrolltop.
// Authors write g-prop:scroll-top and this converts it to the correct DOM property name.
func kebabToCamel(s string) string {
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// CopyVars creates a shallow copy of a variable map.
func CopyVars(vars map[string]any) map[string]any {
	if vars == nil {
		return make(map[string]any)
	}
	cp := make(map[string]any, len(vars))
	for k, v := range vars {
		cp[k] = v
	}
	return cp
}

func getAttrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// DeepCopyJSON deep-copies a value by JSON round-tripping.
func DeepCopyJSON(v any) any {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return v
	}
	return out
}

func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findBody(c); found != nil {
			return found
		}
	}
	return nil
}

// gSlotSelfCloseRe matches self-closing <g-slot .../> tags.
var gSlotSelfCloseRe = regexp.MustCompile(`<g-slot\b([^>]*?)/>`)

// expandSelfClosingGSlot rewrites <g-slot .../> → <g-slot ...></g-slot>
// so Go's html.Parse treats them as proper sibling elements instead of
// nesting subsequent tags inside the first unclosed g-slot.
func expandSelfClosingGSlot(s string) string {
	return gSlotSelfCloseRe.ReplaceAllString(s, `<g-slot$1></g-slot>`)
}
