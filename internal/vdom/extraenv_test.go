package vdom

import (
	"reflect"
	"testing"
)

type extraEnvState struct {
	Name string
}

// Method that collides with an ExtraEnv key, to prove struct members win.
func (s *extraEnvState) Tag() string { return "from-method" }

// ExtraEnv functions must be callable from expressions (this is how the task
// bindings Busy/Progress/Err/Crashed reach templates), and a struct field or
// method of the same name must take precedence for backward compatibility.
func TestResolveExpr_ExtraEnvFunctionsAndPrecedence(t *testing.T) {
	state := &extraEnvState{Name: "x"}
	ctx := &ResolveContext{
		State: reflect.ValueOf(state),
		ExtraEnv: map[string]any{
			"Busy": func(name string) bool { return name == "search" },
			"Tag":  func() string { return "from-extraenv" }, // collides with method
		},
	}

	if got := ResolveExpr("Busy('search')", ctx); got != true {
		t.Errorf("Busy('search') via ExtraEnv = %v, want true", got)
	}
	if got := ResolveExpr("Busy('other')", ctx); got != false {
		t.Errorf("Busy('other') via ExtraEnv = %v, want false", got)
	}
	// The struct method must win over the colliding ExtraEnv entry.
	if got := ResolveExpr("Tag()", ctx); got != "from-method" {
		t.Errorf("collision precedence: Tag() = %v, want struct method to win", got)
	}
}
