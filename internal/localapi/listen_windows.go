//go:build windows

package localapi

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"

	"github.com/Microsoft/go-winio"
)

const windowsPipe = `\\.\pipe\pfremote-v1`

func localEndpoint() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PFREMOTE_LOCAL_ENDPOINT")); override != "" {
		if !strings.HasPrefix(override, `\\.\pipe\`) {
			return "", fmt.Errorf("PFREMOTE_LOCAL_ENDPOINT must name a Windows Named Pipe")
		}
		return override, nil
	}
	return windowsPipe, nil
}

// Listen opens a Named Pipe limited to SYSTEM, administrators, and the user
// who starts this development daemon. The installer will supply a service ACL
// for the production per-machine daemon in a later milestone.
func Listen() (net.Listener, string, error) {
	endpoint, err := localEndpoint()
	if err != nil {
		return nil, "", err
	}
	current, err := user.Current()
	if err != nil {
		return nil, "", fmt.Errorf("resolve current user for pipe ACL: %w", err)
	}
	descriptor := "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;" + current.Uid + ")"
	listener, err := winio.ListenPipe(endpoint, &winio.PipeConfig{
		SecurityDescriptor: descriptor,
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	})
	if err != nil {
		return nil, "", err
	}
	return listener, endpoint, nil
}
