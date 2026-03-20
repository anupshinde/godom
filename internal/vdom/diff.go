package vdom

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Diff computes the minimal set of patches needed to transform oldTree into newTree.
// Both trees must have had ComputeDescendants called on them before diffing.
func Diff(oldTree, newTree Node) []Patch {
	var patches []Patch
	diffHelp(oldTree, newTree, &patches)
	return patches
}

func diffHelp(old, new Node, patches *[]Patch) {
	if old == new {
		return
	}

	if old.NodeType() != new.NodeType() {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.NodeID(),
			Data:   PatchRedrawData{Node: new},
		})
		return
	}

	switch oldN := old.(type) {
	case *TextNode:
		newN := new.(*TextNode)
		diffText(oldN, newN, patches)

	case *ElementNode:
		newN := new.(*ElementNode)
		diffElement(oldN, newN, patches)

	case *KeyedElementNode:
		newN := new.(*KeyedElementNode)
		diffKeyedElement(oldN, newN, patches)

	case *ComponentNode:
		newN := new.(*ComponentNode)
		diffComponent(oldN, newN, patches)

	case *PluginNode:
		newN := new.(*PluginNode)
		diffPlugin(oldN, newN, patches)

	case *LazyNode:
		newN := new.(*LazyNode)
		diffLazy(oldN, newN, patches)
	}
}

// ---------------------------------------------------------------------------
// Text
// ---------------------------------------------------------------------------

func diffText(old, new *TextNode, patches *[]Patch) {
	if old.Text != new.Text {
		*patches = append(*patches, Patch{
			Type:   PatchText,
			NodeID: old.ID,
			Data:   PatchTextData{Text: new.Text},
		})
	}
}

// ---------------------------------------------------------------------------
// Element (non-keyed children)
// ---------------------------------------------------------------------------

func diffElement(old, new *ElementNode, patches *[]Patch) {
	if old.Tag != new.Tag || old.Namespace != new.Namespace {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.ID,
			Data:   PatchRedrawData{Node: new},
		})
		return
	}

	fd := DiffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:   PatchFacts,
			NodeID: old.ID,
			Data:   PatchFactsData{Diff: fd},
		})
	}

	diffChildren(old.Children, new.Children, patches, old.ID)
}

func diffChildren(oldKids, newKids []Node, patches *[]Patch, parentNodeID int) {
	oldLen := len(oldKids)
	newLen := len(newKids)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	for i := 0; i < minLen; i++ {
		diffHelp(oldKids[i], newKids[i], patches)
	}

	if newLen > oldLen {
		*patches = append(*patches, Patch{
			Type:   PatchAppend,
			NodeID: parentNodeID,
			Data:   PatchAppendData{Nodes: newKids[oldLen:]},
		})
	}

	if oldLen > newLen {
		*patches = append(*patches, Patch{
			Type:   PatchRemoveLast,
			NodeID: parentNodeID,
			Data:   PatchRemoveLastData{Count: oldLen - newLen},
		})
	}
}

// ---------------------------------------------------------------------------
// KeyedElement
// ---------------------------------------------------------------------------

func diffKeyedElement(old, new *KeyedElementNode, patches *[]Patch) {
	if old.Tag != new.Tag || old.Namespace != new.Namespace {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.ID,
			Data:   PatchRedrawData{Node: new},
		})
		return
	}

	fd := DiffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:   PatchFacts,
			NodeID: old.ID,
			Data:   PatchFactsData{Diff: fd},
		})
	}

	diffKeyedChildren(old.Children, new.Children, patches, old.ID)
}

