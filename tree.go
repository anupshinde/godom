package godom

import (
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ---------------------------------------------------------------------------
// Template tree — parsed once at Mount() time, reused on every render.
// ---------------------------------------------------------------------------

// templateNode represents one node in the parsed template tree.
// It mirrors the HTML structure but carries directive metadata.
type templateNode struct {
	// For element nodes
	Tag       string
	Namespace string
	Attrs     []html.Attribute // static HTML attributes (non-directive)

	// Directives (extracted from g-* attributes)
	Directives []directive

	// Children (for elements) or nil (for text)
	Children []*templateNode

	// For text nodes
	IsText   bool
	TextParts []textPart // static text + {expr} interpolations

	// For g-for nodes
	IsFor     bool
	ForItem   string // loop variable name, e.g. "todo"
	ForIndex  string // index variable name, e.g. "i" (empty if unused)
	ForList   string // list field, e.g. "Todos"
	ForKey    string // key expression, e.g. "todo.ID" (empty = positional)
	ForBody   []*templateNode // template for each item

	// For component nodes
	IsComponent  bool
	ComponentTag string            // custom element name
	PropExprs    map[string]string // prop name → expression (from :prop="expr")

	// For plugin nodes
	IsPlugin   bool
	PluginName string // plugin name from g-plugin:name
	PluginExpr string // data expression
}

// directive represents a single g-* directive on an element.
type directive struct {
	Type string // "text", "bind", "checked", "if", "show", "class", "attr", "style",
	           // "click", "keydown", "mousedown", "mousemove", "mouseup", "wheel", "drop",
	           // "draggable", "dropzone"
	Name string // modifier name: class name, attr name, style property, key filter, etc.
	Expr string // expression: field name, method call, etc.
}

// textPart represents a segment of text content.
// Either static text or an {expression} interpolation.
type textPart struct {
	Static bool
	Value  string // literal text if Static, expression string if not
}

// ---------------------------------------------------------------------------
// HTML → Template tree parser
// ---------------------------------------------------------------------------

// parseTemplate parses HTML into a template tree.
// componentTags is the set of registered custom element names.
func parseTemplate(htmlStr string, componentTags map[string]bool) ([]*templateNode, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	// Find the body element — template content lives there
	body := findBody(doc)
	if body == nil {
		return nil, fmt.Errorf("no <body> found in HTML")
	}

	var nodes []*templateNode
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if tn := htmlToTemplate(c, componentTags); tn != nil {
			nodes = append(nodes, tn)
		}
	}
	return nodes, nil
}

// htmlToTemplate converts an x/net/html node into a templateNode.
func htmlToTemplate(n *html.Node, componentTags map[string]bool) *templateNode {
	switch n.Type {
	case html.TextNode:
		text := n.Data
		if strings.TrimSpace(text) == "" {
			// Preserve whitespace-only text nodes for formatting
			return &templateNode{IsText: true, TextParts: []textPart{{Static: true, Value: text}}}
		}
		return &templateNode{IsText: true, TextParts: parseTextInterpolations(text)}

	case html.ElementNode:
		return htmlElementToTemplate(n, componentTags)

	case html.CommentNode:
		// Skip comments in template
		return nil

	default:
		return nil
	}
}

// htmlElementToTemplate converts an HTML element into a templateNode.
func htmlElementToTemplate(n *html.Node, componentTags map[string]bool) *templateNode {
	tag := n.Data // tag name

	// Check for g-for — produces a loop template node
	if forExpr := getAttrVal(n, "g-for"); forExpr != "" {
		return parseForTemplate(n, forExpr, componentTags)
	}

	// Check for registered component
	if componentTags[tag] {
		return parseComponentTemplate(n, tag, componentTags)
	}

	// Check for plugin
	pluginName, pluginExpr := extractPluginDirective(n)
	if pluginName != "" {
		tn := &templateNode{
			Tag:        tag,
			IsPlugin:   true,
			PluginName: pluginName,
			PluginExpr: pluginExpr,
		}
		tn.Attrs, tn.Directives = extractAttrsAndDirectives(n)
		return tn
	}

	// Regular element
	tn := &templateNode{Tag: tag}
	tn.Attrs, tn.Directives = extractAttrsAndDirectives(n)

	// Detect SVG namespace
	if tag == "svg" || n.Namespace == "svg" {
		tn.Namespace = "http://www.w3.org/2000/svg"
	}

	// Recurse into children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := htmlToTemplate(c, componentTags); child != nil {
			// Propagate SVG namespace to children
			if tn.Namespace != "" && !child.IsText {
				child.Namespace = tn.Namespace
			}
			tn.Children = append(tn.Children, child)
		}
	}

	return tn
}

