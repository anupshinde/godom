package godom

// Node types — inspired by Elm's virtual-dom.
// These represent the virtual DOM tree that godom diffs to produce patches.

// nodeType constants identify each Node variant.
const (
	nodeText      = iota // plain text content
	nodeElement          // HTML/SVG element with tag, facts, children
	nodeKeyed            // element with keyed children (for efficient list diffing)
	nodeComponent        // stateful child component
	nodePlugin           // opaque JS-managed node (plugin escape hatch)
	nodeLazy             // deferred computation, skipped if inputs unchanged
)

// Node is the interface implemented by all virtual DOM node types.
type Node interface {
	nodeType() int
	// descendantsCount returns the total number of descendant nodes.
	// Used for index-based patch addressing — allows skipping entire subtrees.
	descendantsCount() int
}

// ---------------------------------------------------------------------------
// Text
// ---------------------------------------------------------------------------

// TextNode is a leaf node containing plain text.
type TextNode struct {
	Text string
}

func (n *TextNode) nodeType() int         { return nodeText }
func (n *TextNode) descendantsCount() int { return 0 }

// ---------------------------------------------------------------------------
// Element
// ---------------------------------------------------------------------------

// ElementNode represents an HTML or SVG element.
type ElementNode struct {
	Tag       string // e.g. "div", "span", "path"
	Namespace string // "" for HTML, "http://www.w3.org/2000/svg" for SVG
	Facts     Facts
	Children  []Node

	descendants int // cached count, set by computeDescendants
}

func (n *ElementNode) nodeType() int         { return nodeElement }
func (n *ElementNode) descendantsCount() int { return n.descendants }

// ---------------------------------------------------------------------------
// KeyedElement
// ---------------------------------------------------------------------------

// KeyedChild pairs a stable key with a child node.
// The key identifies the child across renders for efficient diffing.
type KeyedChild struct {
	Key  string
	Node Node
}

// KeyedElementNode is an element whose children have stable keys.
// Produced by g-for with g-key. Enables insert/remove/move diffing
// instead of positional append/truncate.
type KeyedElementNode struct {
	Tag       string
	Namespace string
	Facts     Facts
	Children  []KeyedChild

	descendants int
}

func (n *KeyedElementNode) nodeType() int         { return nodeKeyed }
func (n *KeyedElementNode) descendantsCount() int { return n.descendants }

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

// ComponentNode represents a stateful child component.
// Props are passed from the parent. SubTree is the component's rendered output.
type ComponentNode struct {
	Tag      string         // custom element name, e.g. "todo-item"
	Props    map[string]any // prop values from parent
	Instance *componentInfo // resolved at render time
	SubTree  Node           // rendered by component's own template
}

func (n *ComponentNode) nodeType() int { return nodeComponent }
func (n *ComponentNode) descendantsCount() int {
	if n.SubTree == nil {
		return 0
	}
	return 1 + n.SubTree.descendantsCount()
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

// PluginNode is an opaque node managed by a JS plugin.
// The Go side only knows the data; the bridge calls init/update
// on the registered plugin handler. Analogous to Elm's CUSTOM node.
type PluginNode struct {
	Tag   string // host element tag, e.g. "canvas", "div"
	Name  string // plugin name, e.g. "chart"
	Facts Facts
	Data  any // JSON-serializable data passed to JS plugin
}

func (n *PluginNode) nodeType() int         { return nodePlugin }
func (n *PluginNode) descendantsCount() int { return 0 }

// ---------------------------------------------------------------------------
// Lazy
// ---------------------------------------------------------------------------

// LazyNode defers tree construction until diffing time.
// If Args are referentially equal to the previous render's Args,
// the Cached tree is reused without calling Func.
type LazyNode struct {
	Func   any   // the view function
	Args   []any // arguments checked by reference equality
	Cached Node  // previously computed result (nil on first render)
}

func (n *LazyNode) nodeType() int { return nodeLazy }
func (n *LazyNode) descendantsCount() int {
	if n.Cached == nil {
		return 0
	}
	return 1 + n.Cached.descendantsCount()
}

// ---------------------------------------------------------------------------
// Facts
// ---------------------------------------------------------------------------

// Facts holds all the "attributes" of an element, categorized by type.
// Follows Elm's organizeFacts pattern — different categories require
// different DOM APIs to apply (property assignment vs setAttribute vs
// style.setProperty vs addEventListener).
type Facts struct {
	Props   map[string]any           // DOM properties: id, className, value, checked, ...
	Attrs   map[string]string        // HTML attributes: data-*, aria-*, custom
	AttrsNS map[string]NSAttr        // namespaced attributes: xlink:href, xml:lang
	Styles  map[string]string        // CSS properties: background-color, width, ...
	Events  map[string]EventHandler  // event listeners: click, input, keydown, ...
}

// NSAttr is a namespaced attribute value (used for SVG xlink/xml attributes).
type NSAttr struct {
	Namespace string
	Value     string
}

// EventHandler describes an event listener that routes to a Go method.
type EventHandler struct {
	Handler string // method name on Go struct
	Args    []any  // pre-resolved arguments
	Scope   string // "forGID:index" for child component routing
	Options EventOptions
}

// EventOptions controls event propagation behavior.
type EventOptions struct {
	StopPropagation bool
	PreventDefault  bool
	Key             string // key filter for keydown events
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// computeDescendants recursively calculates and caches descendant counts.
// Must be called after building a tree and before diffing.
func computeDescendants(n Node) int {
	switch n := n.(type) {
	case *TextNode:
		return 0
	case *ElementNode:
		count := 0
		for _, c := range n.Children {
			count += 1 + computeDescendants(c)
		}
		n.descendants = count
		return count
	case *KeyedElementNode:
		count := 0
		for _, kc := range n.Children {
			count += 1 + computeDescendants(kc.Node)
		}
		n.descendants = count
		return count
	case *ComponentNode:
		if n.SubTree != nil {
			n.SubTree = n.SubTree // ensure computed
			return 1 + computeDescendants(n.SubTree)
		}
		return 0
	case *PluginNode:
		return 0
	case *LazyNode:
		if n.Cached != nil {
			return 1 + computeDescendants(n.Cached)
		}
		return 0
	}
	return 0
}
