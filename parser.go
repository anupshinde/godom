package godom

import (
	"bytes"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// binding represents a data-binding directive on an element.
type binding struct {
	GID  string // element's data-gid
	Dir  string // "text", "bind", "checked", "if", "show", "class:name"
	Expr string // expression: "InputText", "todo.Done"
}

// eventBinding represents an event directive on an element.
type eventBinding struct {
	GID    string   // element's data-gid
	Event  string   // "click", "keydown", "input"
	Key    string   // key filter for keydown (e.g. "Enter")
	Method string   // method name: "AddTodo", "Toggle"
	Args   []string // argument expressions: ["i"]
}

// forTemplate represents a g-for loop extracted at parse time.
type forTemplate struct {
	GID          string            // anchor identifier
	ItemVar      string            // "todo"
	IndexVar     string            // "i" (empty if unused)
	ListField    string            // "Todos"
	TemplateHTML string            // rendered template HTML with __IDX__ placeholders
	Bindings     []binding         // bindings inside template (gids have __IDX__)
	Events       []eventBinding    // events inside template (gids have __IDX__)
	Props        map[string]string // prop name → parent expression (e.g. "index" → "i")
	ComponentTag string            // non-empty if this is a registered stateful component
}

// pageBindings holds all parsed binding information for the page.
type pageBindings struct {
	HTML     string         // full HTML with data-gid attributes and g-for anchors
	Bindings []binding      // top-level bindings (not inside g-for)
	Events   []eventBinding // top-level events
	ForLoops []*forTemplate // g-for templates

	// Lookup: field name → indices for fast patch computation
	FieldToBindings map[string][]int
	FieldToForLoops map[string][]int
}

// htmlParser assigns gids and extracts bindings from the DOM tree.
type htmlParser struct {
	gidSeq int
}

func (p *htmlParser) nextGID() string {
	p.gidSeq++
	return fmt.Sprintf("g%d", p.gidSeq)
}

// parsePageHTML parses composed HTML, assigns data-gid to directive elements,
// extracts bindings/events/for-loops, and returns the modified HTML + registry.
func parsePageHTML(composedHTML string) (*pageBindings, error) {
	doc, err := html.Parse(strings.NewReader(composedHTML))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	p := &htmlParser{}
	pb := &pageBindings{
		FieldToBindings: make(map[string][]int),
		FieldToForLoops: make(map[string][]int),
	}

	p.walkNode(doc, pb)

	// Render modified document back to HTML
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return nil, fmt.Errorf("render HTML: %w", err)
	}
	pb.HTML = buf.String()

	// Build field → index lookup tables
	for i, b := range pb.Bindings {
		field := exprRoot(b.Expr)
		if field != "" {
			pb.FieldToBindings[field] = append(pb.FieldToBindings[field], i)
		}
	}
	for i, fl := range pb.ForLoops {
		pb.FieldToForLoops[fl.ListField] = append(pb.FieldToForLoops[fl.ListField], i)
	}

	return pb, nil
}

// walkNode recursively processes the DOM tree, assigning gids and extracting bindings.
func (p *htmlParser) walkNode(n *html.Node, pb *pageBindings) {
	if n.Type == html.ElementNode && hasDirectiveAttr(n) {
		// g-for elements get special treatment
		if getAttr(n, "g-for") != "" {
			p.processForNode(n, pb)
			return // don't recurse; children belong to the template
		}

		gid := p.nextGID()
		setAttr(n, "data-gid", gid)
		p.extractBindings(n, gid, &pb.Bindings)
		p.extractEvents(n, gid, &pb.Events)
	}

	// Recurse into children (collect first to avoid mutation issues)
	var children []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}
	for _, c := range children {
		p.walkNode(c, pb)
	}
}

// processForNode extracts a g-for element as a template, replaces it with
// anchor comments in the DOM, and records the template + bindings.
func (p *htmlParser) processForNode(n *html.Node, pb *pageBindings) {
	forExpr := getAttr(n, "g-for")
	parts := parseForExprParts(forExpr)
	if parts == nil {
		return
	}

	anchorGID := p.nextGID()

	// Parse g-props attribute (prop mappings from parent custom element)
	props := parsePropsAttr(getAttr(n, "g-props"))
	removeAttr(n, "g-props")

	// Check if this is a registered stateful component
	compTag := getAttr(n, "data-g-component")
	removeAttr(n, "data-g-component")

	// Remove g-for attribute from the node before processing its subtree
	removeAttr(n, "g-for")

	// Walk the subtree, assigning gids with __IDX__ placeholder
	ft := &forTemplate{
		GID:          anchorGID,
		ItemVar:      parts.item,
		IndexVar:     parts.index,
		ListField:    parts.list,
		Props:        props,
		ComponentTag: compTag,
	}
	p.walkForSubtree(n, anchorGID, ft)

	// Render the processed element to HTML (this becomes the template)
	var buf bytes.Buffer
	html.Render(&buf, n)
	ft.TemplateHTML = buf.String()

	pb.ForLoops = append(pb.ForLoops, ft)

	// Replace the element in the DOM with anchor comments
	parent := n.Parent
	startAnchor := &html.Node{
		Type: html.CommentNode,
		Data: " g-for:" + anchorGID + " ",
	}
	endAnchor := &html.Node{
		Type: html.CommentNode,
		Data: " /g-for:" + anchorGID + " ",
	}
	parent.InsertBefore(startAnchor, n)
	parent.InsertBefore(endAnchor, n)
	parent.RemoveChild(n)
}

