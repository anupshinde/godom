package godom

// vdomBuildInit builds the initial VDomMessage for a new client connection.
// It resolves the template tree against current state, renders to HTML,
// collects events, and stores the tree for future diffing.
func vdomBuildInit(ci *componentInfo) *VDomMessage {
	// Build node tree from templates + current state
	gid := &gidCounter{seq: ci.gidSeq}
	tree := vdomBuildTree(ci)

	// Render to HTML and collect events
	htmlStr, events := renderToHTMLWithEvents(tree.Children, gid)

	// Store for future diffing
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
		// No previous tree — shouldn't happen, but fall back to init-style
		htmlStr, events := renderToHTMLWithEvents(newTree.Children, gid)
		ci.prevTree = newTree
		ci.gidSeq = gid.seq
		return encodeInitMessage(htmlStr, events)
	}

	patches := diff(ci.prevTree, newTree)
	if len(patches) == 0 {
		return nil
	}

	msg := encodePatchMessage(patches, gid)
	ci.prevTree = newTree
	ci.gidSeq = gid.seq
	return msg
}

// vdomBuildTree resolves the template tree against the current component state.
func vdomBuildTree(ci *componentInfo) *ElementNode {
	ctx := &resolveContext{
		State: ci.value, // reflect.Value pointing to the user's struct
		Vars:  make(map[string]any),
	}
	children := resolveTree(ci.vdomTemplates, ctx)
	root := &ElementNode{Tag: "body", Children: children}
	computeDescendants(root)
	return root
}
