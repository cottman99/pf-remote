//go:build windows

package identity

import (
	"bytes"
	"testing"
)

func TestDPAPIProtector_RoundTrip_UsesCurrentUserProtection(t *testing.T) {
	protector := dpapiProtector{}
	plaintext := []byte("synthetic-device-seed-material")
	ciphertext, err := protector.Protect(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, plaintext) || bytes.Equal(ciphertext, plaintext) {
		t.Fatal("DPAPI output exposed plaintext")
	}
	decrypted, err := protector.Unprotect(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(decrypted)
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("DPAPI round trip changed plaintext")
	}
}
