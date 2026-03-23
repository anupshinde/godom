package template

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// --- Template expansion (custom elements → HTML) ---

// openTagRe matches the opening tag of a custom element (tag name with hyphen).
var openTagRe = regexp.MustCompile(`<([a-z][a-z0-9]*-[a-z0-9-]*)(\s[^>]*?)?\s*/?>`)

// propAttrRe matches :propName="expr" attributes on custom elements.
var propAttrRe = regexp.MustCompile(`:([a-zA-Z][a-zA-Z0-9_]*)\s*=\s*"([^"]*)"`)

// gAttrRe matches g-* attributes (including g-class:done etc.) in an attribute string.
var gAttrRe = regexp.MustCompile(`(g-[a-z]+(?::[a-z-]+)?)\s*=\s*"([^"]*)"`)

// ExpandComponents takes HTML and recursively replaces custom element tags
// with the contents of their corresponding HTML files from the filesystem.
func ExpandComponents(htmlStr string, fsys fs.FS) (string, error) {
	maxDepth := 10
	searchFrom := 0
	for depth := 0; depth < maxDepth; depth++ {
		loc := openTagRe.FindStringSubmatchIndex(htmlStr[searchFrom:])
		if loc == nil {
			break
		}
		// Adjust indices relative to full string
		for i := range loc {
			if loc[i] >= 0 {
				loc[i] += searchFrom
			}
		}

		tagName := htmlStr[loc[2]:loc[3]]

		// Skip g-* tags — these are framework directives, not custom components.
		if strings.HasPrefix(tagName, "g-") {
			searchFrom = loc[1]
			depth--
			continue
		}

		searchFrom = 0
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
			gAttrs := ExtractGAttrs(attrs)
			if gAttrs != "" {
				expanded = TransferAttrsToRoot(expanded, gAttrs)
			}

			// Extract :prop="expr" attributes and encode as g-props on root element
			propsAttr := ExtractProps(attrs)
			if propsAttr != "" {
				expanded = TransferAttrsToRoot(expanded, `g-props="`+propsAttr+`"`)
			}
		}

		htmlStr = htmlStr[:loc[0]] + expanded + htmlStr[end:]
	}

	return htmlStr, nil
}

// ExtractProps pulls out :prop="expr" attributes and encodes them as "name:expr,name:expr".
func ExtractProps(attrs string) string {
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

// ExtractGAttrs pulls out g-* attributes (and g-class:* etc.) from an attribute string.
func ExtractGAttrs(attrs string) string {
	matches := gAttrRe.FindAllString(attrs, -1)
	return strings.Join(matches, " ")
}

// TransferAttrsToRoot inserts attributes into the first opening tag of the HTML.
func TransferAttrsToRoot(htmlStr string, attrs string) string {
	idx := strings.Index(htmlStr, ">")
	if idx < 0 {
		return htmlStr
	}

	if idx > 0 && htmlStr[idx-1] == '/' {
		return htmlStr[:idx-1] + " " + attrs + " />" + htmlStr[idx+1:]
	}

	return htmlStr[:idx] + " " + attrs + htmlStr[idx:]
}

// --- Helpers used by validate.go ---

// ForParts holds parsed parts of a g-for expression.
type ForParts struct {
	Item  string
	Index string
	List  string
}

// ParseForExprParts parses a g-for expression like "item, i in List".
func ParseForExprParts(expr string) *ForParts {
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
	return &ForParts{Item: item, Index: idx, List: list}
}

// ParsePropsAttr parses a g-props attribute value like "index:i,todo:todo"
// into a map of prop name → parent expression.
func ParsePropsAttr(val string) map[string]string {
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

// ExprRoot returns the top-level field name from an expression.
// "InputText" → "InputText", "todo.Done" → "todo"
func ExprRoot(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	if idx := strings.Index(expr, "."); idx != -1 {
		return expr[:idx]
	}
	return expr
}
