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
	for _, want := range []string{"sendClientEnv", "__godom_env__"} {
		if !strings.Contains(bridge, want) {
			t.Errorf("embedded bridge.js missing %q — env delivery not wired", want)
		}
	}
}
