package routelease

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/pkg/targetref"
)

const (
	DefaultTTL     = 2 * time.Minute
	maxReplayIDs   = 4096
	maxActiveLease = 256
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)

type Authorization struct {
	PublicKey  ed25519.PublicKey
	ValidUntil time.Time
}

type Authorizer interface {
	AuthorizeRoute(context.Context, string, string) (Authorization, error)
}

type Endpoint struct {
	ServerAddress string
	ServerPort    uint16
	ServerUser    string
	ServerName    string
	SecretKey     string
}

// Broker makes the target-side FRP proxy available for the lease and must stop
// it no later than the supplied expiry even if the controller disappears.
// Production distribution remains behind this boundary; tests use a local
// synthetic relay.
type Broker interface {
	Open(context.Context, string, string, time.Time) (Endpoint, error)
	Close(context.Context, string) error
}

type Manager struct {
	mu         sync.Mutex
	Authorizer Authorizer
	Broker     Broker
	Now        func() time.Time
	Random     io.Reader
	TTL        time.Duration
	replays    map[string]struct{}
	replayList []string
	active     map[string]activeLease
}

type activeLease struct {
	subject   string
	target    string
	expires   time.Time
	publicKey ed25519.PublicKey
}

func (m *Manager) Issue(ctx context.Context, request Request) (Lease, error) {
	if m.Authorizer == nil || m.Broker == nil {
		return Lease{}, fault("ROUTE_NOT_CONFIGURED", "route", "Gateway routing is not configured.", "Configure the Gateway FRP route broker.")
	}
	if request.SchemaVersion != SchemaVersion || !requestIDPattern.MatchString(request.RequestID) || request.SubjectDeviceID == "" {
		return Lease{}, fault("INVALID_ROUTE_REQUEST", "route", "The route lease request is invalid.", "Send a supported signed route request.")
	}
	if _, err := targetref.Parse(request.CanonicalTarget); err != nil {
		return Lease{}, fault("INVALID_ROUTE_REQUEST", "route", "The route lease target is invalid.", "Use an immutable canonical target.")
	}
	authorization, err := m.Authorizer.AuthorizeRoute(ctx, request.SubjectDeviceID, request.CanonicalTarget)
	if err != nil || len(authorization.PublicKey) != ed25519.PublicKeySize {
		return Lease{}, fault("ROUTE_NOT_AUTHORIZED", "authorize", "The route lease is not authorized.", "Refresh the Grant and Device state before retrying.")
	}
	signature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil || !ed25519.Verify(authorization.PublicKey, RequestMessage(request), signature) {
		return Lease{}, fault("INVALID_SIGNATURE", "route", "The route lease signature is invalid.", "Sign the exact route request with the active subject Device identity.")
	}
	now := time.Now().UTC()
	if m.Now != nil {
		now = m.Now().UTC()
	}
	if !now.Before(authorization.ValidUntil) {
		return Lease{}, fault("ROUTE_NOT_AUTHORIZED", "authorize", "The route authorization has expired.", "Refresh authorization before requesting a route.")
	}
	leaseID, err := randomID(m.Random)
	if err != nil {
		return Lease{}, fault("ENTROPY_UNAVAILABLE", "route", "A route lease ID could not be generated.", "Retry after restoring operating-system entropy.")
	}
	ttl := m.TTL
	if ttl <= 0 || ttl > DefaultTTL {
		ttl = DefaultTTL
	}
	expires := now.Add(ttl)
	if authorization.ValidUntil.Before(expires) {
		expires = authorization.ValidUntil.UTC()
	}
	m.mu.Lock()
	if m.replays == nil {
		m.replays = make(map[string]struct{})
		m.active = make(map[string]activeLease)
	}
	if _, exists := m.replays[request.SubjectDeviceID+"\x00"+request.RequestID]; exists {
		m.mu.Unlock()
		return Lease{}, fault("REQUEST_REPLAYED", "route", "The route request was already used.", "Retry with a fresh request ID.")
	}
	expired := m.pruneLocked(now)
	if len(m.active) >= maxActiveLease {
		m.mu.Unlock()
		return Lease{}, fault("ROUTE_CAPACITY_REACHED", "route", "The Gateway route capacity is reached.", "Wait for another lease to close or expire.")
	}
	m.recordReplayLocked(request.SubjectDeviceID + "\x00" + request.RequestID)
	m.active[leaseID] = activeLease{
		subject: request.SubjectDeviceID, target: request.CanonicalTarget, expires: expires,
		publicKey: append(ed25519.PublicKey(nil), authorization.PublicKey...),
	}
	m.mu.Unlock()
	for _, expiredID := range expired {
		_ = m.Broker.Close(ctx, expiredID)
	}
	endpoint, err := m.Broker.Open(ctx, request.CanonicalTarget, leaseID, expires)
	if err != nil {
		m.mu.Lock()
		delete(m.active, leaseID)
		m.mu.Unlock()
		_ = m.Broker.Close(ctx, leaseID)
		return Lease{}, fault("ROUTE_UNAVAILABLE", "route", "The Gateway FRP route is unavailable.", "Inspect redacted route diagnostics and retry.")
	}
	return Lease{SchemaVersion: SchemaVersion, ID: leaseID, CanonicalTarget: request.CanonicalTarget, Adapter: "frp",
		ServerAddress: endpoint.ServerAddress, ServerPort: endpoint.ServerPort, ServerUser: endpoint.ServerUser,
		ServerName: endpoint.ServerName, SecretKey: endpoint.SecretKey, ExpiresAt: expires}, nil
}

