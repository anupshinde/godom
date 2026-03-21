package vdom

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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

	// For component nodes
	IsComponent  bool
	ComponentTag string            // custom element name
	PropExprs    map[string]string // prop name → expression (from :prop="expr")

	// For plugin nodes
	IsPlugin   bool
	PluginName string // plugin name from g-plugin:name
	PluginExpr string // data expression

	// StableID is a UUID assigned at parse time to unbound form inputs.
	// Used to preserve input values across tree rebuilds.
	StableID string
}

// Directive represents a single g-* directive on an element.
type Directive struct {
	Type string // "text", "bind", "value", "checked", "if", "show", "hide", "class", "attr", "style",
	           // "click", "keydown", "mousedown", "mousemove", "mouseup", "wheel", "drop",
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
// componentTags is the set of registered custom element names.
func ParseTemplate(htmlStr string, componentTags map[string]bool) ([]*TemplateNode, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	body := findBody(doc)
	if body == nil {
		return nil, fmt.Errorf("no <body> found in HTML")
	}

	var nodes []*TemplateNode
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if tn := htmlToTemplate(c, componentTags); tn != nil {
			nodes = append(nodes, tn)
		}
	}
	return nodes, nil
}

func htmlToTemplate(n *html.Node, componentTags map[string]bool) *TemplateNode {
	switch n.Type {
	case html.TextNode:
		text := n.Data
		if strings.TrimSpace(text) == "" {
			return &TemplateNode{IsText: true, TextParts: []TextPart{{Static: true, Value: text}}}
		}
		return &TemplateNode{IsText: true, TextParts: ParseTextInterpolations(text)}

	case html.ElementNode:
		return htmlElementToTemplate(n, componentTags)

	case html.CommentNode:
		return nil

	default:
		return nil
	}
}

func htmlElementToTemplate(n *html.Node, componentTags map[string]bool) *TemplateNode {
	tag := n.Data

	if forExpr := getAttrVal(n, "g-for"); forExpr != "" {
		return parseForTemplate(n, forExpr, componentTags)
	}

	if componentTags[tag] {
		return parseComponentTemplate(n, tag, componentTags)
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
		if child := htmlToTemplate(c, componentTags); child != nil {
			if tn.Namespace != "" && !child.IsText {
				child.Namespace = tn.Namespace
			}
			tn.Children = append(tn.Children, child)
		}
	}

	return tn
}

func parseForTemplate(n *html.Node, forExpr string, componentTags map[string]bool) *TemplateNode {
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

	if componentTags[n.Data] {
		itemTemplate.IsComponent = true
		itemTemplate.ComponentTag = n.Data
		itemTemplate.PropExprs = extractPropExprs(n)
	}

	if n.Data == "svg" || n.Namespace == "svg" {
		itemTemplate.Namespace = "http://www.w3.org/2000/svg"
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := htmlToTemplate(c, componentTags); child != nil {
			if itemTemplate.Namespace != "" && !child.IsText {
				child.Namespace = itemTemplate.Namespace
			}
			itemTemplate.Children = append(itemTemplate.Children, child)
		}
	}

	tn.ForBody = []*TemplateNode{itemTemplate}
	return tn
}

func parseComponentTemplate(n *html.Node, tag string, componentTags map[string]bool) *TemplateNode {
	tn := &TemplateNode{
		Tag:          tag,
		IsComponent:  true,
		ComponentTag: tag,
		PropExprs:    extractPropExprs(n),
	}
	tn.Attrs, tn.Directives = extractAttrsAndDirectives(n)

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := htmlToTemplate(c, componentTags); child != nil {
			tn.Children = append(tn.Children, child)
		}
	}
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
		case a.Key == "g-drop":
			dirs = append(dirs, Directive{Type: "drop", Expr: a.Val})

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

