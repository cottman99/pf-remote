//go:build !windows

package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreate_PermissionWidening_FailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOrCreate(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOrCreate(); err == nil {
		t.Fatal("identity with group or other access was accepted")
	}
}
