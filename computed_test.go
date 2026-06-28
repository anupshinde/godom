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

// The public task wrappers: WithRestart/WithQueue return options, and Task is a
// safe no-op before the island is serving (nil ci / no event loop). The task
// state machine itself is covered in internal/island and internal/server.
func TestTaskPublicAPI_OptionsAndNoOpGuard(t *testing.T) {
	if WithRestart() == nil || WithQueue() == nil {
		t.Error("WithRestart/WithQueue must return non-nil options")
	}
	// Task on an unregistered island (ci == nil) must not run the body or panic.
	app := &cartApp{}
	app.Task("never", func(tk *Task) { t.Error("body must not run without a started loop") })
}