// walkForSubtree assigns gids with __IDX__ placeholders and extracts
// bindings/events for a g-for template subtree.
func (p *htmlParser) walkForSubtree(n *html.Node, anchorGID string, ft *forTemplate) {
	seq := 0

	var walk func(node *html.Node, isRoot bool)
	walk = func(node *html.Node, isRoot bool) {
		if node.Type != html.ElementNode {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				walk(c, false)
			}
			return
		}

		// Assign gid to elements that have directives (or the root element always)
		needsGID := isRoot || hasDirectiveAttr(node)
		if needsGID {
			var gid string
			if isRoot {
				gid = fmt.Sprintf("%s-__IDX__", anchorGID)
			} else {
				gid = fmt.Sprintf("%s-__IDX__-%d", anchorGID, seq)
				seq++
			}
			setAttr(node, "data-gid", gid)

			if hasDirectiveAttr(node) {
				p.extractBindings(node, gid, &ft.Bindings)
				p.extractEvents(node, gid, &ft.Events)
			}
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c, false)
		}
	}

	walk(n, true)
}

// extractBindings reads data-binding directives from an element's attributes.
func (p *htmlParser) extractBindings(n *html.Node, gid string, out *[]binding) {
	for _, a := range n.Attr {
		dir := ""
		switch {
		case a.Key == "g-text":
			dir = "text"
		case a.Key == "g-bind":
			dir = "bind"
		case a.Key == "g-checked":
			dir = "checked"
		case a.Key == "g-if":
			dir = "if"
		case a.Key == "g-show":
			dir = "show"
		case strings.HasPrefix(a.Key, "g-class:"):
			dir = a.Key[2:] // "class:done"
		case strings.HasPrefix(a.Key, "g-attr:"):
			dir = a.Key[2:] // "attr:transform"
		case strings.HasPrefix(a.Key, "g-style:"):
			dir = a.Key[2:] // "style:background-color"
		case strings.HasPrefix(a.Key, "g-plugin:"):
			dir = a.Key[2:] // "plugin:chartjs"
		}
		if dir != "" {
			*out = append(*out, binding{GID: gid, Dir: dir, Expr: a.Val})
		}
	}
}

// extractEvents reads event directives from an element's attributes.
func (p *htmlParser) extractEvents(n *html.Node, gid string, out *[]eventBinding) {
	for _, a := range n.Attr {
		switch {
		case a.Key == "g-click":
			name, args := parseCallExpr(a.Val)
			*out = append(*out, eventBinding{
				GID: gid, Event: "click", Method: name, Args: args,
			})
		case a.Key == "g-keydown":
			// Support multiple bindings separated by semicolons:
			// g-keydown="ArrowUp:PanUp;ArrowDown:PanDown;Enter:Submit"
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
				name, args := parseCallExpr(method)
				*out = append(*out, eventBinding{
					GID: gid, Event: "keydown", Key: key, Method: name, Args: args,
				})
			}
		case a.Key == "g-mousedown" || a.Key == "g-mousemove" || a.Key == "g-mouseup":
			event := a.Key[2:] // "mousedown", "mousemove", "mouseup"
			name, args := parseCallExpr(a.Val)
			*out = append(*out, eventBinding{
				GID: gid, Event: event, Method: name, Args: args,
			})
		case a.Key == "g-wheel":
			name, args := parseCallExpr(a.Val)
			*out = append(*out, eventBinding{
				GID: gid, Event: "wheel", Method: name, Args: args,
			})
		case a.Key == "g-bind":
			// Two-way binding: also register an input event
			*out = append(*out, eventBinding{
				GID: gid, Event: "input", Method: "__bind", Args: []string{a.Val},
			})
		}
	}
}

// --- HTML node helpers ---

