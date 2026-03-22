// Package vdom implements a virtual DOM tree with diffing and patching.
// It handles template parsing, tree resolution, diffing, and patch generation.
// The godom runtime uses this package for its rendering pipeline.
package vdom

// Node type constants identify each Node variant.
const (
	NodeText    = iota // plain text content
	NodeElement        // HTML/SVG element with tag, facts, children
	NodeKeyed          // element with keyed children (for efficient list diffing)
	NodePlugin         // opaque JS-managed node (plugin escape hatch)
	NodeLazy           // deferred computation, skipped if inputs unchanged
)

// Node is the interface implemented by all virtual DOM node types.
type Node interface {
	NodeType() int
	NodeID() int
	DescendantsCount() int
}

// NodeBase holds fields common to all node types.
// Embed this in every concrete node.
type NodeBase struct {
	ID          int // stable identity, assigned by Go, used to address the node in the bridge
	Descendants int // cached count, set by ComputeDescendants
}

func (b *NodeBase) NodeID() int          { return b.ID }
func (b *NodeBase) DescendantsCount() int { return b.Descendants }

// ---------------------------------------------------------------------------
// Text
// ---------------------------------------------------------------------------

// TextNode is a leaf node containing plain text.
type TextNode struct {
	NodeBase
	Text string
}

func (n *TextNode) NodeType() int { return NodeText }

// ---------------------------------------------------------------------------
// Element
// ---------------------------------------------------------------------------

// ElementNode represents an HTML or SVG element.
type ElementNode struct {
	NodeBase
	Tag       string // e.g. "div", "span", "path"
	Namespace string // "" for HTML, "http://www.w3.org/2000/svg" for SVG
	Facts     Facts
	Children  []Node
}

func (n *ElementNode) NodeType() int { return NodeElement }

// AppendChild adds a child node.
func (n *ElementNode) AppendChild(child Node) {
	n.Children = append(n.Children, child)
}

// RemoveChild removes the child at the given index. Returns false if out of bounds.
func (n *ElementNode) RemoveChild(index int) bool {
	if index < 0 || index >= len(n.Children) {
		return false
	}
	n.Children = append(n.Children[:index], n.Children[index+1:]...)
	return true
}

// ReplaceChild replaces the child at the given index. Returns false if out of bounds.
func (n *ElementNode) ReplaceChild(index int, child Node) bool {
	if index < 0 || index >= len(n.Children) {
		return false
	}
	n.Children[index] = child
	return true
}

// RemoveChildByID removes the first child with the given node ID. Returns false if not found.
func (n *ElementNode) RemoveChildByID(id int) bool {
	for i, c := range n.Children {
		if c.NodeID() == id {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// KeyedElement
// ---------------------------------------------------------------------------

// KeyedChild pairs a stable key with a child node.
type KeyedChild struct {
	Key  string
	Node Node
}

// KeyedElementNode is an element whose children have stable keys.
type KeyedElementNode struct {
	NodeBase
	Tag       string
	Namespace string
	Facts     Facts
	Children  []KeyedChild
}

func (n *KeyedElementNode) NodeType() int { return NodeKeyed }

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

// PluginNode is an opaque node managed by a JS plugin.
type PluginNode struct {
	NodeBase
	Tag   string // host element tag, e.g. "canvas", "div"
	Name  string // plugin name, e.g. "chart"
	Facts Facts
	Data  any // JSON-serializable data passed to JS plugin
}

func (n *PluginNode) NodeType() int { return NodePlugin }

// ---------------------------------------------------------------------------
// Lazy
// ---------------------------------------------------------------------------

// LazyNode defers tree construction until diffing time.
type LazyNode struct {
	NodeBase
	Func   any   // the view function
	Args   []any // arguments checked by reference equality
	Cached Node  // previously computed result (nil on first render)
}

func (n *LazyNode) NodeType() int { return NodeLazy }
func (n *LazyNode) DescendantsCount() int {
	if n.Cached == nil {
		return 0
	}
	return 1 + n.Cached.DescendantsCount()
}

// ---------------------------------------------------------------------------
// Facts
// ---------------------------------------------------------------------------

// Facts holds all the "attributes" of an element, categorized by type.
type Facts struct {
	Props   map[string]any          // DOM properties: id, className, value, checked, ...
	Attrs   map[string]string       // HTML attributes: data-*, aria-*, custom
	AttrsNS map[string]NSAttr       // namespaced attributes: xlink:href, xml:lang
	Styles  map[string]string       // CSS properties: background-color, width, ...
	Events  map[string]EventHandler // event listeners: click, input, keydown, ...
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

// FindNodeByID searches the tree for a node with the given ID.
func FindNodeByID(root Node, id int) Node {
	if root == nil {
		return nil
	}
	if root.NodeID() == id {
		return root
	}
	switch n := root.(type) {
	case *ElementNode:
		for _, c := range n.Children {
			if found := FindNodeByID(c, id); found != nil {
				return found
			}
		}
	case *KeyedElementNode:
		for _, kc := range n.Children {
			if found := FindNodeByID(kc.Node, id); found != nil {
				return found
			}
		}
	case *LazyNode:
		if n.Cached != nil {
			return FindNodeByID(n.Cached, id)
		}
	}
	return nil
}

// ComputeDescendants recursively calculates and caches descendant counts.
// Must be called after building a tree and before diffing.
func ComputeDescendants(n Node) int {
	switch n := n.(type) {
	case *TextNode:
		n.Descendants = 0
		return 0
	case *ElementNode:
		count := 0
		for _, c := range n.Children {
			count += 1 + ComputeDescendants(c)
		}
		n.Descendants = count
		return count
	case *KeyedElementNode:
		count := 0
		for _, kc := range n.Children {
			count += 1 + ComputeDescendants(kc.Node)
		}
		n.Descendants = count
		return count
	case *PluginNode:
		n.Descendants = 0
		return 0
	case *LazyNode:
		if n.Cached != nil {
			count := 1 + ComputeDescendants(n.Cached)
			n.Descendants = count
			return count
		}
		n.Descendants = 0
		return 0
	}
	return 0
}
