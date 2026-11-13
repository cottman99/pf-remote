package localapi

import (
	"context"
	"errors"
	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"sync/atomic"
	"testing"
	"time"
)

type terminalProbe struct {
	fail   bool
	calls  atomic.Int32
	closed chan struct{}
}

func (p *terminalProbe) Run(context.Context, string, []string) (actions.ShellRunResult, error) {
	if p.fail {
		return actions.ShellRunResult{}, errors.New("denied")
	}
	return actions.ShellRunResult{}, nil
}
func (p *terminalProbe) RunTerminal(ctx context.Context, target string, ready func(session.Session)) (actions.ShellRunResult, error) {
	p.calls.Add(1)
	ready(session.Session{ID: "terminal-test", CanonicalTarget: target})
	<-ctx.Done()
	close(p.closed)
	return actions.ShellRunResult{}, ctx.Err()
}
func TestTerminalOwnsLifetimeAndMaintenanceLease(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "daemon-stop", true: "revocation"}[revoke], func(t *testing.T) {
			s := actions.New()
			p := &terminalProbe{closed: make(chan struct{})}
			s.Shell = p
			s.Catalog.Capabilities[0].SSHBinding = &contracts.SSHCapabilityBinding{Signature: "synthetic-binding"}
			lifetime, stop := context.WithCancel(context.Background())
			defer stop()
			var revoked atomic.Bool
			gate := &ActionGate{}
			h := Handler{Lifetime: lifetime, Gate: gate, Provider: func() (actions.Service, error) {
				if revoked.Load() {
					return actions.Service{}, errors.New("revoked")
				}
				return s, nil
			}}
			request, cancel := context.WithCancel(context.Background())
			response := h.startTerminal(request, s, "compute-node/shell")
			cancel()
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			if response.Result.(contracts.ShellActionResponse).Status != "launched" {
				t.Fatal(response)
			}
			if gate.BeginUpdate() {
				t.Fatal("update can interrupt live terminal")
			}
			select {
			case <-p.closed:
				t.Fatal("request exit killed terminal")
			case <-time.After(30 * time.Millisecond):
			}
			if revoke {
				revoked.Store(true)
			} else {
				stop()
			}
			select {
			case <-p.closed:
			case <-time.After(3 * time.Second):
				t.Fatal("terminal not canceled")
			}
			until := time.Now().Add(time.Second)
			for !gate.BeginUpdate() {
				if time.Now().After(until) {
					t.Fatal("lease leaked")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}
func TestTerminalFailedAuthenticationNeverLaunches(t *testing.T) {
	s := actions.New()
	p := &terminalProbe{fail: true}
	s.Shell = p
	gate := &ActionGate{}
	response := (Handler{Gate: gate}).startTerminal(context.Background(), s, "compute-node/shell")
	if response.Error == nil || response.Error.Code != "TERMINAL_START_FAILED" || p.calls.Load() != 0 {
		t.Fatal(response)
	}
}
func TestTerminalRejectsUnavailableTargetAndMaintenance(t *testing.T) {
	s := actions.New()
	p := &terminalProbe{}
	s.Shell = p
	for _, target := range []string{"compute-node/desktop", "missing/shell"} {
		r := (Handler{}).startTerminal(context.Background(), s, target)
		if r.Error == nil || p.calls.Load() != 0 {
			t.Fatal(r)
		}
	}
	gate := &ActionGate{}
	gate.BeginUpdate()
	r := (Handler{Gate: gate}).startTerminal(context.Background(), s, "compute-node/shell")
	if r.Error == nil || r.Error.Code != "UPDATE_IN_PROGRESS" {
		t.Fatal(r)
	}
}
