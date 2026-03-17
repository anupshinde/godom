package godom

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// diff computes the minimal set of patches needed to transform oldTree into newTree.
// Both trees must have had computeDescendants called on them before diffing.
func diff(oldTree, newTree Node) []Patch {
	var patches []Patch
	diffHelp(oldTree, newTree, &patches, 0)
	return patches
}

// diffHelp is the recursive core of the diff algorithm.
// index is the current position in a depth-first traversal of the old tree.
func diffHelp(old, new Node, patches *[]Patch, index int) {
	// Identity check — same pointer means no changes.
	if old == new {
		return
	}

	// Different node types → full redraw.
	if old.nodeType() != new.nodeType() {
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	switch oldN := old.(type) {
	case *TextNode:
		newN := new.(*TextNode)
		diffText(oldN, newN, patches, index)

	case *ElementNode:
		newN := new.(*ElementNode)
		diffElement(oldN, newN, patches, index)

	case *KeyedElementNode:
		newN := new.(*KeyedElementNode)
		diffKeyedElement(oldN, newN, patches, index)

	case *ComponentNode:
		newN := new.(*ComponentNode)
		diffComponent(oldN, newN, patches, index)

	case *PluginNode:
		newN := new.(*PluginNode)
		diffPlugin(oldN, newN, patches, index)

	case *LazyNode:
		newN := new.(*LazyNode)
		diffLazy(oldN, newN, patches, index)
	}
}

// ---------------------------------------------------------------------------
// Text
// ---------------------------------------------------------------------------

func diffText(old, new *TextNode, patches *[]Patch, index int) {
	if old.Text != new.Text {
		*patches = append(*patches, Patch{
			Type:  patchText,
			Index: index,
			Data:  PatchTextData{Text: new.Text},
		})
	}
}

// ---------------------------------------------------------------------------
// Element (non-keyed children)
// ---------------------------------------------------------------------------

func diffElement(old, new *ElementNode, patches *[]Patch, index int) {
	// Different tag or namespace → full redraw.
	if old.Tag != new.Tag || old.Namespace != new.Namespace {
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	// Diff facts (properties, attributes, styles, events).
	fd := diffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:  patchFacts,
			Index: index,
			Data:  PatchFactsData{Diff: fd},
		})
	}

	// Diff children.
	diffChildren(old.Children, new.Children, patches, index)
}

// diffChildren diffs two non-keyed child lists.
func diffChildren(oldKids, newKids []Node, patches *[]Patch, parentIndex int) {
	oldLen := len(oldKids)
	newLen := len(newKids)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	// Diff common prefix — each child's index is parent + 1 + sum of preceding descendants.
	childIndex := parentIndex
	for i := 0; i < minLen; i++ {
		childIndex++
		diffHelp(oldKids[i], newKids[i], patches, childIndex)
		childIndex += oldKids[i].descendantsCount()
	}

	// New children appended.
	if newLen > oldLen {
		*patches = append(*patches, Patch{
			Type:  patchAppend,
			Index: parentIndex,
			Data:  PatchAppendData{Nodes: newKids[oldLen:]},
		})
	}

	// Old children removed from end.
	if oldLen > newLen {
		*patches = append(*patches, Patch{
			Type:  patchRemoveLast,
			Index: parentIndex,
			Data:  PatchRemoveLastData{Count: oldLen - newLen},
		})
	}
}

// ---------------------------------------------------------------------------
// KeyedElement
// ---------------------------------------------------------------------------

func diffKeyedElement(old, new *KeyedElementNode, patches *[]Patch, index int) {
	// Different tag or namespace → full redraw.
	if old.Tag != new.Tag || old.Namespace != new.Namespace {
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	// Diff facts.
	fd := diffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:  patchFacts,
			Index: index,
			Data:  PatchFactsData{Diff: fd},
		})
	}

	// For now, keyed diffing falls back to a simple approach:
	// compare by position, redraw if keys don't match.
	// Full keyed diffing (Phase 5) will replace this with the
	// two-pointer lookahead algorithm producing Reorder patches.
	diffKeyedChildrenSimple(old.Children, new.Children, patches, index)
}

