package server

import (
	"strings"
	"testing"
)

// Regression for the bundle-order bug: client modules MUST be emitted after the
// bridge, so a module that calls the godom API at load (e.g. declareCapability)
// finds it defined instead of throwing and aborting the whole bundle.
func TestAssembleBundle_ModulesAfterBridge(t *testing.T) {
	out := assembleBundle(bundleInputs{
		protobufMinJS: "PROTOBUF;",
		protocolJS:    "PROTOCOL;",
		bridge:        "BRIDGE_MARKER;",
		modules:       map[string]string{"m": "MODULE_MARKER;"},
	})
	bridgeAt := strings.Index(out, "BRIDGE_MARKER")
	moduleAt := strings.Index(out, "MODULE_MARKER")
	if bridgeAt < 0 || moduleAt < 0 {
		t.Fatalf("markers missing: bridge=%d module=%d", bridgeAt, moduleAt)
	}
	if moduleAt < bridgeAt {
		t.Errorf("client modules must be emitted AFTER bridge (module@%d, bridge@%d)", moduleAt, bridgeAt)
	}
}

// protobuf/protocol must still come before the bridge (the bridge depends on
// them), and no module is emitted when none is registered.
func TestAssembleBundle_BaseOrderAndNoModules(t *testing.T) {
	out := assembleBundle(bundleInputs{
		protobufMinJS: "PROTOBUF;",
		protocolJS:    "PROTOCOL;",
		bridge:        "BRIDGE_MARKER;",
	})
	if i, j := strings.Index(out, "PROTOBUF"), strings.Index(out, "BRIDGE_MARKER"); i < 0 || i > j {
		t.Errorf("protobuf must precede bridge")
	}
	if strings.Contains(out, "g.modules=g.modules") {
		t.Errorf("no module namespace should be emitted when no modules are registered")
	}
}

// Each module is wrapped so one that throws at load can't take down the others
// (or the bridge).
func TestClientModulesJS_WrapsEachInTryCatch(t *testing.T) {
	js := clientModulesJS(map[string]string{"m": "MODULE_BODY;"})
	if !strings.Contains(js, "try{") || !strings.Contains(js, "}catch(") {
		t.Error("each module should be wrapped in try/catch")
	}
	if !strings.Contains(js, "MODULE_BODY;") {
		t.Error("module body must be present inside the wrapper")
	}
}
