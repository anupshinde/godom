package server

import (
	"reflect"

	"github.com/anupshinde/godom/internal/island"
)

// sharedPtrMaps holds the pointer-sharing relationships between components.
// Built once at startup, used to propagate refreshes to sibling components
// that share embedded pointer fields (e.g. *CounterState).
type sharedPtrMaps struct {
	ptrToCompIdx map[uintptr][]int // pointer address → component indices sharing it
	compIdxToPtr map[int][]uintptr // component index → pointer addresses it holds
	comps        []*island.Info
	pool         *connPool
}

// buildSharedPtrMaps walks all component structs to find embedded pointer fields
// and groups components that share the same pointer address.
func buildSharedPtrMaps(comps []*island.Info) *sharedPtrMaps {
	sm := &sharedPtrMaps{
		ptrToCompIdx: make(map[uintptr][]int),
		compIdxToPtr: make(map[int][]uintptr),
		comps:        comps,
	}
	for idx, ci := range comps {
		v := ci.Value.Elem() // the struct value
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.Anonymous || f.Type.Kind() != reflect.Ptr {
				continue
			}
			fv := v.Field(i)
			if fv.IsNil() {
				continue
			}
			ptr := fv.Pointer()
			sm.ptrToCompIdx[ptr] = append(sm.ptrToCompIdx[ptr], idx)
			sm.compIdxToPtr[idx] = append(sm.compIdxToPtr[idx], ptr)
		}
	}
	// Remove entries where only one component holds the pointer (no sharing).
	for ptr, idxs := range sm.ptrToCompIdx {
		if len(idxs) <= 1 {
			delete(sm.ptrToCompIdx, ptr)
			for _, idx := range idxs {
				sm.compIdxToPtr[idx] = removePtr(sm.compIdxToPtr[idx], ptr)
				if len(sm.compIdxToPtr[idx]) == 0 {
					delete(sm.compIdxToPtr, idx)
				}
			}
		}
	}
	return sm
}

func removePtr(ptrs []uintptr, target uintptr) []uintptr {
	result := ptrs[:0]
	for _, p := range ptrs {
		if p != target {
			result = append(result, p)
		}
	}
	return result
}

// refreshSharedIslands triggers surgical refresh on all other components
// that share an embedded pointer with the given component, using the changed
// field names extracted from the original component's patches.
func (sm *sharedPtrMaps) refreshSharedIslands(compIdx int, changedFields []string) {
	if sm == nil || len(changedFields) == 0 {
		return
	}
	ptrs := sm.compIdxToPtr[compIdx]
	if len(ptrs) == 0 {
		return
	}
	seen := map[int]bool{compIdx: true} // skip self
	for _, ptr := range ptrs {
		for _, sibIdx := range sm.ptrToCompIdx[ptr] {
			if seen[sibIdx] {
				continue
			}
			seen[sibIdx] = true
			sib := sm.comps[sibIdx]
			sib.AddMarkedFields(changedFields...)
			sib.RefreshFn()
		}
	}
}
