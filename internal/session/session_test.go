package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type fixedResolver struct {
	target contracts.Target
	err    error
}

func (r fixedResolver) ResolveAuthorized(string, string) (contracts.Target, error) {
	return r.target, r.err
}

type recordingResolver struct {
	target  contracts.Target
	subject string
}

func (r *recordingResolver) ResolveAuthorized(subject, _ string) (contracts.Target, error) {
	r.subject = subject
	return r.target, nil
}

type mutableResolver struct {
	mu     sync.Mutex
	target contracts.Target
}

func (r *mutableResolver) ResolveAuthorized(string, string) (contracts.Target, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.target, nil
}

func (r *mutableResolver) revokeTarget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.target.Device.State = "revoked"
}

type captureExecutor struct {
	mu        sync.Mutex
	calls     int
	execution Execution
	wait      bool
}

func (e *captureExecutor) Execute(ctx context.Context, execution Execution) (int, error) {
	e.mu.Lock()
	e.calls++
	e.execution = execution
	e.mu.Unlock()
	if e.wait {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	return 7, nil
}

type immediateWatcher struct{ err error }

func (w immediateWatcher) Wait(context.Context, Session) error { return w.err }

type captureRouteProvider struct {
	request route.Request
	value   *captureAcquisition
	err     error
	calls   int
}

func (p *captureRouteProvider) Acquire(_ context.Context, request route.Request) (route.Acquisition, error) {
	p.calls++
	p.request = request
	return p.value, p.err
}

type captureAcquisition struct {
	candidate route.Candidate
	closes    int
}

func (a *captureAcquisition) Candidate() route.Candidate { return a.candidate }
func (a *captureAcquisition) Close() error               { a.closes++; return nil }

type fixtureSigner struct {
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func newFixtureSigner(t *testing.T) fixtureSigner {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x57}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	id, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureSigner{id: id, public: public, private: private}
}

func (s fixtureSigner) DeviceID() string             { return s.id }
func (s fixtureSigner) PublicKey() ed25519.PublicKey { return s.public }
func (s fixtureSigner) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(s.private, message), nil
}

func sessionTarget(t *testing.T, now time.Time) contracts.Target {
	t.Helper()
	signer := newFixtureSigner(t)
	algorithm := "ssh-ed25519"
	var blob bytes.Buffer
	_ = binary.Write(&blob, binary.BigEndian, uint32(len(algorithm)))
	blob.WriteString(algorithm)
	_ = binary.Write(&blob, binary.BigEndian, uint32(ed25519.PublicKeySize))
	blob.Write(bytes.Repeat([]byte{0x31}, ed25519.PublicKeySize))
	binding, err := shellbinding.Sign("fabric-test", "shell-main", 1, []contracts.SSHHostKey{{
		Algorithm: algorithm, PublicKey: base64.RawStdEncoding.EncodeToString(blob.Bytes()),
	}}, signer)
	if err != nil {
		t.Fatal(err)
	}
	device := contracts.Device{
		ID: signer.id, Alias: "compute", DisplayName: "Compute", State: "online",
		IdentityPublicKey: base64.RawURLEncoding.EncodeToString(signer.public),
	}
	capability := contracts.Capability{
		ID: "shell-main", DeviceID: signer.id, Alias: "shell", DisplayName: "Shell",
		Kind: contracts.CapabilityShell, State: "available", SSHBinding: binding,
	}
	return contracts.Target{
		Canonical: "pfremote://fabric-test/devices/" + signer.id + "/capabilities/shell-main",
		Alias:     "compute/shell", Device: device, Capability: capability, Granted: true,
		Authorization: contracts.Authorization{Status: "active", ValidUntil: now.Add(time.Hour)},
	}
}

func sessionRequest() Request {
	return Request{
		Target: "compute/shell", RemoteUser: "operator",
		Route:   route.Candidate{ID: "route-local-1", Adapter: "local", Network: "tcp", Address: "127.0.0.1", Port: 2222},
		Command: []string{"printf", "proof"}, Stdin: bytes.NewReader(nil), Stdout: io.Discard, Stderr: io.Discard,
	}
}

