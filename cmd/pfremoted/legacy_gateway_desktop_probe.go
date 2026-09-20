package main

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
)

// legacyGatewayDesktopProvider rejects a legacy Gateway RDP visitor that is
// listening locally but cannot reach the actual target Desktop. This keeps the
// manual route menu and Smart connect from treating an orphaned FRP visitor as
// a usable route.
type legacyGatewayDesktopProvider struct {
	Provider route.Provider
	Protocol func(string) string
	Timeout  time.Duration
}

func (p legacyGatewayDesktopProvider) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	if p.Provider == nil {
		return nil, errors.New("legacy Gateway route is unavailable")
	}
	acquired, err := p.Provider.Acquire(ctx, request)
	if err != nil || acquired == nil {
		return nil, errors.New("legacy Gateway route is unavailable")
	}
	protocol := ""
	if p.Protocol != nil {
		protocol = strings.ToLower(strings.TrimSpace(p.Protocol(request.CanonicalTarget)))
	}
	if protocol == "rdp" {
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 1500 * time.Millisecond
		}
		if err := verifyRDPHandshake(ctx, acquired.Candidate(), timeout); err != nil {
			_ = acquired.Close()
			return nil, errors.New("legacy Gateway RDP endpoint is unavailable")
		}
	}
	return acquired, nil
}

func verifyRDPHandshake(ctx context.Context, candidate route.Candidate, timeout time.Duration) error {
	if candidate.Validate() != nil {
		return errors.New("RDP route candidate is invalid")
	}
	connection, err := (&net.Dialer{Timeout: timeout}).DialContext(
		ctx,
		candidate.Network,
		net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port))),
	)
	if err != nil {
		return errors.New("RDP route endpoint is unreachable")
	}
	defer connection.Close()
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return errors.New("RDP route deadline is unavailable")
	}
	request := []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x08, 0x00, 0x03, 0x00, 0x00, 0x00}
	if _, err := connection.Write(request); err != nil {
		return errors.New("RDP route request failed")
	}
	response := make([]byte, 6)
	if _, err := io.ReadFull(connection, response); err != nil {
		return errors.New("RDP route did not answer")
	}
	if response[0] != 0x03 || response[1] != 0x00 || response[5] != 0xd0 {
		return errors.New("RDP route returned an unexpected protocol")
	}
	return nil
}

var _ route.Provider = legacyGatewayDesktopProvider{}
