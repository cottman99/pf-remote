package linuxinstaller

import (
	"github.com/cottman99/pf-remote/internal/peerguard"
	"os"
	"path/filepath"
	"testing"
)

func TestPeerFloorRejectsLegacyRecoveryWithoutMutation(t *testing.T) {
	d := t.TempDir()
	t.Setenv("APPDATA", d)
	t.Setenv("XDG_CONFIG_HOME", d)
	p, _ := peerguard.DefaultDirectory()
	if e := peerguard.Require(p); e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(d, "bin")
	os.MkdirAll(filepath.Join(root, ".update"), 0700)
	os.WriteFile(filepath.Join(root, "pfremoted"), []byte("new"), 0700)
	os.WriteFile(filepath.Join(root, ".update", "pending.json"), []byte(`{"existing":{"pfremote":true,"pfremoted":true,"pfremote-update":true}}`), 0600)
	if Recover(root) == nil {
		t.Fatal("restored old authority")
	}
	b, _ := os.ReadFile(filepath.Join(root, "pfremoted"))
	if string(b) != "new" {
		t.Fatal("changed before guard")
	}
}
