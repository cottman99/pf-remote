package contracts

import "testing"

func TestSessionSelectorIsBoundedAndRDPOnly(t *testing.T) {
	for _, p := range []DesktopProfile{{Protocol: "rdp", SessionSelector: "workspace-2"}, {Protocol: "vnc"}} {
		if !p.ValidSessionSelector() {
			t.Fatal("valid selector rejected")
		}
	}
	for _, p := range []DesktopProfile{{Protocol: "vnc", SessionSelector: "workspace-2"}, {Protocol: "rdp", SessionSelector: "bad\r\nsetting:i:1"}, {Protocol: "rdp", SessionSelector: "../workspace"}} {
		if p.ValidSessionSelector() {
			t.Fatal("unsafe selector accepted")
		}
	}
}
