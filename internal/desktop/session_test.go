package desktop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type fixedResolver struct {
	target contracts.Target
}

func (r fixedResolver) ResolveAuthorized(string, string) (contracts.Target, error) {
	return r.target, nil
}

type captureProvider struct {
	calls int
	value *captureAcquisition
}

func (p *captureProvider) Acquire(_ context.Context, _ route.Request) (route.Acquisition, error) {
	p.calls++
	return p.value, nil
}

type captureAcquisition struct {
	closes int
}

func (a *captureAcquisition) Candidate() route.Candidate {
	return route.Candidate{ID: "desktop-route", Adapter: "lan", Network: "tcp", Address: "127.0.0.1", Port: 3389}
}
func (a *captureAcquisition) Close() error { a.closes++; return nil }

type captureExecutor struct {
	calls     int
	execution Execution
}

func (e *captureExecutor) Open(_ context.Context, execution Execution) error {
	e.calls++
	e.execution = execution
	return nil
}

func desktopTarget(now time.Time) contracts.Target {
	return contracts.Target{
		Canonical: "pfremote://fabric-test/devices/device-compute/capabilities/desktop-main",
		Alias:     "compute/desktop", Granted: true,
		Device:        contracts.Device{ID: "device-compute", Alias: "compute", DisplayName: "Compute", State: "online"},
		Capability:    contracts.Capability{ID: "desktop-main", DeviceID: "device-compute", Alias: "desktop", DisplayName: "Engineering desktop", Kind: contracts.CapabilityDesktop, State: "available", DesktopProfile: &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}},
		Authorization: contracts.Authorization{Status: "active", ValidUntil: now.Add(time.Hour)},
	}
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

func (r *mutableResolver) revoke() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.target.Device.State = "revoked"
}

type waitingExecutor struct {
	started chan struct{}
}

func (e waitingExecutor) Open(ctx context.Context, _ Execution) error {
	close(e.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestCoordinatorStopsDesktopWhenAuthorizationChanges(t *testing.T) {
	now := time.Now().UTC()
	resolver := &mutableResolver{target: desktopTarget(now)}
	started := make(chan struct{})
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: resolver,
		RouteProvider: &captureProvider{value: &captureAcquisition{}}, Executor: waitingExecutor{started: started},
		Now: func() time.Time { return now }, AuthorizationPollInterval: time.Millisecond,
	}
	done := make(chan error, 1)
	go func() {
		_, err := coordinator.Run(context.Background(), Request{Target: "compute/desktop"})
		done <- err
	}()
	<-started
	resolver.revoke()
	select {
	case err := <-done:
		var sessionFault *Fault
		if !errors.As(err, &sessionFault) || sessionFault.Code != "AUTHORIZATION_REVOKED" {
			t.Fatalf("error=%#v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("desktop session did not stop after revocation")
	}
}

func TestCoordinatorPinsIdentityRouteAndCleansUp(t *testing.T) {
	now := time.Now().UTC()
	acquisition := &captureAcquisition{}
	provider := &captureProvider{value: acquisition}
	executor := &captureExecutor{}
	coordinator := Coordinator{
		SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: desktopTarget(now)},
		RouteProvider: provider, Executor: executor, Now: func() time.Time { return now },
		NewID: func() (string, error) { return "desktop-session-01", nil },
	}
	result, err := coordinator.Run(context.Background(), Request{Target: "compute/desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.TargetDeviceID != "device-compute" || result.Session.CapabilityID != "desktop-main" || result.Session.Protocol != "rdp" || result.Session.VisualEffectsPolicy != "automatic" || executor.calls != 1 || acquisition.closes != 1 {
		t.Fatalf("result=%#v executor=%d closes=%d", result, executor.calls, acquisition.closes)
	}
}

func TestVisualEffectsPolicyIsPlatformAccurateAndBackwardCompatible(t *testing.T) {
	tests := []struct {
		name    string
		profile contracts.DesktopProfile
		want    string
		valid   bool
	}{
		{name: "legacy virtual defaults to protocol automatic", profile: contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}, want: "automatic", valid: true},
		{name: "legacy physical preserves system", profile: contracts.DesktopProfile{Protocol: "vnc", RenderingEnvironment: "physical", Authentication: "tailscale-device"}, want: "system", valid: true},
		{name: "virtual reduced is target declared", profile: contracts.DesktopProfile{Protocol: "vnc", RenderingEnvironment: "virtual", Authentication: "x509-route-grant", VisualEffectsPolicy: "reduced"}, want: "reduced", valid: true},
		{name: "physical cannot inherit virtual reduction", profile: contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "physical", Authentication: "windows-sso", VisualEffectsPolicy: "reduced"}, want: "", valid: false},
		{name: "unknown policy fails closed", profile: contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso", VisualEffectsPolicy: "disable-dwm"}, want: "", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effectiveVisualEffectsPolicy(test.profile); got != test.want {
				t.Fatalf("policy=%q want=%q", got, test.want)
			}
			if got := validProfile(test.profile); got != test.valid {
				t.Fatalf("valid=%v want=%v", got, test.valid)
			}
		})
	}
}

func TestCoordinatorRejectsChangedIdentityBeforeRoute(t *testing.T) {
	now := time.Now().UTC()
	target := desktopTarget(now)
	target.Capability.ID = "desktop-other"
	provider := &captureProvider{value: &captureAcquisition{}}
	executor := &captureExecutor{}
	coordinator := Coordinator{SubjectDeviceID: "device-controller", Resolver: fixedResolver{target: target}, RouteProvider: provider, Executor: executor, Now: func() time.Time { return now }}
	if _, err := coordinator.Run(context.Background(), Request{Target: "compute/desktop"}); err == nil {
		t.Fatal("expected changed immutable identity to fail")
	}
	if provider.calls != 0 || executor.calls != 0 {
		t.Fatalf("route calls=%d executor calls=%d", provider.calls, executor.calls)
	}
}
