//go:build windows

package desktopcredential

import (
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestNativeRDPEnvelopeIsCurrentUserUnicodeAndDetectsTampering(t *testing.T) {
	secret := strings.Join([]string{"synthetic", "密码", "🔐"}, "-")
	envelope, err := SealRDPPassword(secret)
	if err != nil || strings.Contains(envelope, secret) {
		t.Fatal("native RDP password protection failed")
	}
	sealed, err := hex.DecodeString(envelope)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := transformDPAPI(sealed, false)
	defer clear(plain)
	if err != nil || len(plain)%2 != 0 {
		t.Fatal("native envelope is not readable by the current user")
	}
	units := make([]uint16, len(plain)/2)
	defer clear(units)
	for i := range units {
		units[i] = uint16(plain[2*i]) | uint16(plain[2*i+1])<<8
	}
	if string(utf16.Decode(units)) != secret {
		t.Fatal("native RDP credential Unicode encoding changed")
	}
	sealed[len(sealed)/2] ^= 1
	if _, err = transformDPAPI(sealed, false); err == nil {
		t.Fatal("modified credential envelope was accepted")
	}
}
