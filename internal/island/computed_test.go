package island

import (
	"fmt"
	"reflect"
	"testing"
)

type cartState struct {
	Qty             int
	UnitPrice       int
	Subtotal        int
	SubtotalText    string
	CheckoutEnabled bool
}

func computedCI(s any) *Info {
	v := reflect.ValueOf(s)
	return &Info{Value: v, Typ: v.Elem().Type()}
}

func cartComputeds(c *cartState) []ComputedDef {
	return []ComputedDef{
		{Name: "Subtotal", Fn: func() any { return c.Qty * c.UnitPrice }, Deps: []string{"Qty", "UnitPrice"}},
		{Name: "SubtotalText", Fn: func() any { return fmt.Sprintf("$%d", c.Subtotal) }, Deps: []string{"Subtotal"}},
		{Name: "CheckoutEnabled", Fn: func() any { return c.Qty > 0 }, Deps: []string{"Qty"}},
	}
}

// Register seeds initial values, and a marked input expands to every reachable
// computed, recomputing them in dependency order (so SubtotalText sees the fresh
// Subtotal). The returned expanded set drives the surgical patch.
func TestComputed_ExpandRecomputeTransitive(t *testing.T) {
	cart := &cartState{Qty: 0, UnitPrice: 5}
	ci := computedCI(cart)
	if err := ci.RegisterComputed(cartComputeds(cart)); err != nil {
		t.Fatal(err)
	}

	// Seeded at Register from the initial state.
	if cart.Subtotal != 0 || cart.SubtotalText != "$0" || cart.CheckoutEnabled {
		t.Fatalf("initial seed wrong: %+v", cart)
	}

	cart.Qty = 3
	expanded := ci.ExpandAndRecompute([]string{"Qty"})

	want := map[string]bool{"Qty": true, "Subtotal": true, "SubtotalText": true, "CheckoutEnabled": true}
	for _, f := range expanded {
		delete(want, f)
	}
	if len(want) != 0 {
		t.Errorf("expanded set %v missing %v", expanded, want)
	}
	if cart.Subtotal != 15 || cart.SubtotalText != "$15" || !cart.CheckoutEnabled {
		t.Errorf("recompute (topo order) wrong: %+v", cart)
	}
}

// Only computeds reachable from the marked field are recomputed/expanded — an
// unrelated computed is left alone.
func TestComputed_ExpandIsSelective(t *testing.T) {
	cart := &cartState{Qty: 1, UnitPrice: 2}
	ci := computedCI(cart)
	if err := ci.RegisterComputed(cartComputeds(cart)); err != nil {
		t.Fatal(err)
	}
	cart.CheckoutEnabled = false // sentinel: must not be touched by a UnitPrice change

	cart.UnitPrice = 10
	expanded := ci.ExpandAndRecompute([]string{"UnitPrice"})

	has := map[string]bool{}
	for _, f := range expanded {
		has[f] = true
	}
	if !has["Subtotal"] || !has["SubtotalText"] {
		t.Errorf("UnitPrice change should reach Subtotal+SubtotalText; got %v", expanded)
	}
	if has["CheckoutEnabled"] {
		t.Errorf("UnitPrice change must not reach CheckoutEnabled; got %v", expanded)
	}
	if cart.Subtotal != 10 || cart.CheckoutEnabled {
		t.Errorf("selective recompute wrong: %+v", cart)
	}
}

func TestComputed_RecomputeAll(t *testing.T) {
	cart := &cartState{Qty: 4, UnitPrice: 3}
	ci := computedCI(cart)
	if err := ci.RegisterComputed(cartComputeds(cart)); err != nil {
		t.Fatal(err)
	}
	cart.Qty = 5
	ci.RecomputeAll()
	if cart.Subtotal != 15 || cart.SubtotalText != "$15" || !cart.CheckoutEnabled {
		t.Errorf("RecomputeAll wrong: %+v", cart)
	}
}

func TestComputed_ValidationErrors(t *testing.T) {
	noop := func() any { return 0 }

	cases := []struct {
		name string
		defs []ComputedDef
	}{
		{"unknown field name", []ComputedDef{{Name: "Ghost", Fn: noop}}},
		{"unknown dependency", []ComputedDef{{Name: "Subtotal", Fn: noop, Deps: []string{"Ghost"}}}},
		{"duplicate computed", []ComputedDef{{Name: "Subtotal", Fn: noop}, {Name: "Subtotal", Fn: noop}}},
		{"missing fn", []ComputedDef{{Name: "Subtotal"}}},
		{"dependency cycle", []ComputedDef{
			{Name: "Subtotal", Fn: noop, Deps: []string{"SubtotalText"}},
			{Name: "SubtotalText", Fn: func() any { return "" }, Deps: []string{"Subtotal"}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := computedCI(&cartState{}).RegisterComputed(c.defs); err == nil {
				t.Errorf("expected an error for %s", c.name)
			}
		})
	}
}

// A computed whose fn returns a value not assignable/convertible to the field
// type leaves the field unchanged (and logs) rather than panicking.
func TestComputed_TypeMismatchLeavesFieldUnchanged(t *testing.T) {
	cart := &cartState{Subtotal: 7}
	ci := computedCI(cart)
	if err := ci.RegisterComputed([]ComputedDef{
		{Name: "Subtotal", Fn: func() any { return "not-an-int" }, Deps: []string{"Qty"}},
	}); err != nil {
		t.Fatal(err)
	}
	// RegisterComputed seeded once (mismatch → unchanged from zero value).
	if cart.Subtotal != 0 {
		t.Logf("note: seed left Subtotal=%d", cart.Subtotal)
	}
	cart.Subtotal = 42
	ci.RecomputeAll() // mismatch again → must not clobber or panic
	if cart.Subtotal != 42 {
		t.Errorf("type-mismatch recompute changed the field to %d, want 42 (unchanged)", cart.Subtotal)
	}
}
