//go:build !windows

package localapi

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func localEndpoint() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PFREMOTE_LOCAL_ENDPOINT")); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("PFREMOTE_LOCAL_ENDPOINT must be an absolute Unix socket path")
		}
		return filepath.Clean(override), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(configDir, "pfremote")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(directory, "daemon-v1.sock"), nil
}

func Listen() (net.Listener, string, error) {
	endpoint, err := localEndpoint()
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(endpoint), 0o700); err != nil {
		return nil, "", err
	}
	if err := os.Remove(endpoint); err != nil && !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("remove stale local socket: %w", err)
	}
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, "", err
	}
	if err := os.Chmod(endpoint, 0o600); err != nil {
		_ = listener.Close()
		return nil, "", err
	}
	return listener, endpoint, nil
}