func (m *Manager) Release(ctx context.Context, request ReleaseRequest) (ReleaseResponse, error) {
	if request.SchemaVersion != SchemaVersion || !requestIDPattern.MatchString(request.RequestID) || !requestIDPattern.MatchString(request.LeaseID) {
		return ReleaseResponse{}, fault("INVALID_ROUTE_REQUEST", "route", "The route release request is invalid.", "Send a supported signed release request.")
	}
	m.mu.Lock()
	lease, exists := m.active[request.LeaseID]
	m.mu.Unlock()
	if !exists || lease.subject != request.SubjectDeviceID {
		return ReleaseResponse{}, fault("ROUTE_LEASE_NOT_FOUND", "route", "The route lease is not active.", "No cleanup is required for an inactive lease.")
	}
	signature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil || !ed25519.Verify(lease.publicKey, ReleaseMessage(request), signature) {
		return ReleaseResponse{}, fault("INVALID_SIGNATURE", "route", "The route release signature is invalid.", "Sign the exact release request with the subject Device identity.")
	}
	m.mu.Lock()
	current, stillActive := m.active[request.LeaseID]
	if !stillActive || current.subject != request.SubjectDeviceID {
		m.mu.Unlock()
		return ReleaseResponse{}, fault("ROUTE_LEASE_NOT_FOUND", "route", "The route lease is not active.", "No cleanup is required for an inactive lease.")
	}
	delete(m.active, request.LeaseID)
	m.mu.Unlock()
	if err := m.Broker.Close(ctx, request.LeaseID); err != nil {
		return ReleaseResponse{}, fault("ROUTE_CLEANUP_FAILED", "route", "The Gateway could not finish route cleanup.", "Retry cleanup using the opaque lease ID.")
	}
	return ReleaseResponse{SchemaVersion: SchemaVersion, LeaseID: request.LeaseID, Status: "released"}, nil
}

func (m *Manager) pruneLocked(now time.Time) []string {
	var expired []string
	for id, lease := range m.active {
		if !now.Before(lease.expires) {
			delete(m.active, id)
			expired = append(expired, id)
		}
	}
	return expired
}

func (m *Manager) recordReplayLocked(key string) {
	if len(m.replayList) >= maxReplayIDs {
		delete(m.replays, m.replayList[0])
		m.replayList = m.replayList[1:]
	}
	m.replays[key] = struct{}{}
	m.replayList = append(m.replayList, key)
}

func randomID(source io.Reader) (string, error) {
	if source == nil {
		source = rand.Reader
	}
	value := make([]byte, 16)
	if _, err := io.ReadFull(source, value); err != nil {
		return "", err
	}
	return "lease-" + hex.EncodeToString(value), nil
}

func fault(code, stage, summary, remediation string) *Fault {
	return &Fault{Code: code, Stage: stage, Summary: summary, Remediation: remediation}
}
