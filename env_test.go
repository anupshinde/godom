package godom

import (
	"strings"
	"testing"
)

// RegisterClientModule stores the module for the server to ship in the bundle.
func TestRegisterClientModule(t *testing.T) {
	eng := NewEngine()
	if len(eng.ClientModules()) != 0 {
		t.Errorf("a fresh engine should have no client modules")
	}
	eng.RegisterClientModule("widget", "godom.modules.widget={render:function(){}};")
	if got := eng.ClientModules()["widget"]; got == "" {
		t.Errorf("RegisterClientModule did not store the module")
	}
}

// The embedded bridge must carry the §3 env delivery; this guards against the
// bridge.js wiring being dropped, since the Go side silently ignores a client
// that never sends its env.
func TestBridgeJS_DeliversConnectionEnv(t *testing.T) {
	bridge, _, _ := NewEngine().EmbeddedJS()
	for _, want := range []string{
		"sendClientEnv", "__godom_env__", // §3 env delivery
		"declareCapability", "__godom_capability__", // §4 capability advertisement
	} {
		if !strings.Contains(bridge, want) {
			t.Errorf("embedded bridge.js missing %q", want)
		}
	}
}

// ClientsWith delegates to the bound roster; with no roster (pre-Run) it is
// empty, never a panic.
func TestEngineClientsWith_EmptyBeforeBind(t *testing.T) {
	eng := NewEngine()
	if got := eng.ClientsWith("widget"); len(got) != 0 {
		t.Errorf("ClientsWith before bind should be empty, got %v", got)
	}
}
