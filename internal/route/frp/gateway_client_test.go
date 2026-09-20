package frp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/gateway"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/routelease"
)

type clientSigner struct {
	id      string
	private ed25519.PrivateKey
}

func (s clientSigner) DeviceID() string { return s.id }
func (s clientSigner) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(s.private, message), nil
}

type clientAuthorizer struct {
	public ed25519.PublicKey
	expiry time.Time
}

func (a clientAuthorizer) AuthorizeRoute(context.Context, string, string) (routelease.Authorization, error) {
	return routelease.Authorization{PublicKey: a.public, ValidUntil: a.expiry}, nil
}

type clientBroker struct {
	closed []string
}

func (b *clientBroker) Open(context.Context, string, string, time.Time) (routelease.Endpoint, error) {
	return routelease.Endpoint{ServerAddress: "relay.example.com", ServerPort: 7000, ServerUser: "fabric-test", ServerName: "proxy-example-01", SecretKey: "fx_" + "client_secret_0123456789"}, nil
}

func (b *clientBroker) Close(_ context.Context, leaseID string) error {
	b.closed = append(b.closed, leaseID)
	return nil
}

func TestGatewayClientUsesSignedLoopbackControlPlane(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x51}, ed25519.SeedSize))
	broker := &clientBroker{}
	manager := &routelease.Manager{Authorizer: clientAuthorizer{public: private.Public().(ed25519.PublicKey), expiry: now.Add(time.Hour)},
		Broker: broker, Now: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x31}, 16))}
	server := httptest.NewServer((gateway.Server{Version: "test", Commit: "test", RouteLeases: manager}).Handler())
	defer server.Close()
	ids := []string{"request-client-01", "release-client-01"}
	client := GatewayClient{BaseURL: server.URL, HTTPClient: server.Client(), Signer: clientSigner{id: "device-controller", private: private},
		ClientVersion: "1.0.0", NewRequestID: func() (string, error) { id := ids[0]; ids = ids[1:]; return id, nil }}
	request := route.Request{SubjectDeviceID: "device-controller", CanonicalTarget: "pfremote://fabric-test/devices/device-target/capabilities/shell-main", AuthorizationExpiry: now.Add(time.Hour)}
	lease, err := client.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if lease.CanonicalTarget != request.CanonicalTarget || lease.Adapter != "frp" {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	if err := client.Release(context.Background(), lease.ID); err != nil {
		t.Fatal(err)
	}
	if len(broker.closed) != 1 || broker.closed[0] != lease.ID {
		t.Fatalf("broker cleanup = %v", broker.closed)
	}
}

func TestGatewayClientRejectsNonTLSRemoteControlPlane(t *testing.T) {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x52}, ed25519.SeedSize))
	client := GatewayClient{BaseURL: "http://192.0.2.10:47832", Signer: clientSigner{id: "device-controller", private: private}, ClientVersion: "1.0.0", NewRequestID: func() (string, error) { return "request-client-02", nil }}
	request := route.Request{SubjectDeviceID: "device-controller", CanonicalTarget: "pfremote://fabric-test/devices/device-target/capabilities/shell-main", AuthorizationExpiry: time.Now().Add(time.Hour)}
	if _, err := client.Acquire(context.Background(), request); err == nil {
		t.Fatal("remote plaintext Gateway endpoint was accepted")
	}
}
