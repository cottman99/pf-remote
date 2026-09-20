package frp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
)

type fakeLeaseClient struct {
	lease       Lease
	err         error
	released    []string
	releaseErr  error
	lastRequest route.Request
}

func (c *fakeLeaseClient) Acquire(_ context.Context, request route.Request) (Lease, error) {
	c.lastRequest = request
	return c.lease, c.err
}

func (c *fakeLeaseClient) Release(_ context.Context, leaseID string) error {
	c.released = append(c.released, leaseID)
	return c.releaseErr
}

type fakeStarter struct {
	process *fakeProcess
	err     error
	path    string
	content string
}

func (s *fakeStarter) Start(_ context.Context, _, configPath string) (Process, error) {
	s.path = configPath
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	s.content = string(content)
	if s.err != nil {
		return nil, s.err
	}
	if s.process == nil {
		s.process = &fakeProcess{}
	}
	return s.process, nil
}

type fakeProcess struct{ stops int }

func (p *fakeProcess) Stop() error { p.stops++; return nil }

func testRequest(now time.Time) route.Request {
	return route.Request{
		SubjectDeviceID: "device-controller", CanonicalTarget: "pfremote://fabric-test/devices/device-target/capabilities/shell-main",
		AuthorizationExpiry: now.Add(time.Hour),
	}
}

func testLease(now time.Time) Lease {
	return Lease{
		SchemaVersion: LeaseSchema, ID: "lease-example-01", Adapter: "frp",
		CanonicalTarget: "pfremote://fabric-test/devices/device-target/capabilities/shell-main",
		ServerAddress:   "relay.example.com", ServerPort: 7000, ServerUser: "fabric-test",
		ServerName: "proxy-example-01", SecretKey: "fx_" + "adapter_secret_0123456789", ExpiresAt: now.Add(5 * time.Minute),
	}
}

func TestAdapterAcquiresLoopbackCandidateAndCleansOwnedResources(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	client := &fakeLeaseClient{lease: testLease(now)}
	starter := &fakeStarter{}
	adapter := Adapter{
		Binary: "frpc-test", StateDir: t.TempDir(), AuthToken: "fx_" + "gateway_adapter_token",
		Client: client, Starter: starter, Now: func() time.Time { return now },
		NewPort: func() (uint16, error) { return 24680, nil }, Probe: func(context.Context, string) error { return nil },
	}
	acquired, err := adapter.Acquire(context.Background(), testRequest(now))
	if err != nil {
		t.Fatal(err)
	}
	candidate := acquired.Candidate()
	if candidate.Adapter != "frp" || candidate.Address != "127.0.0.1" || candidate.Port != 24680 || candidate.ID != client.lease.ID {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	for _, required := range []string{
		`type = "stcp"`, `bindAddr = "127.0.0.1"`, `bindPort = 24680`,
		`transport.tls.enable = true`, `transport.useEncryption = true`,
	} {
		if !strings.Contains(starter.content, required) {
			t.Fatalf("visitor config missing %q", required)
		}
	}
	if err := acquired.Close(); err != nil {
		t.Fatal(err)
	}
	if starter.process.stops != 1 || len(client.released) != 1 || client.released[0] != client.lease.ID {
		t.Fatalf("stops=%d releases=%v", starter.process.stops, client.released)
	}
	if _, err := os.Stat(starter.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session configuration survived cleanup: %v", err)
	}
	if err := acquired.Close(); err != nil || starter.process.stops != 1 || len(client.released) != 1 {
		t.Fatalf("cleanup is not idempotent: err=%v stops=%d releases=%v", err, starter.process.stops, client.released)
	}
}

func TestAdapterFailsClosedAndReleasesInvalidLease(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	lease := testLease(now)
	lease.CanonicalTarget = "pfremote://fabric-test/devices/device-other/capabilities/shell-main"
	client := &fakeLeaseClient{lease: lease}
	starter := &fakeStarter{}
	adapter := Adapter{Binary: "frpc-test", StateDir: t.TempDir(), Client: client, Starter: starter, Now: func() time.Time { return now }}
	_, err := adapter.Acquire(context.Background(), testRequest(now))
	if err == nil || starter.path != "" || len(client.released) != 1 {
		t.Fatalf("err=%v starter=%q releases=%v", err, starter.path, client.released)
	}
	if strings.Contains(err.Error(), lease.SecretKey) || strings.Contains(err.Error(), lease.ServerAddress) {
		t.Fatalf("secret-bearing lease leaked through error: %v", err)
	}
}

func TestAdapterCleansUpAfterStartAndReadinessFailures(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	for name, starterError := range map[string]error{"start": errors.New("private process detail"), "readiness": nil} {
		t.Run(name, func(t *testing.T) {
			client := &fakeLeaseClient{lease: testLease(now)}
			starter := &fakeStarter{err: starterError}
			ctx, cancel := context.WithCancel(context.Background())
			if name == "readiness" {
				cancel()
			} else {
				defer cancel()
			}
			adapter := Adapter{
				Binary: "frpc-test", StateDir: t.TempDir(), Client: client, Starter: starter, Now: func() time.Time { return now },
				NewPort: func() (uint16, error) { return 24681, nil }, Probe: func(context.Context, string) error { return errors.New("not ready") },
			}
			_, err := adapter.Acquire(ctx, testRequest(now))
			if err == nil || strings.Contains(err.Error(), "private") || len(client.released) != 1 {
				t.Fatalf("err=%v releases=%v", err, client.released)
			}
			if starter.path != "" {
				if _, statErr := os.Stat(starter.path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("configuration survived failed acquisition: %v", statErr)
				}
			}
			if name == "readiness" && starter.process.stops != 1 {
				t.Fatalf("visitor process was not stopped: %d", starter.process.stops)
			}
		})
	}
}
