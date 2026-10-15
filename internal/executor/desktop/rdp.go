package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RDPOptions projects the selected local preset into a native client launch.
// Passwords go only to the Windows vault, never to the RDP file or JSON.
type RDPOptions struct {
	Endpoint        string
	SessionSelector string
	Username        string
	Password        string `json:"-"`
	CredentialOwner string
	Windowed        bool
}

func (r RDPOptions) content() (string, error) {
	if r.Endpoint == "" || strings.ContainsAny(r.Endpoint, "\r\n\x00") {
		return "", errors.New("invalid RDP endpoint")
	}
	if !validSessionSelector(r.SessionSelector) {
		return "", errors.New("invalid RDP session selector")
	}
	if len(r.Username) > 256 || strings.ContainsAny(r.Username, "\r\n\x00") {
		return "", errors.New("invalid RDP account")
	}
	content := fmt.Sprintf("full address:s:%s\r\naudiomode:i:0\r\naudiocapturemode:i:1\r\nredirectclipboard:i:1\r\nscreen mode id:i:2\r\nredirectprinters:i:1\r\nredirectcomports:i:1\r\nredirectsmartcards:i:1\r\nredirectwebauthn:i:1\r\nredirectposdevices:i:1\r\nredirectlocation:i:1\r\ndrivestoredirect:s:*\r\ndevicestoredirect:s:*\r\nusbdevicestoredirect:s:*\r\ncamerastoredirect:s:*\r\nauthentication level:i:2\r\nprompt for credentials:i:0\r\n", r.Endpoint)
	if r.SessionSelector != "" {
		content += "domain:s:" + r.SessionSelector + "\r\n"
	}
	if r.Username != "" {
		content += "username:s:" + r.Username + "\r\n"
	}
	if r.Windowed {
		content = strings.Replace(content, "screen mode id:i:2", "screen mode id:i:1", 1)
	}
	return content, nil
}

func validSessionSelector(value string) bool {
	if len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func writeRDPProfile(options RDPOptions) (string, func(), error) {
	content, err := options.content()
	if err != nil {
		return "", func() {}, err
	}
	// Windows temp inherits the signed-in user's protected directory ACL.
	directory, err := os.MkdirTemp("", "pfremote-rdp-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "desktop.rdp")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}