func TestCoordinatorPinsVerifiedTargetRouteAndFreshSession(t *testing.T) {
	now := time.Now().UTC()
	target := sessionTarget(t, now)
	executor := &captureExecutor{}
	ids := []string{"session-first", "session-second"}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: target}, Executor: executor, Now: func() time.Time { return now },
		NewID: func() (string, error) { id := ids[0]; ids = ids[1:]; return id, nil },
	}
	first, err := coordinator.Run(context.Background(), sessionRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Run(context.Background(), sessionRequest())
	if err != nil {
		t.Fatal(err)
	}
	if first.ExitCode != 7 || first.Session.ID == second.Session.ID || executor.calls != 2 {
		t.Fatalf("first=%#v second=%#v calls=%d", first, second, executor.calls)
	}
	if first.Session.FabricID != "fabric-test" || first.Session.TargetDeviceID != target.Device.ID ||
		first.Session.CapabilityID != "shell-main" || first.Session.RouteID != "route-local-1" ||
		len(first.Session.HostKeyFingerprints) != 1 {
		t.Fatalf("session was not fully pinned: %#v", first.Session)
	}
	target.Capability.SSHBinding.HostKeys[0].PublicKey = "mutated"
	if executor.execution.Binding.HostKeys[0].PublicKey == "mutated" {
		t.Fatal("executor retained mutable catalog binding state")
	}
}

func TestCoordinatorAcquiresGatewayRouteAfterTargetAuthenticationAndCleansIt(t *testing.T) {
	now := time.Now().UTC()
	target := sessionTarget(t, now)
	acquired := &captureAcquisition{candidate: route.Candidate{ID: "lease-route-01", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: 24680}}
	provider := &captureRouteProvider{value: acquired}
	executor := &captureExecutor{}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: target}, RouteProvider: provider,
		Executor: executor, Now: func() time.Time { return now }, NewID: func() (string, error) { return "session-frp-route", nil },
	}
	request := sessionRequest()
	request.Route = route.Candidate{}
	result, err := coordinator.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || provider.request.CanonicalTarget != target.Canonical || provider.request.SubjectDeviceID != coordinator.SubjectDeviceID ||
		!provider.request.AuthorizationExpiry.Equal(target.Authorization.ValidUntil) {
		t.Fatalf("route request was not bound to verified authorization: %#v", provider.request)
	}
	if result.Session.RouteAdapter != "frp" || result.Session.RouteID != acquired.candidate.ID || acquired.closes != 1 {
		t.Fatalf("result=%#v closes=%d", result, acquired.closes)
	}
	if executor.execution.Route != acquired.candidate {
		t.Fatalf("executor route = %#v", executor.execution.Route)
	}
}

func TestCoordinatorDoesNotAcquireRouteForInvalidTargetBinding(t *testing.T) {
	now := time.Now().UTC()
	target := sessionTarget(t, now)
	target.Capability.SSHBinding.BindingVersion++
	provider := &captureRouteProvider{value: &captureAcquisition{candidate: route.Candidate{ID: "lease-route-01", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: 24680}}}
	coordinator := Coordinator{SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: target}, RouteProvider: provider, Executor: &captureExecutor{}, Now: func() time.Time { return now }}
	request := sessionRequest()
	request.Route = route.Candidate{}
	if _, err := coordinator.Run(context.Background(), request); err == nil {
		t.Fatal("tampered target binding was accepted")
	}
	if provider.calls != 0 {
		t.Fatalf("route acquired before target authentication: %d", provider.calls)
	}
}

func TestCoordinatorPinsSelectedFallbackAndSafeDiagnostics(t *testing.T) {
	now := time.Now().UTC()
	frp := &captureRouteProvider{err: errors.New("private gateway detail")}
	tailscaleRoute := &captureAcquisition{candidate: route.Candidate{ID: "route-tailscale-01", Adapter: "tailscale", Network: "tcp", Address: "192.0.2.20", Port: 22}}
	selector := route.Selector{Providers: []route.NamedProvider{
		{Adapter: "frp", Provider: frp},
		{Adapter: "tailscale", Provider: &captureRouteProvider{value: tailscaleRoute}},
	}}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: sessionTarget(t, now)},
		RouteProvider: selector, Executor: &captureExecutor{}, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "session-selected-fallback", nil },
	}
	request := sessionRequest()
	request.Route = route.Candidate{}
	result, err := coordinator.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.RouteAdapter != "tailscale" || len(result.Session.RouteDiagnostics) != 2 ||
		result.Session.RouteDiagnostics[0].Code != "ACQUIRE_FAILED" || strings.Contains(fmt.Sprint(result.Session.RouteDiagnostics), "private") {
		t.Fatalf("session=%#v", result.Session)
	}
}

func TestCoordinatorBindsClaimedSubjectToAuthorizationResolution(t *testing.T) {
	now := time.Now().UTC()
	resolver := &recordingResolver{target: sessionTarget(t, now)}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: resolver, Executor: &captureExecutor{}, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "session-subject-bound", nil },
	}
	request := sessionRequest()
	if _, err := coordinator.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if resolver.subject != coordinator.SubjectDeviceID {
		t.Fatalf("resolved subject = %q, want %q", resolver.subject, coordinator.SubjectDeviceID)
	}
}