func extractPropExprs(n *html.Node) map[string]string {
	props := make(map[string]string)
	for _, a := range n.Attr {
		if strings.HasPrefix(a.Key, ":") {
			props[a.Key[1:]] = a.Val
		}
	}
	if len(props) == 0 {
		return nil
	}
	return props
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

// ResolveContext holds the state and loop variables available during tree resolution.
type ResolveContext struct {
	State    reflect.Value        // the component struct (or pointer to it)
	Vars     map[string]any       // loop variables: {todo: item, i: index}
	IDs      *IDCounter           // node ID allocator (must persist across renders)
	Bindings map[string][]Binding // field name → bindings (built during resolve)

	// Unbound input support
	UnboundValues map[string]any    // stableKey → stored value (passed in from component.Info)
	NodeStableIDs map[int]string    // nodeID → stableKey (built during resolve, read by server)
	ForIndices    []int             // current g-for loop index stack (for composite stable keys)
}

// addBinding records a dependency from a field expression to a node.
func (ctx *ResolveContext) addBinding(expr string, nodeID int, kind, prop string) {
	// Only record bindings for simple field names (not loop vars, not negated, not dotted)
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "!") || strings.Contains(expr, ".") {
		return
	}
	if _, isVar := ctx.Vars[expr]; isVar {
		return
	}
	// For bracket expressions like "Inputs[first]", bind to the field name "Inputs"
	bindKey := expr
	if field, _, ok := ParseMapAccess(expr); ok {
		bindKey = field
	}
	if ctx.Bindings == nil {
		ctx.Bindings = make(map[string][]Binding)
	}
	ctx.Bindings[bindKey] = append(ctx.Bindings[bindKey], Binding{NodeID: nodeID, Kind: kind, Prop: prop, Expr: expr})
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

	if t.IsPlugin {
		return []Node{resolvePluginNode(t, ctx)}
	}
	if t.IsComponent {
		return []Node{resolveComponentNode(t, ctx)}
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
	for _, p := range t.TextParts {
		if p.Static {
			sb.WriteString(p.Value)
		} else {
			val := ResolveExpr(p.Value, ctx)
			sb.WriteString(fmt.Sprint(val))
		}
	}
	return []Node{&TextNode{NodeBase: NodeBase{ID: nextID(ctx)}, Text: sb.String()}}
}

func resolveElementNode(t *TemplateNode, ctx *ResolveContext) Node {
	id := nextID(ctx)
	el := &ElementNode{
		NodeBase:  NodeBase{ID: id},
		Tag:       t.Tag,
		Namespace: t.Namespace,
		Facts:     resolveFacts(t, ctx, id),
	}

	for _, d := range t.Directives {
		if d.Type == "text" {
			val := ResolveExpr(d.Expr, ctx)
			text := fmt.Sprint(val)
			textID := nextID(ctx)
			if text != "" {
				el.Children = []Node{&TextNode{NodeBase: NodeBase{ID: textID}, Text: text}}
			}
			ctx.addBinding(d.Expr, textID, "text", "")
			return el
		}
	}

	el.Children = ResolveTree(t.Children, ctx)
	return el
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

func resolveComponentNode(t *TemplateNode, ctx *ResolveContext) Node {
	props := make(map[string]any)
	for name, expr := range t.PropExprs {
		props[name] = ResolveExpr(expr, ctx)
	}
	return &ComponentNode{
		NodeBase: NodeBase{ID: nextID(ctx)},
		Tag:      t.ComponentTag,
		Props:    props,
	}
}

// ---------------------------------------------------------------------------
// Facts resolution
// ---------------------------------------------------------------------------

func resolveFacts(t *TemplateNode, ctx *ResolveContext, nodeID int) Facts {
	var f Facts

	for _, a := range t.Attrs {
		if a.Key == "class" || a.Key == "style" || a.Key == "id" {
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			if a.Key == "class" {
				f.Props["className"] = a.Val
			} else {
				f.Props[a.Key] = a.Val
			}
		} else {
			if f.Attrs == nil {
				f.Attrs = make(map[string]string)
			}
			f.Attrs[a.Key] = a.Val
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

		case "click", "mousedown", "mousemove", "mouseup", "wheel", "drop":
			method, args := ParseMethodCall(d.Expr)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			resolvedArgs := resolveArgs(args, ctx)
			f.Events[d.Type] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
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
		if d.Type == "bind" {
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

// ResolveExpr resolves an expression string against the context.
// It tries fields first, then zero-arg single-return methods.
// Supports leading ! for boolean negation.
func ResolveExpr(expr string, ctx *ResolveContext) any {
	expr = strings.TrimSpace(expr)

	// Boolean negation: !Expr
	if strings.HasPrefix(expr, "!") {
		inner := strings.TrimSpace(expr[1:])
		val := ResolveExpr(inner, ctx)
		return !IsTruthy(val)
	}

	if expr == "true" {
		return true
	}
	if expr == "false" {
		return false
	}

	// Method call with args: Method(arg1, arg2)
	if strings.Contains(expr, "(") {
		method, argExprs := ParseMethodCall(expr)
		resolved := resolveArgs(argExprs, ctx)
		return callMethodWithArgs(ctx.State, method, resolved)
	}

	if ctx.Vars != nil {
		if val, ok := ctx.Vars[expr]; ok {
			return val
		}
		if dotIdx := strings.Index(expr, "."); dotIdx != -1 {
			root := expr[:dotIdx]
			if val, ok := ctx.Vars[root]; ok {
				return resolveFieldPath(val, expr[dotIdx+1:])
			}
		}
	}

	// Try field first, then method.
	if val := resolveStructField(ctx.State, expr); val != nil {
		return val
	}
	return callMethod(ctx.State, expr)
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
