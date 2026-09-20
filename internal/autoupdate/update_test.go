package autoupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"
)

func TestBootstrapNeverResetsMissingTrust(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	previous := PublisherKey
	t.Cleanup(func() { PublisherKey = previous })
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	PublisherKey = base64.StdEncoding.EncodeToString(pub)
	if err := Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(); err != nil {
		t.Fatal(err)
	}
	trust, cache, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteStatus(cache, Status{Phase: "failed", Version: "0.1.0", Sequence: 3}); err != nil {
		t.Fatal(err)
	}
	s, err := ReadStatus(cache)
	if err != nil || s.Phase != "failed" || s.Sequence != 3 {
		t.Fatalf("%+v %v", s, err)
	}
	if err := os.Remove(trust); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(); err == nil {
		t.Fatal("missing replay state silently reset")
	}
}
