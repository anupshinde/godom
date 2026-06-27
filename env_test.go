package godom

import (
	"strings"
	"testing"
)

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