func diffKeyedChildren(oldKids, newKids []KeyedChild, patches *[]Patch, parentNodeID int) {
	oldLen := len(oldKids)
	newLen := len(newKids)

	if oldLen == 0 && newLen == 0 {
		return
	}

	// Build map from key → old index for cross-position matching.
	oldKeyIndex := make(map[string]int, oldLen)
	for i, kc := range oldKids {
		oldKeyIndex[kc.Key] = i
	}

	// Build map from key → new index.
	newKeyIndex := make(map[string]int, newLen)
	for i, kc := range newKids {
		newKeyIndex[kc.Key] = i
	}

	// Track which old children are consumed (matched or removed).
	oldUsed := make([]bool, oldLen)

	// For each new child, find its match in old children.
	// matchedOld[newIdx] = oldIdx if matched, -1 if new insert.
	matchedOld := make([]int, newLen)
	for i := range matchedOld {
		matchedOld[i] = -1
	}
	for i, kc := range newKids {
		if oi, ok := oldKeyIndex[kc.Key]; ok {
			matchedOld[i] = oi
			oldUsed[oi] = true
		}
	}

	// Collect removes: old children not present in new list.
	// We process removes in reverse order so that indices stay valid.
	var removes []ReorderRemove
	for i := oldLen - 1; i >= 0; i-- {
		if !oldUsed[i] {
			removes = append(removes, ReorderRemove{Index: i, Key: oldKids[i].Key})
		}
	}

	// Build the old key order after removes are applied.
	var survivingOldKeys []string
	for i := 0; i < oldLen; i++ {
		if oldUsed[i] {
			survivingOldKeys = append(survivingOldKeys, oldKids[i].Key)
		}
	}

	// Determine inserts and moves.
	currentKeys := make([]string, len(survivingOldKeys))
	copy(currentKeys, survivingOldKeys)

	var inserts []ReorderInsert
	var subPatches []Patch

	for ni := 0; ni < newLen; ni++ {
		newKey := newKids[ni].Key
		oi := matchedOld[ni]

		if oi == -1 {
			// Pure insert — new key not in old list.
			inserts = append(inserts, ReorderInsert{
				Index: ni,
				Key:   newKey,
				Node:  newKids[ni].Node,
			})
			currentKeys = sliceInsert(currentKeys, ni, newKey)
			continue
		}

		// Matched. Check if it's already in the right position.
		currentPos := sliceIndexOf(currentKeys, newKey)
		if currentPos == ni {
			// Already in correct position — just diff the nodes.
			diffHelp(oldKids[oi].Node, newKids[ni].Node, &subPatches)
			continue
		}

		// Move: remove from current position, insert at target position.
		currentKeys = sliceRemove(currentKeys, currentPos)
		removes = append(removes, ReorderRemove{Index: currentPos, Key: newKey})
		inserts = append(inserts, ReorderInsert{
			Index: ni,
			Key:   newKey,
		})
		currentKeys = sliceInsert(currentKeys, ni, newKey)

		// Diff the matched nodes.
		diffHelp(oldKids[oi].Node, newKids[ni].Node, &subPatches)
	}

	// Only emit a reorder patch if there are actual changes.
	if len(removes) == 0 && len(inserts) == 0 && len(subPatches) == 0 {
		return
	}

	// If there are no structural changes (no removes, no inserts) just emit sub-patches directly.
	if len(removes) == 0 && len(inserts) == 0 {
		*patches = append(*patches, subPatches...)
		return
	}

	// Sort removes in descending order so indices remain valid during application.
	sortRemovesDesc(removes)

	*patches = append(*patches, Patch{
		Type:   PatchReorder,
		NodeID: parentNodeID,
		Data: PatchReorderData{
			Inserts: inserts,
			Removes: removes,
			Patches: subPatches,
		},
	})
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

func diffComponent(old, new *ComponentNode, patches *[]Patch) {
	if old.Tag != new.Tag {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.ID,
			Data:   PatchRedrawData{Node: new},
		})
		return
	}

	if old.SubTree != nil && new.SubTree != nil {
		diffHelp(old.SubTree, new.SubTree, patches)
	} else if new.SubTree != nil {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.ID,
			Data:   PatchRedrawData{Node: new},
		})
	}
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

func diffPlugin(old, new *PluginNode, patches *[]Patch) {
	if old.Name != new.Name || old.Tag != new.Tag {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.ID,
			Data:   PatchRedrawData{Node: new},
		})
		return
	}

	fd := DiffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:   PatchFacts,
			NodeID: old.ID,
			Data:   PatchFactsData{Diff: fd},
		})
	}

	if !jsonEqual(old.Data, new.Data) {
		*patches = append(*patches, Patch{
			Type:   PatchPlugin,
			NodeID: old.ID,
			Data:   PatchPluginData{Data: new.Data},
		})
	}
}

