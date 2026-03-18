package vdom

import (
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
}

// Directive represents a single g-* directive on an element.
type Directive struct {
	Type string // "text", "bind", "checked", "if", "show", "class", "attr", "style",
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
		case a.Key == "g-checked":
			dirs = append(dirs, Directive{Type: "checked", Expr: a.Val})
		case a.Key == "g-if":
			dirs = append(dirs, Directive{Type: "if", Expr: a.Val})
		case a.Key == "g-show":
			dirs = append(dirs, Directive{Type: "show", Expr: a.Val})

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

// ResolveContext holds the state and loop variables available during tree resolution.
type ResolveContext struct {
	State reflect.Value  // the component struct (or pointer to it)
	Vars  map[string]any // loop variables: {todo: item, i: index}
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

func resolveTextNode(t *TemplateNode, ctx *ResolveContext) []Node {
	if len(t.TextParts) == 1 && t.TextParts[0].Static {
		return []Node{&TextNode{Text: t.TextParts[0].Value}}
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
	return []Node{&TextNode{Text: sb.String()}}
}

func resolveElementNode(t *TemplateNode, ctx *ResolveContext) Node {
	el := &ElementNode{
		Tag:       t.Tag,
		Namespace: t.Namespace,
		Facts:     resolveFacts(t, ctx),
	}

	for _, d := range t.Directives {
		if d.Type == "text" {
			val := ResolveExpr(d.Expr, ctx)
			text := fmt.Sprint(val)
			if text != "" {
				el.Children = []Node{&TextNode{Text: text}}
			}
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
			State: ctx.State,
			Vars:  CopyVars(ctx.Vars),
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
	data := ResolveExpr(t.PluginExpr, ctx)
	data = DeepCopyJSON(data)
	return &PluginNode{
		Tag:   t.Tag,
		Name:  t.PluginName,
		Facts: resolveFacts(t, ctx),
		Data:  data,
	}
}

func resolveComponentNode(t *TemplateNode, ctx *ResolveContext) Node {
	props := make(map[string]any)
	for name, expr := range t.PropExprs {
		props[name] = ResolveExpr(expr, ctx)
	}
	return &ComponentNode{
		Tag:   t.ComponentTag,
		Props: props,
	}
}

// ---------------------------------------------------------------------------
// Facts resolution
// ---------------------------------------------------------------------------

func resolveFacts(t *TemplateNode, ctx *ResolveContext) Facts {
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
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			f.Events["input"] = EventHandler{
				Handler: "__bind__",
				Args:    []any{d.Expr},
			}

		case "checked":
			val := ResolveExpr(d.Expr, ctx)
			if f.Props == nil {
				f.Props = make(map[string]any)
			}
			f.Props["checked"] = IsTruthy(val)

		case "show":
			val := ResolveExpr(d.Expr, ctx)
			if !IsTruthy(val) {
				if f.Styles == nil {
					f.Styles = make(map[string]string)
				}
				f.Styles["display"] = "none"
			}

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

		case "attr":
			val := ResolveExpr(d.Expr, ctx)
			if f.Attrs == nil {
				f.Attrs = make(map[string]string)
			}
			f.Attrs[d.Name] = fmt.Sprint(val)

		case "style":
			val := ResolveExpr(d.Expr, ctx)
			if f.Styles == nil {
				f.Styles = make(map[string]string)
			}
			f.Styles[d.Name] = fmt.Sprint(val)

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
			if f.Events == nil {
				f.Events = make(map[string]EventHandler)
			}
			val := ResolveExpr(d.Expr, ctx)
			f.Events["dragstart"] = EventHandler{
				Handler: "__draggable__",
				Args:    []any{d.Name, val},
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

	return f
}

// ---------------------------------------------------------------------------
// Expression resolution
// ---------------------------------------------------------------------------

// ResolveExpr resolves an expression string against the context.
func ResolveExpr(expr string, ctx *ResolveContext) any {
	expr = strings.TrimSpace(expr)

	if expr == "true" {
		return true
	}
	if expr == "false" {
		return false
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

	return resolveStructField(ctx.State, expr)
}

func resolveStructField(v reflect.Value, path string) any {
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
