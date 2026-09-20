//go:build !windows

package localapi

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

func setTestEndpoint(t *testing.T) {
	t.Helper()
	t.Setenv("PFREMOTE_LOCAL_ENDPOINT", filepath.Join(t.TempDir(), "daemon.sock"))
}

func dialTestEndpoint(endpoint string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", endpoint, timeout)
}
