//go:build windows

package localapi

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dial(ctx context.Context) (net.Conn, error) {
	endpoint, err := localEndpoint()
	if err != nil {
		return nil, err
	}
	return winio.DialPipeContext(ctx, endpoint)
}
