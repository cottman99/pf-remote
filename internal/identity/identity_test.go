package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type xorProtector struct{}

func (xorProtector) Name() string { return "test-xor" }

func (xorProtector) Protect(value []byte) ([]byte, error) { return xor(value), nil }

func (xorProtector) Unprotect(value []byte) ([]byte, error) { return xor(value), nil }

func xor(value []byte) []byte {
	result := append([]byte(nil), value...)
	for index := range result {
		result[index] ^= 0x5a
	}
	return result
}

func TestLoadOrCreate_NewStore_ReturnsStableSigningIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	store, err := newStore(path, xorProtector{}, bytes.NewReader(bytes.Repeat([]byte{0x42}, ed25519.SeedSize)))
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if created.DeviceID() != loaded.DeviceID() || !bytes.Equal(created.PublicKey(), loaded.PublicKey()) {
		t.Fatal("reloaded identity changed")
	}
	message := []byte("activation proof")
	signature, err := loaded.Sign(message)
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(loaded.PublicKey(), message, signature) {
		t.Fatal("signature did not verify")
	}
}

func TestLoadOrCreate_CorruptStore_FailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newStore(path, xorProtector{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOrCreate(); err == nil {
		t.Fatal("corrupt identity store was silently replaced")
	}
}

func TestLoadOrCreate_MismatchedPublicKey_FailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	store, err := newStore(path, xorProtector{}, bytes.NewReader(bytes.Repeat([]byte{0x31}, ed25519.SeedSize)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOrCreate(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored document
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatal(err)
	}
	stored.DeviceID = "device-invalid"
	content, err = json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOrCreate(); err == nil {
		t.Fatal("mismatched identity store was accepted")
	}
}

func TestLoadOrCreate_ConcurrentCreators_ConvergeOnOneIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	const workers = 8
	results := make(chan string, workers)
	errorsFound := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			store, err := newStore(path, xorProtector{}, nil)
			if err != nil {
				errorsFound <- err
				return
			}
			loaded, err := store.LoadOrCreate()
			if err != nil {
				errorsFound <- err
				return
			}
			results <- loaded.DeviceID()
		}()
	}
	group.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	var expected string
	for result := range results {
		if expected == "" {
			expected = result
		}
		if result != expected {
			t.Fatalf("concurrent creators returned different identities: %q and %q", expected, result)
		}
	}
}

func TestPublicKey_ReturnedSliceCannotMutateIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	store, err := newStore(path, xorProtector{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	first := loaded.PublicKey()
	first[0] ^= 0xff
	if bytes.Equal(first, loaded.PublicKey()) {
		t.Fatal("public key accessor exposed mutable identity storage")
	}
}

func TestIdentity_EmptyIdentityCannotSign(t *testing.T) {
	if _, err := (Identity{}).Sign([]byte("message")); err == nil {
		t.Fatal("empty identity signed a message")
	}
}

func TestVerify_InvalidInputs_ReturnsFalse(t *testing.T) {
	if Verify(nil, []byte("message"), nil) {
		t.Fatal("invalid signature inputs verified")
	}
}

func TestDeviceIDFromPublicKey_InvalidKey_ReturnsError(t *testing.T) {
	if _, err := DeviceIDFromPublicKey(nil); err == nil {
		t.Fatal("invalid public key produced a Device ID")
	}
}

func TestDefaultPath_ReturnsVersionedIdentityFilename(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "device-identity-v1.json" {
		t.Fatalf("default path = %q", path)
	}
}
