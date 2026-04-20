package template

import (
	"errors"
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

// FSLayer is one filesystem search location for partial resolution.
// Layers are consulted in order; the first hit wins.
type FSLayer struct {
	FS      fs.FS
	BaseDir string
	// Label is shown in "not found" error messages to help identify which
	// layer was searched (e.g. "island \"solar\" FS", "shared").
	Label string
}

// ExpandPartials takes HTML and recursively replaces custom element tags
// with partial content from a single filesystem (sibling-file lookup).
// This is the simple entry point; internally delegates to ExpandPartialsLayered.
func ExpandPartials(htmlStr string, fsys fs.FS, baseDir string) (string, error) {
	return ExpandPartialsLayered(htmlStr, []FSLayer{{FS: fsys, BaseDir: baseDir}}, nil)
}

// ExpandPartialsLayered takes HTML and recursively replaces custom element tags
// with partial content. For each tag it consults, in order:
//  1. Each FSLayer at path.Join(layer.BaseDir, tag+".html")
//  2. registry[tag] (registered via Engine.RegisterPartial / UsePartials)
//
// If no source has the tag, an error is returned that lists every location
// searched to aid debugging.
func ExpandPartialsLayered(htmlStr string, layers []FSLayer, registry map[string]string) (string, error) {
	maxExpansions := 10
	searchFrom := 0
	for expansions := 0; expansions < maxExpansions; expansions++ {
		loc := openTagRe.FindStringSubmatchIndex(htmlStr[searchFrom:])
		if loc == nil {
			break
		}
		for i := range loc {
			if loc[i] >= 0 {
				loc[i] += searchFrom
			}
		}

		tagName := htmlStr[loc[2]:loc[3]]

		// Skip g-* tags — framework directives, not custom elements (partials).
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

		openTag := htmlStr[loc[0]:loc[1]]
		var end int
		if strings.HasSuffix(openTag, "/>") {
			end = loc[1]
		} else {
			closeTag := "</" + tagName + ">"
			closeIdx := strings.Index(htmlStr[loc[1]:], closeTag)
			if closeIdx < 0 {
				return "", fmt.Errorf("partial %q: missing closing tag", tagName)
			}
			end = loc[1] + closeIdx + len(closeTag)
		}

		compHTML, err := lookupPartial(tagName, layers, registry)
		if err != nil {
			return "", err
		}

		expanded := strings.TrimSpace(string(compHTML))
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

// lookupPartial resolves a tag name to HTML content. Tries FS layers in order,
// then the registry. Returns a structured error listing everything tried.
func lookupPartial(tagName string, layers []FSLayer, registry map[string]string) ([]byte, error) {
	var tried []string
	for _, layer := range layers {
		if layer.FS == nil {
			continue
		}
		p := path.Join(layer.BaseDir, tagName+".html")
		label := p
		if layer.Label != "" {
			label = fmt.Sprintf("%s (%s)", p, layer.Label)
		}
		tried = append(tried, label)
		b, err := fs.ReadFile(layer.FS, p)
		if err == nil {
			return b, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("partial %q: %w", tagName, err)
		}
	}
	if registry != nil {
		if html, ok := registry[tagName]; ok {
			return []byte(html), nil
		}
		tried = append(tried, fmt.Sprintf("registry[%q]", tagName))
	}
	if len(tried) == 0 {
		return nil, fmt.Errorf("partial %q: not found (no FS layers or registry configured)", tagName)
	}
	return nil, fmt.Errorf("partial %q: not found; searched:\n  - %s",
		tagName, strings.Join(tried, "\n  - "))
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
