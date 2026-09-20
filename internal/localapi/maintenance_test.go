package localapi

import "testing"

func TestUpdateGateDrainsAndRecovers(t *testing.T) {
	g := &ActionGate{}
	leave, ok := g.Enter()
	if !ok {
		t.Fatal("initial operation blocked")
	}
	if g.BeginUpdate() {
		t.Fatal("update began with active operation")
	}
	leave()
	if !g.BeginUpdate() {
		t.Fatal("idle update refused")
	}
	if _, ok := g.Enter(); ok {
		t.Fatal("new operation admitted during update")
	}
	g.CancelUpdate()
	leave, ok = g.Enter()
	if !ok {
		t.Fatal("failed update left actions blocked")
	}
	leave()
}