func hasDirectiveAttr(n *html.Node) bool {
	for _, a := range n.Attr {
		if strings.HasPrefix(a.Key, "g-") {
			return true
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func removeAttr(n *html.Node, key string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return
		}
	}
}

type forParts struct {
	item  string
	index string
	list  string
}

func parseForExprParts(expr string) *forParts {
	parts := strings.SplitN(expr, " in ", 2)
	if len(parts) != 2 {
		return nil
	}
	left := strings.TrimSpace(parts[0])
	list := strings.TrimSpace(parts[1])
	vars := strings.Split(left, ",")
	item := strings.TrimSpace(vars[0])
	idx := ""
	if len(vars) > 1 {
		idx = strings.TrimSpace(vars[1])
	}
	return &forParts{item: item, index: idx, list: list}
}

// parsePropsAttr parses a g-props attribute value like "index:i,todo:todo"
// into a map of prop name → parent expression.
func parsePropsAttr(val string) map[string]string {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	props := make(map[string]string)
	for _, pair := range strings.Split(val, ",") {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return props
}

// exprRoot returns the top-level field name from an expression.
// "InputText" → "InputText", "todo.Done" → "todo"
func exprRoot(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	if idx := strings.Index(expr, "."); idx != -1 {
		return expr[:idx]
	}
	return expr
}

// --- Template expansion (custom elements → HTML) ---

// openTagRe matches the opening tag of a custom element (tag name with hyphen).
var openTagRe = regexp.MustCompile(`<([a-z][a-z0-9]*-[a-z0-9-]*)(\s[^>]*?)?\s*/?>`)

// propAttrRe matches :propName="expr" attributes on custom elements.
var propAttrRe = regexp.MustCompile(`:([a-zA-Z][a-zA-Z0-9_]*)\s*=\s*"([^"]*)"`)

// gAttrRe matches g-* attributes (including g-class:done etc.) in an attribute string.
var gAttrRe = regexp.MustCompile(`(g-[a-z]+(?::[a-z-]+)?)\s*=\s*"([^"]*)"`)

// expandComponents takes HTML and recursively replaces custom element tags
// with the contents of their corresponding HTML files from the filesystem.
func expandComponents(htmlStr string, fsys fs.FS, registry map[string]*componentReg) (string, error) {
	maxDepth := 10
	for depth := 0; depth < maxDepth; depth++ {
		loc := openTagRe.FindStringSubmatchIndex(htmlStr)
		if loc == nil {
			break
		}

		tagName := htmlStr[loc[2]:loc[3]]
		var attrs string
		if loc[4] >= 0 {
			attrs = strings.TrimSpace(htmlStr[loc[4]:loc[5]])
		}

		// Determine if self-closing or has a closing tag
		openTag := htmlStr[loc[0]:loc[1]]
		var end int
		if strings.HasSuffix(openTag, "/>") {
			// Self-closing
			end = loc[1]
		} else {
			// Find matching closing tag
			closeTag := "</" + tagName + ">"
			closeIdx := strings.Index(htmlStr[loc[1]:], closeTag)
			if closeIdx < 0 {
				return "", fmt.Errorf("component %q: missing closing tag", tagName)
			}
			end = loc[1] + closeIdx + len(closeTag)
		}

		// Load component HTML
		compHTML, err := fs.ReadFile(fsys, tagName+".html")
		if err != nil {
			return "", fmt.Errorf("component %q: %w", tagName, err)
		}

		expanded := strings.TrimSpace(string(compHTML))

		// Transfer g-* attributes from the custom tag to the component's root element
		if attrs != "" {
			gAttrs := extractGAttrs(attrs)
			if gAttrs != "" {
				expanded = transferAttrsToRoot(expanded, gAttrs)
			}

			// Extract :prop="expr" attributes and encode as g-props on root element
			propsAttr := extractProps(attrs)
			if propsAttr != "" {
				expanded = transferAttrsToRoot(expanded, `g-props="`+propsAttr+`"`)
			}
		}

		// Mark registered (stateful) components so the parser can scope them
		if _, ok := registry[tagName]; ok {
			expanded = transferAttrsToRoot(expanded, `data-g-component="`+tagName+`"`)
		}

		htmlStr = htmlStr[:loc[0]] + expanded + htmlStr[end:]
	}

	return htmlStr, nil
}

// extractProps pulls out :prop="expr" attributes and encodes them as "name:expr,name:expr".
func extractProps(attrs string) string {
	matches := propAttrRe.FindAllStringSubmatch(attrs, -1)
	if len(matches) == 0 {
		return ""
	}
	var parts []string
	for _, m := range matches {
		parts = append(parts, m[1]+":"+m[2])
	}
	return strings.Join(parts, ",")
}

// extractGAttrs pulls out g-* attributes (and g-class:* etc.) from an attribute string.
func extractGAttrs(attrs string) string {
	matches := gAttrRe.FindAllString(attrs, -1)
	return strings.Join(matches, " ")
}

// transferAttrsToRoot inserts attributes into the first opening tag of the HTML.
func transferAttrsToRoot(htmlStr string, attrs string) string {
	idx := strings.Index(htmlStr, ">")
	if idx < 0 {
		return htmlStr
	}

	if htmlStr[idx-1] == '/' {
		return htmlStr[:idx-1] + " " + attrs + " />" + htmlStr[idx+1:]
	}

	return htmlStr[:idx] + " " + attrs + htmlStr[idx:]
}

// findIndexHTML locates index.html in the filesystem,
// checking root and one level of subdirectories.
func findIndexHTML(fsys fs.FS) (fs.FS, error) {
	if _, err := fs.ReadFile(fsys, "index.html"); err == nil {
		return fsys, nil
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("cannot read filesystem: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			if _, err := fs.ReadFile(fsys, e.Name()+"/index.html"); err == nil {
				sub, err := fs.Sub(fsys, e.Name())
				if err != nil {
					return nil, err
				}
				return sub, nil
			}
		}
	}

	return nil, fmt.Errorf("index.html not found in filesystem")
}
