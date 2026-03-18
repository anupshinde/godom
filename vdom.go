package godom

import "github.com/anupshinde/godom/vdom"

// vdomBuildInit builds the initial VDomMessage for a new client connection.
func vdomBuildInit(ci *componentInfo) *VDomMessage {
	gid := &gidCounter{seq: ci.gidSeq}
	tree := vdomBuildTree(ci)

	htmlStr, events := renderToHTMLWithEvents(tree.Children, gid)

	ci.prevTree = tree
	ci.gidSeq = gid.seq

	return encodeInitMessage(htmlStr, events)
}

// vdomBuildUpdate rebuilds the tree, diffs against the previous tree,
// and returns a VDomMessage with patches. Returns nil if no changes.
func vdomBuildUpdate(ci *componentInfo) *VDomMessage {
	gid := &gidCounter{seq: ci.gidSeq}
	newTree := vdomBuildTree(ci)

	if ci.prevTree == nil {
		htmlStr, events := renderToHTMLWithEvents(newTree.Children, gid)
		ci.prevTree = newTree
		ci.gidSeq = gid.seq
		return encodeInitMessage(htmlStr, events)
	}

	patches := vdom.Diff(ci.prevTree, newTree)
	if len(patches) == 0 {
		return nil
	}

	msg := encodePatchMessage(patches, gid)
	ci.prevTree = newTree
	ci.gidSeq = gid.seq
	return msg
}

// vdomBuildTree resolves the template tree against the current component state.
func vdomBuildTree(ci *componentInfo) *vdom.ElementNode {
	ctx := &vdom.ResolveContext{
		State: ci.value,
		Vars:  make(map[string]any),
	}
	children := vdom.ResolveTree(ci.vdomTemplates, ctx)
	root := &vdom.ElementNode{Tag: "body", Children: children}
	vdom.ComputeDescendants(root)
	return root
}
