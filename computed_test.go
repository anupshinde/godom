package godom

import "testing"

type cartApp struct {
	Island
	Qty       int
	UnitPrice int
	Subtotal  int
}

// Compute() declarations are stashed on the Island embed before Register; Register
// overwrites that embed, so this also verifies the declarations are extracted
// (via the promoted method) before the overwrite and that RegisterComputed seeds
// the initial value through the public path.
func TestCompute_RegisterSeedsInitialValue(t *testing.T) {
	app := &cartApp{Qty: 2, UnitPrice: 5}
	app.TargetName = "cart"
	app.TemplateHTML = `<div><span g-text="Subtotal"></span></div>`
	app.Compute("Subtotal", func() any { return app.Qty * app.UnitPrice }, "Qty", "UnitPrice")

	eng := NewEngine()
	eng.Register(app)

	if app.Subtotal != 10 {
		t.Errorf("Compute+Register should seed Subtotal=10, got %d", app.Subtotal)
	}
}
