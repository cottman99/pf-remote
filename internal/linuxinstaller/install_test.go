package linuxinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCorruptRecoveryDoesNotRemoveInstalledBinaries(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".update"), 0700)
	os.WriteFile(filepath.Join(root, "pfremoted"), []byte("retained"), 0700)
	os.WriteFile(filepath.Join(root, ".update", "pending.json"), []byte(`{"existing":{}}`), 0600)
	if Recover(root) == nil {
		t.Fatal("accepted incomplete recovery")
	}
	b, _ := os.ReadFile(filepath.Join(root, "pfremoted"))
	if string(b) != "retained" {
		t.Fatal("corrupt recovery changed installed binary")
	}
}

func TestFailedActivationRestoresAllBinaries(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	for _, name := range names {
		os.WriteFile(filepath.Join(root, name), []byte("old"), 0700)
		os.WriteFile(filepath.Join(stage, name+"-linux-x64"), []byte("new"), 0700)
	}
	calls := 0
	err := Upgrade(root, stage, func() error {
		calls++
		if calls == 1 {
			return errors.New("unhealthy")
		}
		return nil
	})
	if err == nil || calls != 2 {
		t.Fatal("missing rollback")
	}
	for _, name := range names {
		b, _ := os.ReadFile(filepath.Join(root, name))
		if string(b) != "old" {
			t.Fatal("old binary not restored")
		}
	}
	if err := Recover(root); err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(root, stage, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		b, _ := os.ReadFile(filepath.Join(root, name))
		if string(b) != "new" {
			t.Fatal("new binary not installed")
		}
	}
}
