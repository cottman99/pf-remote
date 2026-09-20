package routelease

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

type fixedAuthorizer struct {
	authorization Authorization
	err           error
	subject       string
	target        string
}

func (a *fixedAuthorizer) AuthorizeRoute(_ context.Context, subject, target string) (Authorization, error) {
	a.subject, a.target = subject, target
	return a.authorization, a.err
}

type captureBroker struct {
	endpoint Endpoint
	err      error
	opens    []string
	closes   []string
}

func (b *captureBroker) Open(_ context.Context, target, leaseID string, _ time.Time) (Endpoint, error) {
	b.opens = append(b.opens, target+"\x00"+leaseID)
	return b.endpoint, b.err
}

func (b *captureBroker) Close(_ context.Context, leaseID string) error {
	b.closes = append(b.closes, leaseID)
	return nil
}

func signedRequest(private ed25519.PrivateKey, target, requestID string) Request {
	request := Request{SchemaVersion: SchemaVersion, SubjectDeviceID: "device-controller", CanonicalTarget: target, RequestID: requestID, ClientVersion: "1.0.0"}
	request.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, RequestMessage(request)))
	return request
}

func signedRelease(private ed25519.PrivateKey, leaseID, requestID string) ReleaseRequest {
	request := ReleaseRequest{SchemaVersion: SchemaVersion, SubjectDeviceID: "device-controller", LeaseID: leaseID, RequestID: requestID, ClientVersion: "1.0.0"}
	request.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, ReleaseMessage(request)))
	return request
}

func TestManagerIssuesBoundLeaseAndAuthenticatesRelease(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	target := "pfremote://fabric-test/devices/device-target/capabilities/shell-main"
	authorizer := &fixedAuthorizer{authorization: Authorization{PublicKey: public, ValidUntil: now.Add(time.Hour)}}
	broker := &captureBroker{endpoint: Endpoint{ServerAddress: "relay.example.com", ServerPort: 7000, ServerUser: "fabric-test", ServerName: "proxy-example-01", SecretKey: "fx_" + "route_secret_0123456789"}}
	manager := &Manager{Authorizer: authorizer, Broker: broker, Now: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x12}, 16))}
	lease, err := manager.Issue(context.Background(), signedRequest(private, target, "request-example-01"))
	if err != nil {
		t.Fatal(err)
	}
	if lease.CanonicalTarget != target || lease.Adapter != "frp" || !lease.ExpiresAt.Equal(now.Add(DefaultTTL)) || authorizer.target != target {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	response, err := manager.Release(context.Background(), signedRelease(private, lease.ID, "release-example-01"))
	if err != nil || response.Status != "released" || len(broker.closes) != 1 || broker.closes[0] != lease.ID {
		t.Fatalf("response=%#v err=%v closes=%v", response, err, broker.closes)
	}
	if _, err := manager.Release(context.Background(), signedRelease(private, lease.ID, "release-example-02")); err == nil {
		t.Fatal("released lease remained active")
	}
}

func TestManagerFailsClosedBeforeBroker(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x42}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	target := "pfremote://fabric-test/devices/device-target/capabilities/shell-main"
	for name, mutate := range map[string]func(*Request, *fixedAuthorizer){
		"bad signature": func(request *Request, _ *fixedAuthorizer) { request.Signature = "bad" },
		"unauthorized":  func(_ *Request, authorizer *fixedAuthorizer) { authorizer.err = errors.New("revoked") },
		"bad target":    func(request *Request, _ *fixedAuthorizer) { request.CanonicalTarget = "not-a-target" },
	} {
		t.Run(name, func(t *testing.T) {
			authorizer := &fixedAuthorizer{authorization: Authorization{PublicKey: public, ValidUntil: now.Add(time.Hour)}}
			broker := &captureBroker{}
			manager := &Manager{Authorizer: authorizer, Broker: broker, Now: func() time.Time { return now }}
			request := signedRequest(private, target, "request-example-02")
			mutate(&request, authorizer)
			if _, err := manager.Issue(context.Background(), request); err == nil {
				t.Fatal("invalid lease request was accepted")
			}
			if len(broker.opens) != 0 {
				t.Fatalf("broker opened before authorization: %v", broker.opens)
			}
		})
	}
}

func TestManagerRejectsReplayAndCleansExpiredBrokerLease(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x43}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	target := "pfremote://fabric-test/devices/device-target/capabilities/shell-main"
	authorizer := &fixedAuthorizer{authorization: Authorization{PublicKey: public, ValidUntil: now.Add(time.Hour)}}
	broker := &captureBroker{endpoint: Endpoint{ServerAddress: "relay.example.com", ServerPort: 7000, ServerName: "proxy-example-01", SecretKey: "fx_" + "route_secret_0123456789"}}
	manager := &Manager{Authorizer: authorizer, Broker: broker, Now: func() time.Time { return now }, TTL: time.Second,
		Random: bytes.NewReader(append(append(bytes.Repeat([]byte{0x21}, 16), bytes.Repeat([]byte{0x22}, 16)...), bytes.Repeat([]byte{0x23}, 16)...))}
	request := signedRequest(private, target, "request-example-03")
	first, err := manager.Issue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Issue(context.Background(), request); err == nil {
		t.Fatal("replayed request was accepted")
	}
	now = now.Add(2 * time.Second)
	if _, err := manager.Issue(context.Background(), signedRequest(private, target, "request-example-04")); err != nil {
		t.Fatal(err)
	}
	if len(broker.closes) != 1 || broker.closes[0] != first.ID {
		t.Fatalf("expired lease cleanup = %v", broker.closes)
	}
}

func TestManagerCleansBrokerAfterPartialOpenFailure(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x44}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	target := "pfremote://fabric-test/devices/device-target/capabilities/shell-main"
	broker := &captureBroker{err: errors.New("private broker detail")}
	manager := &Manager{
		Authorizer: &fixedAuthorizer{authorization: Authorization{PublicKey: public, ValidUntil: now.Add(time.Hour)}},
		Broker:     broker, Now: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x24}, 16)),
	}
	_, err := manager.Issue(context.Background(), signedRequest(private, target, "request-example-05"))
	var structured *Fault
	if !errors.As(err, &structured) || structured.Code != "ROUTE_UNAVAILABLE" || structured.Stage != "route" || len(broker.closes) != 1 {
		t.Fatalf("err=%#v closes=%v", err, broker.closes)
	}
	if err != nil && (bytes.Contains([]byte(err.Error()), []byte("private")) || bytes.Contains([]byte(err.Error()), []byte("relay.example.com"))) {
		t.Fatalf("unsafe route failure: %v", err)
	}
}
