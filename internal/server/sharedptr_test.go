package server

import (
	"reflect"
	"testing"

	"github.com/anupshinde/godom/internal/component"
)

// --- buildSharedPtrMaps tests ---

// Shared state type used by two components.
type sharedState struct {
	Score int
}

// compA embeds *sharedState as an anonymous pointer field.
type compA struct {
	Component struct{}
	*sharedState
	Name string
}

// compB also embeds *sharedState — sharing the pointer with compA.
type compB struct {
	Component struct{}
	*sharedState
	Label string
}

// compNoShared has no embedded pointer fields.
type compNoShared struct {
	Component struct{}
	Title string
}

// compNilPtr has an embedded pointer that is nil.
type compNilPtr struct {
	Component struct{}
	*sharedState
}

func makeCI(app interface{}) *component.Info {
	v := reflect.ValueOf(app)
	return &component.Info{
		Value: v,
		Typ:   v.Elem().Type(),
	}
}

func TestBuildSharedPtrMaps_TwoComponentsSharingPointer(t *testing.T) {
	shared := &sharedState{Score: 42}
	a := &compA{sharedState: shared, Name: "A"}
	b := &compB{sharedState: shared, Label: "B"}

	ciA := makeCI(a)
	ciB := makeCI(b)
	comps := []*component.Info{ciA, ciB}

	sm := buildSharedPtrMaps(comps)

	// Both components should be in ptrToCompIdx for the shared pointer.
	if len(sm.ptrToCompIdx) != 1 {
		t.Fatalf("expected 1 shared pointer, got %d", len(sm.ptrToCompIdx))
	}
	for _, idxs := range sm.ptrToCompIdx {
		if len(idxs) != 2 {
			t.Errorf("expected 2 components sharing pointer, got %d", len(idxs))
		}
	}
	// Both components should appear in compIdxToPtr.
	if len(sm.compIdxToPtr) != 2 {
		t.Errorf("expected 2 entries in compIdxToPtr, got %d", len(sm.compIdxToPtr))
	}
}

func TestBuildSharedPtrMaps_NoSharing(t *testing.T) {
	a := &compA{sharedState: &sharedState{Score: 1}, Name: "A"}
	b := &compB{sharedState: &sharedState{Score: 2}, Label: "B"}

	ciA := makeCI(a)
	ciB := makeCI(b)
	comps := []*component.Info{ciA, ciB}

	sm := buildSharedPtrMaps(comps)

	// Different pointers → no sharing → maps should be empty after pruning.
	if len(sm.ptrToCompIdx) != 0 {
		t.Errorf("expected 0 shared pointers, got %d", len(sm.ptrToCompIdx))
	}
	if len(sm.compIdxToPtr) != 0 {
		t.Errorf("expected 0 compIdxToPtr entries, got %d", len(sm.compIdxToPtr))
	}
}

func TestBuildSharedPtrMaps_NoEmbeddedPointers(t *testing.T) {
	a := &compNoShared{Title: "hello"}
	ci := makeCI(a)
	comps := []*component.Info{ci}

	sm := buildSharedPtrMaps(comps)

	if len(sm.ptrToCompIdx) != 0 {
		t.Errorf("expected 0 shared pointers, got %d", len(sm.ptrToCompIdx))
	}
}

func TestBuildSharedPtrMaps_NilEmbeddedPointer(t *testing.T) {
	a := &compNilPtr{} // sharedState is nil
	ci := makeCI(a)
	comps := []*component.Info{ci}

	sm := buildSharedPtrMaps(comps)

	if len(sm.ptrToCompIdx) != 0 {
		t.Errorf("expected 0 shared pointers for nil embedded ptr, got %d", len(sm.ptrToCompIdx))
	}
}

func TestBuildSharedPtrMaps_EmptyComps(t *testing.T) {
	sm := buildSharedPtrMaps(nil)

	if len(sm.ptrToCompIdx) != 0 || len(sm.compIdxToPtr) != 0 {
		t.Error("expected empty maps for nil comps")
	}
}

func TestRemovePtr(t *testing.T) {
	ptrs := []uintptr{10, 20, 30, 40}

	result := removePtr(ptrs, 20)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	for _, p := range result {
		if p == 20 {
			t.Error("expected 20 to be removed")
		}
	}
}

func TestRemovePtr_NotPresent(t *testing.T) {
	ptrs := []uintptr{10, 20, 30}
	result := removePtr(ptrs, 99)
	if len(result) != 3 {
		t.Errorf("expected 3 elements unchanged, got %d", len(result))
	}
}

func TestRemovePtr_Empty(t *testing.T) {
	result := removePtr(nil, 5)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestRefreshSharedComponents_PropagatesRefresh(t *testing.T) {
	shared := &sharedState{Score: 10}
	a := &compA{sharedState: shared, Name: "A"}
	b := &compB{sharedState: shared, Label: "B"}

	ciA := makeCI(a)
	ciB := makeCI(b)
	comps := []*component.Info{ciA, ciB}

	sm := buildSharedPtrMaps(comps)
	sm.pool = &connPool{}

	// Track whether RefreshFn is called on sibling.
	var refreshedB bool
	ciB.RefreshFn = func() { refreshedB = true }
	ciA.RefreshFn = func() {} // shouldn't be called (self is skipped)

	sm.refreshSharedComponents(0, []string{"Score"})

	if !refreshedB {
		t.Error("expected RefreshFn to be called on sibling component B")
	}
}

func TestRefreshSharedComponents_NilSM(t *testing.T) {
	// Should not panic with nil receiver.
	var sm *sharedPtrMaps
	sm.refreshSharedComponents(0, []string{"X"})
}

func TestRefreshSharedComponents_EmptyChangedFields(t *testing.T) {
	sm := &sharedPtrMaps{}
	// Should return early without panic.
	sm.refreshSharedComponents(0, nil)
	sm.refreshSharedComponents(0, []string{})
}

func TestRefreshSharedComponents_NoSharedPtrs(t *testing.T) {
	sm := &sharedPtrMaps{
		ptrToCompIdx: make(map[uintptr][]int),
		compIdxToPtr: make(map[int][]uintptr),
	}
	// Component 0 has no shared pointers → should return early.
	sm.refreshSharedComponents(0, []string{"X"})
}

func TestRefreshSharedComponents_SkipsSelf(t *testing.T) {
	shared := &sharedState{Score: 5}
	a := &compA{sharedState: shared, Name: "A"}
	b := &compB{sharedState: shared, Label: "B"}

	ciA := makeCI(a)
	ciB := makeCI(b)
	comps := []*component.Info{ciA, ciB}

	sm := buildSharedPtrMaps(comps)
	sm.pool = &connPool{}

	var calledA, calledB bool
	ciA.RefreshFn = func() { calledA = true }
	ciB.RefreshFn = func() { calledB = true }

	// Refresh from comp 0 (A) should only call B's RefreshFn, not A's.
	sm.refreshSharedComponents(0, []string{"Score"})

	if calledA {
		t.Error("RefreshFn should not be called on the originating component")
	}
	if !calledB {
		t.Error("RefreshFn should be called on sibling component B")
	}
}