// parseForTemplate extracts a g-for loop node.
func parseForTemplate(n *html.Node, forExpr string, componentTags map[string]bool) *templateNode {
	item, index, list := parseForExpr(forExpr)

	keyExpr := getAttrVal(n, "g-key")

	tn := &templateNode{
		IsFor:    true,
		ForItem:  item,
		ForIndex: index,
		ForList:  list,
		ForKey:   keyExpr,
		Tag:      n.Data,
	}

	// The g-for element itself becomes the per-item template.
	// Remove g-for and g-key attrs, keep everything else.
	itemTemplate := &templateNode{Tag: n.Data}
	itemTemplate.Attrs, itemTemplate.Directives = extractAttrsAndDirectives(n)

	// Check if this is also a component
	if componentTags[n.Data] {
		itemTemplate.IsComponent = true
		itemTemplate.ComponentTag = n.Data
		itemTemplate.PropExprs = extractPropExprs(n)
	}

	// Detect SVG namespace on the item template
	if n.Data == "svg" || n.Namespace == "svg" {
		itemTemplate.Namespace = "http://www.w3.org/2000/svg"
	}

	// Recurse into children for the item template
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if child := htmlToTemplate(c, componentTags); child != nil {
			if itemTemplate.Namespace != "" && !child.IsText {
				child.Namespace = itemTemplate.Namespace
			}
			itemTemplate.Children = append(itemTemplate.Children, child)
		}
	}

	tn.ForBody = []*templateNode{itemTemplate}
	return tn
}

// parseComponentTemplate extracts a component node.
func parseComponentTemplate(n *html.Node, tag string, componentTags map[string]bool) *templateNode {
	tn := &templateNode{
		Tag:          tag,
		IsComponent:  true,
		ComponentTag: tag,
		PropExprs:    extractPropExprs(n),
	}
	tn.Attrs, tn.Directives = extractAttrsAndDirectives(n)

	// Components may have slot content (children)
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

// extractAttrsAndDirectives separates g-* directives from static HTML attributes.
// Removes g-for, g-key, :prop attributes (handled elsewhere).
func extractAttrsAndDirectives(n *html.Node) ([]html.Attribute, []directive) {
	var attrs []html.Attribute
	var dirs []directive

	for _, a := range n.Attr {
		switch {
		// Skip directives handled at a higher level
		case a.Key == "g-for", a.Key == "g-key":
			continue
		case strings.HasPrefix(a.Key, ":"):
			continue // prop expressions handled by extractPropExprs

		// Data binding directives
		case a.Key == "g-text":
			dirs = append(dirs, directive{Type: "text", Expr: a.Val})
		case a.Key == "g-bind":
			dirs = append(dirs, directive{Type: "bind", Expr: a.Val})
		case a.Key == "g-checked":
			dirs = append(dirs, directive{Type: "checked", Expr: a.Val})
		case a.Key == "g-if":
			dirs = append(dirs, directive{Type: "if", Expr: a.Val})
		case a.Key == "g-show":
			dirs = append(dirs, directive{Type: "show", Expr: a.Val})

		// Class/attr/style bindings (Vue-style modifier syntax)
		case strings.HasPrefix(a.Key, "g-class:"):
			dirs = append(dirs, directive{Type: "class", Name: a.Key[len("g-class:"):], Expr: a.Val})
		case strings.HasPrefix(a.Key, "g-attr:"):
			dirs = append(dirs, directive{Type: "attr", Name: a.Key[len("g-attr:"):], Expr: a.Val})
		case strings.HasPrefix(a.Key, "g-style:"):
			dirs = append(dirs, directive{Type: "style", Name: a.Key[len("g-style:"):], Expr: a.Val})

		// Event directives
		case a.Key == "g-click":
			dirs = append(dirs, directive{Type: "click", Expr: a.Val})
		case a.Key == "g-keydown":
			// Multiple bindings: "ArrowUp:PanUp;ArrowDown:PanDown"
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
				dirs = append(dirs, directive{Type: "keydown", Name: key, Expr: method})
			}
		case a.Key == "g-mousedown":
			dirs = append(dirs, directive{Type: "mousedown", Expr: a.Val})
		case a.Key == "g-mousemove":
			dirs = append(dirs, directive{Type: "mousemove", Expr: a.Val})
		case a.Key == "g-mouseup":
			dirs = append(dirs, directive{Type: "mouseup", Expr: a.Val})
		case a.Key == "g-wheel":
			dirs = append(dirs, directive{Type: "wheel", Expr: a.Val})
		case a.Key == "g-drop":
			dirs = append(dirs, directive{Type: "drop", Expr: a.Val})

		// Drag & drop
		case a.Key == "g-draggable" || strings.HasPrefix(a.Key, "g-draggable:"):
			group := ""
			if strings.HasPrefix(a.Key, "g-draggable:") {
				group = a.Key[len("g-draggable:"):]
			}
			dirs = append(dirs, directive{Type: "draggable", Name: group, Expr: a.Val})
		case a.Key == "g-dropzone":
			dirs = append(dirs, directive{Type: "dropzone", Expr: a.Val})

		// Plugin (also handled at element level, but capture directive for facts)
		case strings.HasPrefix(a.Key, "g-plugin:"):
			// Already handled by extractPluginDirective; skip here
			continue

		// Regular HTML attribute
		default:
			attrs = append(attrs, a)
		}
	}

	return attrs, dirs
}

