//go:build windows

package localapi

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func setTestEndpoint(t *testing.T) {
	t.Helper()
	t.Setenv("PFREMOTE_LOCAL_ENDPOINT", fmt.Sprintf(`\\.\pipe\pfremote-test-%d-%d`, os.Getpid(), time.Now().UnixNano()))
}

func dialTestEndpoint(endpoint string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(endpoint, &timeout)
}
