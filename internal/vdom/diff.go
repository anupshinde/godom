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
	diffHelp(oldTree, newTree, &patches, 0)
	return patches
}

func diffHelp(old, new Node, patches *[]Patch, index int) {
	if old == new {
		return
	}

	if old.NodeType() != new.NodeType() {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
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
			Type:  PatchText,
			Index: index,
			Data:  PatchTextData{Text: new.Text},
		})
	}
}

// ---------------------------------------------------------------------------
// Element (non-keyed children)
// ---------------------------------------------------------------------------

func diffElement(old, new *ElementNode, patches *[]Patch, index int) {
	if old.Tag != new.Tag || old.Namespace != new.Namespace {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	fd := DiffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:  PatchFacts,
			Index: index,
			Data:  PatchFactsData{Diff: fd},
		})
	}

	diffChildren(old.Children, new.Children, patches, index)
}

func diffChildren(oldKids, newKids []Node, patches *[]Patch, parentIndex int) {
	oldLen := len(oldKids)
	newLen := len(newKids)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	childIndex := parentIndex
	for i := 0; i < minLen; i++ {
		childIndex++
		diffHelp(oldKids[i], newKids[i], patches, childIndex)
		childIndex += oldKids[i].DescendantsCount()
	}

	if newLen > oldLen {
		*patches = append(*patches, Patch{
			Type:  PatchAppend,
			Index: parentIndex,
			Data:  PatchAppendData{Nodes: newKids[oldLen:]},
		})
	}

	if oldLen > newLen {
		*patches = append(*patches, Patch{
			Type:  PatchRemoveLast,
			Index: parentIndex,
			Data:  PatchRemoveLastData{Count: oldLen - newLen},
		})
	}
}

// ---------------------------------------------------------------------------
// KeyedElement
// ---------------------------------------------------------------------------

func diffKeyedElement(old, new *KeyedElementNode, patches *[]Patch, index int) {
	if old.Tag != new.Tag || old.Namespace != new.Namespace {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	fd := DiffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:  PatchFacts,
			Index: index,
			Data:  PatchFactsData{Diff: fd},
		})
	}

	diffKeyedChildrenSimple(old.Children, new.Children, patches, index)
}

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
			*patches = append(*patches, Patch{
				Type:  PatchRedraw,
				Index: childIndex,
				Data:  PatchRedrawData{Node: newKids[i].Node},
			})
		} else {
			diffHelp(oldKids[i].Node, newKids[i].Node, patches, childIndex)
		}
		childIndex += oldKids[i].Node.DescendantsCount()
	}

	if newLen > oldLen {
		appendNodes := make([]Node, newLen-oldLen)
		for i := oldLen; i < newLen; i++ {
			appendNodes[i-oldLen] = newKids[i].Node
		}
		*patches = append(*patches, Patch{
			Type:  PatchAppend,
			Index: parentIndex,
			Data:  PatchAppendData{Nodes: appendNodes},
		})
	}

	if oldLen > newLen {
		*patches = append(*patches, Patch{
			Type:  PatchRemoveLast,
			Index: parentIndex,
			Data:  PatchRemoveLastData{Count: oldLen - newLen},
		})
	}
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

func diffComponent(old, new *ComponentNode, patches *[]Patch, index int) {
	if old.Tag != new.Tag {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	if old.SubTree != nil && new.SubTree != nil {
		diffHelp(old.SubTree, new.SubTree, patches, index)
	} else if new.SubTree != nil {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
	}
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

func diffPlugin(old, new *PluginNode, patches *[]Patch, index int) {
	if old.Name != new.Name || old.Tag != new.Tag {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new},
		})
		return
	}

	fd := DiffFacts(&old.Facts, &new.Facts)
	if !fd.IsEmpty() {
		*patches = append(*patches, Patch{
			Type:  PatchFacts,
			Index: index,
			Data:  PatchFactsData{Diff: fd},
		})
	}

	if !jsonEqual(old.Data, new.Data) {
		*patches = append(*patches, Patch{
			Type:  PatchPlugin,
			Index: index,
			Data:  PatchPluginData{Data: new.Data},
		})
	}
}

// ---------------------------------------------------------------------------
// Lazy
// ---------------------------------------------------------------------------

func diffLazy(old, new *LazyNode, patches *[]Patch, index int) {
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
		diffHelp(old.Cached, new.Cached, &subPatches, index)
		if len(subPatches) > 0 {
			*patches = append(*patches, Patch{
				Type:  PatchLazy,
				Index: index,
				Data:  PatchLazyData{Patches: subPatches},
			})
		}
	} else if new.Cached != nil {
		*patches = append(*patches, Patch{
			Type:  PatchRedraw,
			Index: index,
			Data:  PatchRedrawData{Node: new.Cached},
		})
	}
}

func lazyArgsEqual(old, new *LazyNode) bool {
	if len(old.Args) != len(new.Args) {
		return false
	}
	if reflect.ValueOf(old.Func).Pointer() != reflect.ValueOf(new.Func).Pointer() {
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
