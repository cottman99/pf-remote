package shellbinding

import (
	"os"
	"path/filepath"
	"testing"
)

const syntheticED25519PublicKey = "AAAAC3NzaC1lZDI1NTE5AAAAIMvF3FK8rr2A2r9iVfj0x8l26LThB5GXxJ7XyQPRZP5Z"

func TestDiscoverHostKeysReadsOnlyBoundedRegularPublicKeys(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "ssh_host_ed25519_key.pub")
	if err := os.WriteFile(first, []byte("ssh-ed25519 "+syntheticED25519PublicKey+" synthetic-comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := DiscoverHostKeys(first, filepath.Join(dir, "missing.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Algorithm != "ssh-ed25519" || keys[0].PublicKey != syntheticED25519PublicKey {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestDiscoverHostKeysRejectsInvalidOrLinkedSources(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid.pub")
	if err := os.WriteFile(invalid, []byte("not-a-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverHostKeys(invalid); err == nil {
		t.Fatal("invalid public key accepted")
	}
	link := filepath.Join(dir, "linked.pub")
	if err := os.Symlink(invalid, link); err == nil {
		if _, err := DiscoverHostKeys(link); err == nil {
			t.Fatal("linked public key accepted")
		}
	}
}