// diffKeyedChildrenSimple is a temporary placeholder that diffs keyed children
// by position. If keys match, diff the subtrees. If they don't, redraw.
// Phase 5 replaces this with proper keyed reordering.
func diffKeyedChildrenSimple(oldKids, newKids []KeyedChild, patches *[]Patch, parentIndex int) {
	oldLen := len(oldKids)
	newLen := len(newKids)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	childIndex := parentIndex
	for i := 0; i < minLen; i++ {
		childIndex++
		if oldKids[i].Key != newKids[i].Key {
			// Different key at same position → redraw this child.
			*patches = append(*patches, Patch{
				Type:  patchRedraw,
				Index: childIndex,
				Data:  PatchRedrawData{Node: newKids[i].Node},
			})
		} else {
			diffHelp(oldKids[i].Node, newKids[i].Node, patches, childIndex)
		}
		childIndex += oldKids[i].Node.descendantsCount()
	}

	if newLen > oldLen {
		appendNodes := make([]Node, newLen-oldLen)
		for i := oldLen; i < newLen; i++ {
			appendNodes[i-oldLen] = newKids[i].Node
		}
		*patches = append(*patches, Patch{
			Type:  patchAppend,
			Index: parentIndex,
			Data:  PatchAppendData{Nodes: appendNodes},
		})
	}

	if oldLen > newLen {
		*patches = append(*patches, Patch{
			Type:  patchRemoveLast,
			Index: parentIndex,
			Data:  PatchRemoveLastData{Count: oldLen - newLen},
		})
	}
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

func diffComponent(old, new *ComponentNode, patches *[]Patch, index int) {
	// Different component type → full redraw.
	if old.Tag != new.Tag {
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	// If both have subtrees, diff them.
	if old.SubTree != nil && new.SubTree != nil {
		diffHelp(old.SubTree, new.SubTree, patches, index)
	} else if new.SubTree != nil {
		// Old had no subtree, new does → redraw.
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
	}
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

func diffPlugin(old, new *PluginNode, patches *[]Patch, index int) {
	// Different plugin or tag → full redraw.
	if old.Name != new.Name || old.Tag != new.Tag {
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	// Diff facts.
	fd := diffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:  patchFacts,
			Index: index,
			Data:  PatchFactsData{Diff: fd},
		})
	}

	// Diff plugin data (JSON comparison).
	if !jsonEqual(old.Data, new.Data) {
		*patches = append(*patches, Patch{
			Type:  patchPlugin,
			Index: index,
			Data:  PatchPluginData{Data: new.Data},
		})
	}
}

// ---------------------------------------------------------------------------
// Lazy
// ---------------------------------------------------------------------------

func diffLazy(old, new *LazyNode, patches *[]Patch, index int) {
	// Check referential equality of function and args.
	if lazyArgsEqual(old, new) {
		// Inputs unchanged — reuse cached result.
		new.Cached = old.Cached
		return
	}

	// Inputs changed — force evaluation of the new lazy node.
	// The caller must have already evaluated new.Cached by calling the Func.
	// If Cached is nil, we need to evaluate it now.
	if new.Cached == nil {
		new.Cached = evaluateLazy(new)
	}
	if old.Cached == nil {
		old.Cached = evaluateLazy(old)
	}

	// Diff the cached results.
	if old.Cached != nil && new.Cached != nil {
		var subPatches []Patch
		diffHelp(old.Cached, new.Cached, &subPatches, index)
		if len(subPatches) > 0 {
			*patches = append(*patches, Patch{
				Type:  patchLazy,
				Index: index,
				Data:  PatchLazyData{Patches: subPatches},
			})
		}
	} else if new.Cached != nil {
		*patches = append(*patches, Patch{
			Type:  patchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new.Cached},
		})
	}
}

// lazyArgsEqual checks if two lazy nodes have the same function and arguments
// by reference equality (pointer comparison).
func lazyArgsEqual(old, new *LazyNode) bool {
	if len(old.Args) != len(new.Args) {
		return false
	}
	// Compare function pointer.
	if reflect.ValueOf(old.Func).Pointer() != reflect.ValueOf(new.Func).Pointer() {
		return false
	}
	// Compare each argument by pointer/value identity.
	for i := range old.Args {
		if !refEqual(old.Args[i], new.Args[i]) {
			return false
		}
	}
	return true
}

// refEqual checks reference equality for lazy argument comparison.
func refEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if va.Type() != vb.Type() {
		return false
	}
	switch va.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return va.Pointer() == vb.Pointer()
	default:
		// Value types: use == comparison.
		return a == b
	}
}

// evaluateLazy calls a lazy node's function with its args.
// Returns nil if the function can't be called.
func evaluateLazy(n *LazyNode) Node {
	if n.Func == nil {
		return nil
	}
	fn := reflect.ValueOf(n.Func)
	if fn.Kind() != reflect.Func {
		return nil
	}
	args := make([]reflect.Value, len(n.Args))
	for i, a := range n.Args {
		args[i] = reflect.ValueOf(a)
	}
	results := fn.Call(args)
	if len(results) == 0 {
		return nil
	}
	if node, ok := results[0].Interface().(Node); ok {
		return node
	}
	return nil
}