// extractPropExprs extracts :prop="expr" bindings from a component element.
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

// extractPluginDirective looks for a g-plugin:name attribute.
func extractPluginDirective(n *html.Node) (name, expr string) {
	for _, a := range n.Attr {
		if strings.HasPrefix(a.Key, "g-plugin:") {
			return a.Key[len("g-plugin:"):], a.Val
		}
	}
	return "", ""
}

// ---------------------------------------------------------------------------
// Text interpolation: "Hello {{Name}}, you have {{Count}} items"
// ---------------------------------------------------------------------------

// parseTextInterpolations splits text containing {{expr}} into parts.
func parseTextInterpolations(text string) []textPart {
	var parts []textPart
	for {
		start := strings.Index(text, "{{")
		if start == -1 {
			if text != "" {
				parts = append(parts, textPart{Static: true, Value: text})
			}
			break
		}
		end := strings.Index(text[start:], "}}")
		if end == -1 {
			// No closing braces — treat rest as static
			parts = append(parts, textPart{Static: true, Value: text})
			break
		}
		end += start // adjust to absolute position

		// Static text before the expression
		if start > 0 {
			parts = append(parts, textPart{Static: true, Value: text[:start]})
		}
		// The expression (inside the double braces)
		expr := strings.TrimSpace(text[start+2 : end])
		if expr != "" {
			parts = append(parts, textPart{Static: false, Value: expr})
		}
		text = text[end+2:]
	}
	if len(parts) == 0 {
		return []textPart{{Static: true, Value: ""}}
	}
	return parts
}

// ---------------------------------------------------------------------------
// g-for expression parsing: "todo in Todos", "todo, i in Todos"
// ---------------------------------------------------------------------------

