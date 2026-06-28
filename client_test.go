package godom

import "testing"

// fakeClientSource is a stand-in roster. Client == server.Client (type alias),
// so []*Client satisfies server.ClientSource without importing internal/server.
type fakeClientSource struct{ n int }

func (f fakeClientSource) Clients() []*Client { return make([]*Client, f.n) }

// Before the server binds a roster, Clients() must report nil (not an empty
// slice and not a panic) — there is genuinely no connection source yet.
func TestEngineClients_NilBeforeBind(t *testing.T) {
	eng := NewEngine()
	if got := eng.Clients(); got != nil {
		t.Errorf("Clients() before bind: want nil, got %v (len %d)", got, len(got))
	}
}

// After BindClients, Clients() must delegate to the bound source — proving the
// EngineConfig wiring that the server uses at startup actually reaches the
// public accessor.
func TestEngineClients_DelegatesToBoundSource(t *testing.T) {
	eng := NewEngine()
	eng.BindClients(fakeClientSource{n: 3})
	if got := eng.Clients(); len(got) != 3 {
		t.Errorf("Clients() after bind: want len 3 from bound source, got %d", len(got))
	}

	// Rebinding replaces the source.
	eng.BindClients(fakeClientSource{n: 0})
	if got := eng.Clients(); got == nil || len(got) != 0 {
		t.Errorf("Clients() after rebind to empty source: want non-nil len 0, got %v", got)
	}
}
