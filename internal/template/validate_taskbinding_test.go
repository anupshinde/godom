package template

import (
	"testing"

	"github.com/anupshinde/godom/internal/island"
)

// The §2 task-state bindings are engine-provided (ExtraEnv) functions, not
// methods. The validator must accept them in the directives where they're used —
// otherwise a template that follows §2 ("bind Busy(name) instead of a Loading
// field") fatals at Register. Regression for the validator/renderer mismatch.
func TestValidateDirectives_AcceptsTaskBindings(t *testing.T) {
	valid := []string{
		`<div g-text="Busy('search')"></div>`,
		`<div g-if="Busy('search')"></div>`,
		`<div g-class:on="Busy('search')"></div>`,
		`<div g-class:off="!Busy('search')"></div>`,
		`<p g-text="Progress('search')"></p>`,
		`<p g-text="Err('search')"></p>`,
		`<p g-if="Crashed('search')"></p>`,
	}
	for _, html := range valid {
		if err := ValidateDirectives(html, newValTestCI()); err != nil {
			t.Errorf("task binding should validate: %s\n  got: %v", html, err)
		}
	}
}

// A non-reserved unknown callable must still be rejected — the fix must not
// blanket-accept every parenthesised name.
func TestValidateDirectives_StillRejectsUnknownCallable(t *testing.T) {
	if err := ValidateDirectives(`<div g-text="Bogus('x')"></div>`, newValTestCI()); err == nil {
		t.Error("an unknown callable must still be rejected")
	}
}

func TestIsReservedBinding(t *testing.T) {
	for _, n := range island.ReservedBindingNames {
		if !island.IsReservedBinding(n) {
			t.Errorf("%q should be a reserved binding", n)
		}
	}
	if island.IsReservedBinding("Save") {
		t.Error("an ordinary method name must not be a reserved binding")
	}
}

// Ternary expressions resolve at runtime via expr-lang, so the validator must
// accept them instead of treating the whole string as a bare field name. The
// §2 binding pattern `Busy('x') ? 'a' : 'b'` relies on this.
func TestValidateDirectives_AcceptsTernary(t *testing.T) {
	valid := []string{
		`<span g-text="Visible ? 'yes' : 'no'"></span>`,
		`<button g-text="Busy('search') ? 'Working' : 'Run'"></button>`,
		`<button g-attr:disabled="Busy('search')"></button>`,
	}
	for _, html := range valid {
		if err := ValidateDirectives(html, newValTestCI()); err != nil {
			t.Errorf("should validate: %s\n  got: %v", html, err)
		}
	}
	// A bare unknown field is still rejected (the fix didn't blanket-accept).
	if err := ValidateDirectives(`<span g-text="Nope"></span>`, newValTestCI()); err == nil {
		t.Error("an unknown bare field must still be rejected")
	}
}
