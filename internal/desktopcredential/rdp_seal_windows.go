//go:build windows

package desktopcredential

import (
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf16"
)

// SealRDPPassword returns MSTSC's current-user DPAPI password-51 representation.
// It is unique to this launch and never updates a shared TERMSRV credential.
func SealRDPPassword(password string) (string, error) {
	if password == "" || strings.ContainsRune(password, 0) {
		return "", errors.New("invalid RDP credential")
	}
	units := utf16.Encode([]rune(password))
	defer clear(units)
	if len(units) > 1200 {
		return "", errors.New("invalid RDP credential length")
	}
	plain := make([]byte, len(units)*2)
	defer clear(plain)
	for i, u := range units {
		plain[2*i], plain[2*i+1] = byte(u), byte(u>>8)
	}
	sealed, err := transformDPAPI(plain, true)
	defer clear(sealed)
	if err != nil {
		return "", errors.New("protect native RDP credential")
	}
	return hex.EncodeToString(sealed), nil
}
