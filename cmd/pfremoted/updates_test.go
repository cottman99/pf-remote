package main

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestBackgroundUpdatesRequirePinnedBootstrap(t *testing.T) {
	saved := publisherPublicKey
	t.Cleanup(func() { publisherPublicKey = saved })
	publisherPublicKey = ""
	monitor := backgroundUpdates(filepath.Join(t.TempDir(), "state.db"))
	if monitor.Status().Status != "skip" {
		t.Fatal("unprovisioned build enabled discovery")
	}
	publisherPublicKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	monitor = backgroundUpdates(filepath.Join(t.TempDir(), "state.db"))
	if monitor.Check(context.Background()) {
		t.Fatal("missing durable trust accepted")
	}
}
