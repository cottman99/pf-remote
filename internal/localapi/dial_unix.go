//go:build !windows

package localapi

import (
	"context"
	"net"
)

func dial(ctx context.Context) (net.Conn, error) {
	endpoint, err := localEndpoint()
	if err != nil {
		return nil, err
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", endpoint)
}