func TestCoordinatorRejectsBeforeExecutor(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	tests := map[string]func(*contracts.Target){
		"not granted":            func(target *contracts.Target) { target.Granted = false },
		"expired":                func(target *contracts.Target) { target.Authorization.ValidUntil = now },
		"revoked Device":         func(target *contracts.Target) { target.Device.State = "revoked" },
		"unavailable Capability": func(target *contracts.Target) { target.Capability.State = "offline" },
		"non-Shell Capability":   func(target *contracts.Target) { target.Capability.Kind = contracts.CapabilityDesktop },
		"tampered binding":       func(target *contracts.Target) { target.Capability.SSHBinding.BindingVersion++ },
		"inconsistent canonical target": func(target *contracts.Target) {
			target.Canonical = "pfremote://fabric-test/devices/device-other/capabilities/shell-main"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			target := sessionTarget(t, now)
			mutate(&target)
			executor := &captureExecutor{}
			coordinator := Coordinator{SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: target}, Executor: executor, Now: func() time.Time { return now }}
			if _, err := coordinator.Run(context.Background(), sessionRequest()); err == nil {
				t.Fatal("invalid session was accepted")
			} else if name == "tampered binding" {
				var structured *Fault
				if !errors.As(err, &structured) || structured.Stage != "target-auth" || structured.Code != "TARGET_IDENTITY_INVALID" {
					t.Fatalf("unstructured target-auth failure: %#v", err)
				}
			}
			if executor.calls != 0 {
				t.Fatalf("executor called %d times", executor.calls)
			}
		})
	}
}

func TestCoordinatorStopsExecutionOnObservedRevocation(t *testing.T) {
	now := time.Now().UTC()
	executor := &captureExecutor{wait: true}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: sessionTarget(t, now)}, Executor: executor,
		Watcher: immediateWatcher{err: errors.New("binding revoked")}, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "session-revoked", nil },
	}
	_, err := coordinator.Run(context.Background(), sessionRequest())
	if !errors.Is(err, ErrAuthorizationRevoked) || executor.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, executor.calls)
	}
}

func TestCoordinatorPollsAndStopsOnCatalogRevocation(t *testing.T) {
	now := time.Now().UTC()
	resolver := &mutableResolver{target: sessionTarget(t, now)}
	executor := &captureExecutor{wait: true}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: resolver, Executor: executor,
		AuthorizationPollInterval: 10 * time.Millisecond, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "session-polled-revocation", nil },
	}
	done := make(chan error, 1)
	go func() {
		_, err := coordinator.Run(context.Background(), sessionRequest())
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		executor.mu.Lock()
		started := executor.calls == 1
		executor.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("executor did not start")
		}
		time.Sleep(time.Millisecond)
	}
	resolver.revokeTarget()
	select {
	case err := <-done:
		if !errors.Is(err, ErrAuthorizationRevoked) {
			t.Fatalf("revocation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("polled revocation did not stop the Session")
	}
}

func TestCoordinatorStopsExecutionAtAuthorizationExpiry(t *testing.T) {
	now := time.Now().UTC()
	target := sessionTarget(t, now)
	target.Authorization.ValidUntil = now.Add(150 * time.Millisecond)
	executor := &captureExecutor{wait: true}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: target}, Executor: executor, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "session-expired", nil },
	}
	started := time.Now()
	_, err := coordinator.Run(context.Background(), sessionRequest())
	if !errors.Is(err, ErrAuthorizationExpired) || executor.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, executor.calls)
	}
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("authorization expiry elapsed = %s", elapsed)
	}
}

func TestCoordinatorRedactsCallerCancellationCause(t *testing.T) {
	now := time.Now().UTC()
	executor := &captureExecutor{wait: true}
	acquired := &captureAcquisition{candidate: route.Candidate{ID: "lease-cancel-01", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: 24682}}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: sessionTarget(t, now)},
		RouteProvider: &captureRouteProvider{value: acquired}, Executor: executor, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "session-canceled", nil },
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	go cancel(errors.New("private-path-and-command"))
	request := sessionRequest()
	request.Route = route.Candidate{}
	_, err := coordinator.Run(ctx, request)
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-path") {
		t.Fatalf("unsafe cancellation error: %v", err)
	}
	if acquired.closes != 1 {
		t.Fatalf("canceled Session did not clean its route: %d", acquired.closes)
	}
}