// ---------------------------------------------------------------------------
// Lazy
// ---------------------------------------------------------------------------

func diffLazy(old, new *LazyNode, patches *[]Patch) {
	if lazyArgsEqual(old, new) {
		new.Cached = old.Cached
		return
	}

	if new.Cached == nil {
		new.Cached = evaluateLazy(new)
	}
	if old.Cached == nil {
		old.Cached = evaluateLazy(old)
	}

	if old.Cached != nil && new.Cached != nil {
		var subPatches []Patch
		diffHelp(old.Cached, new.Cached, &subPatches)
		if len(subPatches) > 0 {
			*patches = append(*patches, Patch{
				Type:   PatchLazy,
				NodeID: old.ID,
				Data:   PatchLazyData{Patches: subPatches},
			})
		}
	} else if new.Cached != nil {
		*patches = append(*patches, Patch{
			Type:   PatchRedraw,
			NodeID: old.ID,
			Data:   PatchRedrawData{Node: new.Cached},
		})
	}
}

func lazyArgsEqual(old, new *LazyNode) bool {
	if len(old.Args) != len(new.Args) {
		return false
	}
	if old.Func == nil || new.Func == nil {
		if old.Func != new.Func {
			return false
		}
	} else if reflect.ValueOf(old.Func).Pointer() != reflect.ValueOf(new.Func).Pointer() {
		return false
	}
	for i := range old.Args {
		if !refEqual(old.Args[i], new.Args[i]) {
			return false
		}
	}
	return true
}

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
		return a == b
	}
}

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

// DiffFacts computes the difference between two Facts structs.
func DiffFacts(old, new *Facts) FactsDiff {
	var d FactsDiff

	d.Props = diffMapAny(old.Props, new.Props)
	d.Attrs = diffMapString(old.Attrs, new.Attrs)
	d.AttrsNS = diffMapNSAttr(old.AttrsNS, new.AttrsNS)
	d.Styles = diffMapString(old.Styles, new.Styles)
	d.Events = diffEvents(old.Events, new.Events)

	return d
}

func diffMapAny(old, new map[string]any) map[string]any {
	if len(old) == 0 && len(new) == 0 {
		return nil
	}

	var diff map[string]any

	for k, oldVal := range old {
		newVal, exists := new[k]
		if !exists {
			if diff == nil {
				diff = make(map[string]any)
			}
			diff[k] = nil
		} else if !valEqual(oldVal, newVal) {
			if diff == nil {
				diff = make(map[string]any)
			}
			diff[k] = newVal
		}
	}

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
			diff[k] = ""
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
			diff[k] = NSAttr{}
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
			diff[k] = nil
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

func valEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
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
	return reflect.DeepEqual(a, b)
}

func eventHandlerEqual(a, b EventHandler) bool {
	return a.Handler == b.Handler &&
		a.Scope == b.Scope &&
		a.Options == b.Options &&
		argsEqual(a.Args, b.Args)
}

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

// ---------------------------------------------------------------------------
// Slice helpers for simulating DOM order
// ---------------------------------------------------------------------------

func sliceInsert(s []string, i int, val string) []string {
	if i >= len(s) {
		return append(s, val)
	}
	s = append(s, "")
	copy(s[i+1:], s[i:])
	s[i] = val
	return s
}

func sliceRemove(s []string, i int) []string {
	return append(s[:i], s[i+1:]...)
}

func sliceIndexOf(s []string, val string) int {
	for i, v := range s {
		if v == val {
			return i
		}
	}
	return -1
}

func sortRemovesDesc(removes []ReorderRemove) {
	// Simple insertion sort — remove lists are typically small.
	for i := 1; i < len(removes); i++ {
		j := i
		for j > 0 && removes[j].Index > removes[j-1].Index {
			removes[j], removes[j-1] = removes[j-1], removes[j]
			j--
		}
	}
}
