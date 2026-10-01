package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RDPOptions contains no credentials. Windows owns certificate verification,
// credential storage and audio device access.
type RDPOptions struct {
	Endpoint        string
	SessionSelector string
	AudioCapture    bool
}

func (r RDPOptions) content() (string, error) {
	if r.Endpoint == "" || strings.ContainsAny(r.Endpoint, "\r\n\x00") {
		return "", errors.New("invalid RDP endpoint")
	}
	if !validSessionSelector(r.SessionSelector) {
		return "", errors.New("invalid RDP session selector")
	}
	capture := 0
	if r.AudioCapture {
		capture = 1
	}
	content := fmt.Sprintf("full address:s:%s\r\naudiomode:i:0\r\naudiocapturemode:i:%d\r\nauthentication level:i:2\r\nprompt for credentials:i:0\r\n", r.Endpoint, capture)
	if r.SessionSelector != "" {
		content += "domain:s:" + r.SessionSelector + "\r\n"
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