func parseForExpr(expr string) (item, index, list string) {
	// "todo, i in Todos" or "todo in Todos"
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

// resolveContext holds the state and loop variables available during tree resolution.
type resolveContext struct {
	State reflect.Value         // the component struct (or pointer to it)
	Vars  map[string]any        // loop variables: {todo: item, i: index}
}

// resolveTree resolves a list of template nodes into concrete Nodes.
func resolveTree(templates []*templateNode, ctx *resolveContext) []Node {
	var nodes []Node
	for _, t := range templates {
		resolved := resolveTemplateNode(t, ctx)
		nodes = append(nodes, resolved...)
	}
	return nodes
}

// resolveTemplateNode resolves a single template node into zero or more Nodes.
// May return zero nodes (g-if=false) or many (g-for expansion).
func resolveTemplateNode(t *templateNode, ctx *resolveContext) []Node {
	if t.IsText {
		return resolveTextNode(t, ctx)
	}
	if t.IsFor {
		return resolveForNode(t, ctx)
	}

	// Check g-if before building the element
	for _, d := range t.Directives {
		if d.Type == "if" {
			val := resolveExpr(d.Expr, ctx)
			if !isTruthy(val) {
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

// resolveTextNode resolves text interpolations.
func resolveTextNode(t *templateNode, ctx *resolveContext) []Node {
	// Optimize: single static part → single TextNode
	if len(t.TextParts) == 1 && t.TextParts[0].Static {
		return []Node{&TextNode{Text: t.TextParts[0].Value}}
	}

	// Multiple parts or dynamic: concatenate into one text node
	var sb strings.Builder
	for _, p := range t.TextParts {
		if p.Static {
			sb.WriteString(p.Value)
		} else {
			val := resolveExpr(p.Value, ctx)
			sb.WriteString(fmt.Sprint(val))
		}
	}
	return []Node{&TextNode{Text: sb.String()}}
}

// resolveElementNode resolves a regular element.
func resolveElementNode(t *templateNode, ctx *resolveContext) Node {
	el := &ElementNode{
		Tag:       t.Tag,
		Namespace: t.Namespace,
		Facts:     resolveFacts(t, ctx),
	}

	// Resolve children
	// If g-text directive exists, it replaces all children with a single text node
	for _, d := range t.Directives {
		if d.Type == "text" {
			val := resolveExpr(d.Expr, ctx)
			el.Children = []Node{&TextNode{Text: fmt.Sprint(val)}}
			return el
		}
	}

	el.Children = resolveTree(t.Children, ctx)
	return el
}

// resolveForNode expands a g-for into children.
func resolveForNode(t *templateNode, ctx *resolveContext) []Node {
	listVal := resolveExpr(t.ForList, ctx)
	rv := reflect.ValueOf(listVal)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return nil
	}

	hasKey := t.ForKey != ""
	if hasKey {
		// Produce a KeyedElementNode — the parent receives keyed children
		// We don't have the parent tag here; the caller wraps this.
		// For now, return individual keyed children that the parent collects.
		// Actually, g-for replaces itself with its expanded items.
		// The keyed case needs special handling at the parent level.
		// For simplicity: return a slice of nodes. If keyed, the parent
		// should be a KeyedElementNode — but that's decided by the parent.
		// TODO: revisit when integrating with parent element resolution.
	}

	var nodes []Node
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()

		// Build child context with loop variables
		childCtx := &resolveContext{
			State: ctx.State,
			Vars:  copyVars(ctx.Vars),
		}
		childCtx.Vars[t.ForItem] = item
		if t.ForIndex != "" {
			childCtx.Vars[t.ForIndex] = i
		}

		// Resolve the item template (ForBody is the per-item template)
		for _, bodyTmpl := range t.ForBody {
			resolved := resolveTemplateNode(bodyTmpl, childCtx)
			nodes = append(nodes, resolved...)
		}
	}

	return nodes
}

// resolvePluginNode resolves a plugin element.
func resolvePluginNode(t *templateNode, ctx *resolveContext) Node {
	data := resolveExpr(t.PluginExpr, ctx)
	return &PluginNode{
		Tag:   t.Tag,
		Name:  t.PluginName,
		Facts: resolveFacts(t, ctx),
		Data:  data,
	}
}

// resolveComponentNode resolves a component element.
func resolveComponentNode(t *templateNode, ctx *resolveContext) Node {
	props := make(map[string]any)
	for name, expr := range t.PropExprs {
		props[name] = resolveExpr(expr, ctx)
	}
	return &ComponentNode{
		Tag:   t.ComponentTag,
		Props: props,
		// Instance and SubTree are resolved by the framework after diffing
		// detects prop changes.
	}
}

// ---------------------------------------------------------------------------
// Facts resolution
// ---------------------------------------------------------------------------

// resolveFacts resolves directives into a Facts struct.
func resolveFacts(t *templateNode, ctx *resolveContext) Facts {
	var f Facts

	// Start with static HTML attributes
	for _, a := range t.Attrs {
		if a.Key == "class" || a.Key == "style" || a.Key == "id" {
			// Common attributes stored as properties for efficiency
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

	// Apply directives
	for _, d := range t.Directives {
		switch d.Type {
		case "text":
			// Handled at element level (replaces children)
			continue
		case "if":
			// Handled before element creation
			continue

		case "bind":
			// Two-way binding: set value property + input event
			val := resolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["value"] = fmt.Sprint(val)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			f.Events["input"] = EventHandler{
				Handler: "__bind__",
				Args:    []any{d.Expr},
			}

		case "checked":
			val := resolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["checked"] = isTruthy(val)

		case "show":
			val := resolveExpr(d.Expr, ctx)
			if !isTruthy(val) {
				if f.Styles == nil {
					f.Styles = make(map[string]string)
				}
				f.Styles["display"] = "none"
			}

		case "class":
			val := resolveExpr(d.Expr, ctx)
			if isTruthy(val) {
				// Add class to existing className
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

		case "attr":
			val := resolveExpr(d.Expr, ctx)
			if f.Attrs == nil {
				f.Attrs = make(map[string]string)
			}
			f.Attrs[d.Name] = fmt.Sprint(val)

		case "style":
			val := resolveExpr(d.Expr, ctx)
			if f.Styles == nil {
				f.Styles = make(map[string]string)
			}
			f.Styles[d.Name] = fmt.Sprint(val)

		case "click", "mousedown", "mousemove", "mouseup", "wheel", "drop":
			method, args := parseMethodCall(d.Expr)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			resolvedArgs := resolveArgs(args, ctx)
			f.Events[d.Type] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
			}

		case "keydown":
			method, args := parseMethodCall(d.Expr)
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			resolvedArgs := resolveArgs(args, ctx)
			// Key-specific keydown events get a composite key in the events map
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
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			val := resolveExpr(d.Expr, ctx)
			f.Events["dragstart"] = EventHandler{
				Handler: "__draggable__",
				Args:    []any{d.Name, val}, // group, value
			}

		case "dropzone":
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			method, args := parseMethodCall(d.Expr)
			resolvedArgs := resolveArgs(args, ctx)
			f.Events["drop"] = EventHandler{
				Handler: method,
				Args:    resolvedArgs,
			}
		}
	}

	return f
}

// ---------------------------------------------------------------------------
// Expression resolution
// ---------------------------------------------------------------------------

// resolveExpr resolves an expression string against the context.
// Supports: field names ("Count"), dotted paths ("todo.Text"), loop variables,
// and literals (true, false, integers, quoted strings).
func resolveExpr(expr string, ctx *resolveContext) any {
	expr = strings.TrimSpace(expr)

	// Literals
	if expr == "true" {
		return true
	}
	if expr == "false" {
		return false
	}

	// Check loop variables first
	if ctx.Vars != nil {
		// Direct variable: "todo", "i"
		if val, ok := ctx.Vars[expr]; ok {
			return val
		}
		// Dotted path starting with loop variable: "todo.Text"
		if dotIdx := strings.Index(expr, "."); dotIdx != -1 {
			root := expr[:dotIdx]
			if val, ok := ctx.Vars[root]; ok {
				return resolveFieldPath(val, expr[dotIdx+1:])
			}
		}
	}

	// Resolve against struct state
	return resolveStructField(ctx.State, expr)
}

// resolveStructField reads a field (or dotted path) from a reflect.Value struct.
func resolveStructField(v reflect.Value, path string) any {
	// Dereference pointers
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
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
		// Dereference intermediate pointers
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
	}
	return v.Interface()
}

// resolveFieldPath reads a dotted path from an arbitrary value.
func resolveFieldPath(val any, path string) any {
	v := reflect.ValueOf(val)
	return resolveStructField(v, path)
}

// resolveArgs resolves a list of argument expressions.
func resolveArgs(argExprs []string, ctx *resolveContext) []any {
	if len(argExprs) == 0 {
		return nil
	}
	args := make([]any, len(argExprs))
	for i, expr := range argExprs {
		args[i] = resolveExpr(expr, ctx)
	}
	return args
}

// ---------------------------------------------------------------------------
// Method call parsing: "Save", "Toggle(i)", "Remove(i, todo.ID)"
// ---------------------------------------------------------------------------

func parseMethodCall(expr string) (method string, args []string) {
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

// isTruthy returns whether a value is considered true for g-if/g-show/g-class.
func isTruthy(val any) bool {
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
		// Use reflect for other types (slices, etc.)
		rv := reflect.ValueOf(val)
		switch rv.Kind() {
		case reflect.Slice, reflect.Map:
			return rv.Len() > 0
		default:
			return true
		}
	}
}

func copyVars(vars map[string]any) map[string]any {
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
