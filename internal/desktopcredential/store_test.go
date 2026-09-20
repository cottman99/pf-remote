package desktopcredential

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testProtector struct{}

func (testProtector) Name() string { return "test-protector" }
func (testProtector) Protect(value []byte) ([]byte, error) {
	result := append([]byte("protected:"), value...)
	return result, nil
}
func (testProtector) Unprotect(value []byte) ([]byte, error) {
	if !bytes.HasPrefix(value, []byte("protected:")) {
		return nil, errors.New("invalid")
	}
	return append([]byte(nil), value[len("protected:"):]...), nil
}

func TestStoreBindsProtectedCredentialToCanonicalTarget(t *testing.T) {
	store := newStore(t.TempDir(), testProtector{})
	target := "pfremote://fabric-example/devices/device-a/capabilities/desktop-a"
	if err := store.Save(target, "synthetic-password"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(loaded)
	if string(loaded) != "synthetic-password" {
		t.Fatalf("loaded credential mismatch")
	}
	files, err := filepath.Glob(filepath.Join(store.root, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("credential files = %v, %v", files, err)
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(content, []byte("synthetic-password")) || bytes.Contains(content, []byte(target)) {
		t.Fatal("stored credential leaked plaintext or canonical target")
	}
	if _, err := store.Load("pfremote://fabric-example/devices/device-b/capabilities/desktop-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong-target load = %v", err)
	}
}

func TestStoreRejectsEmptyCredential(t *testing.T) {
	store := newStore(t.TempDir(), testProtector{})
	if err := store.Save("target", ""); err == nil {
		t.Fatal("expected empty credential rejection")
	}
}
