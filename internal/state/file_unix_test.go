//go:build !windows

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_ProtectsDatabaseFileAndDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(dir, "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = dir:%o file:%o", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
	}
}
