package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
)

type gatewayProbeAcquisition struct {
	candidate route.Candidate
	closed    bool
}

func (a *gatewayProbeAcquisition) Candidate() route.Candidate { return a.candidate }
func (a *gatewayProbeAcquisition) Close() error {
	a.closed = true
	return nil
}

func TestLegacyGatewayRDPRequiresARealProtocolResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		request := make([]byte, 19)
		if _, readErr := io.ReadFull(connection, request); readErr == nil {
			_, _ = connection.Write([]byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0})
		}
	}()
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	acquisition := &gatewayProbeAcquisition{candidate: route.Candidate{ID: "gateway-rdp", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: port}}
	provider := legacyGatewayDesktopProvider{
		Provider: routeStatusProvider{acquisition: acquisition},
		Protocol: func(string) string { return "rdp" },
		Timeout:  time.Second,
	}
	got, err := provider.Acquire(context.Background(), route.Request{CanonicalTarget: "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"})
	if err != nil || got != acquisition || acquisition.closed {
		t.Fatalf("Acquire() = %#v, %v; closed=%v", got, err, acquisition.closed)
	}
	_ = got.Close()
}

func TestLegacyGatewayRDPRejectsAnOrphanedVisitor(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
	}()
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	acquisition := &gatewayProbeAcquisition{candidate: route.Candidate{ID: "gateway-rdp", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: port}}
	provider := legacyGatewayDesktopProvider{
		Provider: routeStatusProvider{acquisition: acquisition},
		Protocol: func(string) string { return "rdp" },
		Timeout:  time.Second,
	}
	if got, err := provider.Acquire(context.Background(), route.Request{CanonicalTarget: "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"}); err == nil || got != nil || !acquisition.closed {
		t.Fatalf("Acquire() = %#v, %v; closed=%v", got, err, acquisition.closed)
	}
}
