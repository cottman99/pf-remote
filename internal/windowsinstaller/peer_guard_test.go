package windowsinstaller

import (
	"github.com/cottman99/pf-remote/internal/peerguard"
	"os"
	"path/filepath"
	"testing"
)

func TestPeerFloorRejectsLegacyRollback(t *testing.T) {
	d := t.TempDir()
	t.Setenv("APPDATA", d)
	t.Setenv("XDG_CONFIG_HOME", d)
	p, _ := peerguard.DefaultDirectory()
	if e := peerguard.Require(p); e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(d, "installation")
	os.MkdirAll(filepath.Join(root, "versions", "0.1.0-alpha.118"), 0700)
	os.WriteFile(filepath.Join(root, "versions", "0.1.0-alpha.118", "PFRemoteCenter.exe"), []byte("old"), 0700)
	before := State{SchemaVersion: StateSchema, Current: "0.1.0-alpha.119", Previous: "0.1.0-alpha.118"}
	writeJSONAtomic(filepath.Join(root, "current.json"), before)
	if _, e := Rollback(root); e == nil {
		t.Fatal("legacy rollback allowed")
	}
	after, e := loadState(root)
	if e != nil || after != before {
		t.Fatal("state changed")
	}
	writeJSONAtomic(filepath.Join(root, pendingName), pendingUpgrade{"pfremote.pending-upgrade/v1", State{SchemaVersion: StateSchema, Current: "0.1.0-alpha.118"}})
	called := false
	if _, e = RecoverUpgrade(root, func(Result) error { called = true; return nil }); e == nil || called {
		t.Fatal("legacy recovery activation allowed")
	}
}
