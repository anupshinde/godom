package server

import "testing"

func TestClient_HasAndAddCapability(t *testing.T) {
	c := &Client{}
	if c.Has("widget") {
		t.Error("a fresh client should have no capabilities")
	}
	c.addCapability("widget")
	if !c.Has("widget") {
		t.Error("Has should be true after addCapability")
	}
	if c.Has("other") {
		t.Error("Has should be false for an undeclared capability")
	}
}

func TestApplyClientCapability(t *testing.T) {
	c := &Client{}
	applyClientCapability(c, [][]byte{[]byte(`"widget"`)})
	if !c.Has("widget") {
		t.Error("applyClientCapability should record the advertised capability")
	}

	// Safe no-ops: malformed JSON, empty name, no args, nil client.
	applyClientCapability(c, [][]byte{[]byte(`not json`)})
	applyClientCapability(c, [][]byte{[]byte(`""`)})
	applyClientCapability(c, nil)
	applyClientCapability(nil, [][]byte{[]byte(`"x"`)})
	if c.Has("") {
		t.Error("an empty capability name must not be recorded")
	}
}

// ClientsWith returns exactly the clients that advertised the capability — this
// is what lets a fan-out target only capable tabs (and find a singleton owner).
func TestClientsWith(t *testing.T) {
	a := &Client{id: "a"}
	b := &Client{id: "b"}
	cc := &Client{id: "c"}
	a.addCapability("chart")
	cc.addCapability("chart")
	b.addCapability("other")

	got := ClientsWith([]*Client{a, b, cc, nil}, "chart")
	if len(got) != 2 {
		t.Fatalf("expected 2 clients with 'chart', got %d", len(got))
	}
	have := map[string]bool{got[0].id: true, got[1].id: true}
	if !have["a"] || !have["c"] {
		t.Errorf("ClientsWith('chart') should be {a, c}, got %v", have)
	}
	if len(ClientsWith(nil, "chart")) != 0 {
		t.Error("ClientsWith over an empty set should be empty")
	}
	if len(ClientsWith([]*Client{b}, "chart")) != 0 {
		t.Error("a client without the capability must be excluded")
	}
}
