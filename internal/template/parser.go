package template

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// --- Template expansion (custom elements → HTML) ---

// openTagRe matches the opening tag of a custom element (tag name with hyphen).
var openTagRe = regexp.MustCompile(`<([a-z][a-z0-9]*-[a-z0-9-]*)(\s[^>]*?)?\s*/?>`)

// gAttrRe matches g-* attributes (including g-class:done etc.) in an attribute string.
var gAttrRe = regexp.MustCompile(`(g-[a-z]+(?::[a-z-]+)?)\s*=\s*"([^"]*)"`)

// ExpandComponents takes HTML and recursively replaces custom element tags
// with the contents of their corresponding HTML files from the filesystem.
func ExpandComponents(htmlStr string, fsys fs.FS, baseDir string) (string, error) {
	maxExpansions := 10
	searchFrom := 0
	for expansions := 0; expansions < maxExpansions; expansions++ {
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
			expansions--
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
		compHTML, err := fs.ReadFile(fsys, path.Join(baseDir, tagName+".html"))
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
		}

		htmlStr = htmlStr[:loc[0]] + expanded + htmlStr[end:]
	}

	return htmlStr, nil
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
