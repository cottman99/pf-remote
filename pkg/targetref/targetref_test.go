package targetref

import "testing"

func TestRoundTrip(t *testing.T) {
	raw := "pfremote://fabric-demo/devices/device-demo/capabilities/shell-main"
	r, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.String(); got != raw {
		t.Fatalf("round trip = %q, want %q", got, raw)
	}
}

func TestRejectsLocationAndSecrets(t *testing.T) {
	cases := []string{
		"ssh://fabric-demo/devices/a/capabilities/b",
		"pfremote://fabric-demo/devices/a/capabilities/b?token=secret",
		"pfremote://fabric-demo/devices/a/capabilities/b/extra",
		"pfremote://fabric-demo/devices/a/capabilities/b#fragment",
	}
	for _, raw := range cases {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", raw)
		}
	}
}