// ---------------------------------------------------------------------------
// Facts diffing
// ---------------------------------------------------------------------------

// diffFacts computes the difference between two Facts structs.
func diffFacts(old, new *Facts) FactsDiff {
	var d FactsDiff

	d.Props = diffMapAny(old.Props, new.Props)
	d.Attrs = diffMapString(old.Attrs, new.Attrs)
	d.AttrsNS = diffMapNSAttr(old.AttrsNS, new.AttrsNS)
	d.Styles = diffMapString(old.Styles, new.Styles)
	d.Events = diffEvents(old.Events, new.Events)

	return d
}

// diffMapAny diffs two map[string]any — returns changes only.
func diffMapAny(old, new map[string]any) map[string]any {
	if len(old) == 0 && len(new) == 0 {
		return nil
	}

	var diff map[string]any

	// Check for removals and changes.
	for k, oldVal := range old {
		newVal, exists := new[k]
		if !exists {
			// Removed.
			if diff == nil {
				diff = make(map[string]any)
			}
			diff[k] = nil
		} else if !valEqual(oldVal, newVal) {
			// Changed.
			if diff == nil {
				diff = make(map[string]any)
			}
			diff[k] = newVal
		}
	}

	// Check for additions.
	for k, newVal := range new {
		if _, exists := old[k]; !exists {
			if diff == nil {
				diff = make(map[string]any)
			}
			diff[k] = newVal
		}
	}

	return diff
}

// diffMapString diffs two map[string]string.
func diffMapString(old, new map[string]string) map[string]string {
	if len(old) == 0 && len(new) == 0 {
		return nil
	}

	var diff map[string]string

	for k, oldVal := range old {
		newVal, exists := new[k]
		if !exists {
			if diff == nil {
				diff = make(map[string]string)
			}
			diff[k] = "" // empty = remove
		} else if oldVal != newVal {
			if diff == nil {
				diff = make(map[string]string)
			}
			diff[k] = newVal
		}
	}

	for k, newVal := range new {
		if _, exists := old[k]; !exists {
			if diff == nil {
				diff = make(map[string]string)
			}
			diff[k] = newVal
		}
	}

	return diff
}

// diffMapNSAttr diffs two map[string]NSAttr.
func diffMapNSAttr(old, new map[string]NSAttr) map[string]NSAttr {
	if len(old) == 0 && len(new) == 0 {
		return nil
	}

	var diff map[string]NSAttr

	for k, oldVal := range old {
		newVal, exists := new[k]
		if !exists {
			if diff == nil {
				diff = make(map[string]NSAttr)
			}
			diff[k] = NSAttr{} // empty = remove
		} else if oldVal != newVal {
			if diff == nil {
				diff = make(map[string]NSAttr)
			}
			diff[k] = newVal
		}
	}

	for k, newVal := range new {
		if _, exists := old[k]; !exists {
			if diff == nil {
				diff = make(map[string]NSAttr)
			}
			diff[k] = newVal
		}
	}

	return diff
}

// diffEvents diffs two event handler maps.
func diffEvents(old, new map[string]EventHandler) map[string]*EventHandler {
	if len(old) == 0 && len(new) == 0 {
		return nil
	}

	var diff map[string]*EventHandler

	for k := range old {
		newVal, exists := new[k]
		if !exists {
			if diff == nil {
				diff = make(map[string]*EventHandler)
			}
			diff[k] = nil // nil = remove
		} else if !eventHandlerEqual(old[k], newVal) {
			if diff == nil {
				diff = make(map[string]*EventHandler)
			}
			v := newVal
			diff[k] = &v
		}
	}

	for k := range new {
		if _, exists := old[k]; !exists {
			if diff == nil {
				diff = make(map[string]*EventHandler)
			}
			v := new[k]
			diff[k] = &v
		}
	}

	return diff
}

// ---------------------------------------------------------------------------
// Equality helpers
// ---------------------------------------------------------------------------

// valEqual compares two property values for equality.
func valEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Fast path for common types.
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	}
	// Fallback: reflect.DeepEqual.
	return reflect.DeepEqual(a, b)
}

// eventHandlerEqual compares two event handlers.
func eventHandlerEqual(a, b EventHandler) bool {
	return a.Handler == b.Handler &&
		a.Scope == b.Scope &&
		a.Options == b.Options &&
		argsEqual(a.Args, b.Args)
}

// argsEqual compares two argument slices.
func argsEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !valEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// jsonEqual compares two values by JSON representation.
// Used for plugin data comparison.
func jsonEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return fmt.Sprint(a) == fmt.Sprint(b)
	}
	return string(ja) == string(jb)
}
