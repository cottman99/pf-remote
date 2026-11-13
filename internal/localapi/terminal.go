package localapi

import (
	"context"
	"errors"
	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"sync"
	"time"
)

func (h Handler) startTerminal(requestContext context.Context, s actions.Service, target string) Response {
	initial, e := s.Inspect(target)
	if e != nil || initial.Target.Capability.Kind != contracts.CapabilityShell || !initial.Target.Granted || initial.Target.Authorization.Status != "active" || initial.Target.Capability.State != "available" {
		return failed("TERMINAL_UNAVAILABLE", "authorize", "This terminal is not available.", "Refresh this computer and its authorization.")
	}
	runner, ok := s.Shell.(actions.TerminalRunner)
	if !ok {
		return failed("TERMINAL_UNAVAILABLE", "terminal", "Interactive terminal is not configured.", "Update this client and retry.")
	}
	leave := func() {}
	if h.Gate != nil {
		var entered bool
		leave, entered = h.Gate.Enter()
		if !entered {
			return failed("UPDATE_IN_PROGRESS", "maintenance", "PF Remote is updating.", "Retry shortly.")
		}
	}
	lifetime := h.Lifetime
	if lifetime == nil {
		lifetime = context.Background()
	}
	ctx, cancel := context.WithCancel(lifetime)
	ready := make(chan session.Session, 1)
	var startedOnce sync.Once
	done := make(chan error, 1)
	// The request may end after launch; ownership remains with the native process.
	go func() {
		defer leave()
		defer cancel()
		preflight, finishPreflight := context.WithTimeout(ctx, 20*time.Second)
		checked, e := s.Connect(preflight, initial.Target.Canonical)
		finishPreflight()
		if e != nil || checked.ExitCode != 0 {
			done <- errors.New("SSH authentication failed")
			return
		}
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					live := s
					if h.Provider != nil {
						var e error
						live, e = h.Provider()
						if e != nil {
							cancel()
							return
						}
					}
					i, e := live.Inspect(initial.Target.Canonical)
					if e != nil || !i.Target.Granted || i.Target.Authorization.Status != "active" || i.Target.Capability.State != "available" || i.Target.Capability.SSHBinding == nil || initial.Target.Capability.SSHBinding == nil || i.Target.Capability.SSHBinding.Signature != initial.Target.Capability.SSHBinding.Signature {
						cancel()
						return
					}
				}
			}
		}()
		result, e := runner.RunTerminal(ctx, initial.Target.Canonical, func(s session.Session) { startedOnce.Do(func() { ready <- s }) })
		if s.Activity != nil {
			status := "completed"
			if e != nil || result.ExitCode != 0 {
				status = "failed"
			}
			s.Activity.Record(result.SessionID, initial.Target.Canonical, "terminal", status)
		}
		if e == nil {
			e = errors.New("terminal exited before launch")
		}
		done <- e
	}()
	select {
	case started := <-ready:
		return Response{SchemaVersion: SchemaVersion, Result: contracts.ShellActionResponse{SchemaVersion: contracts.ShellActionSchema, Action: "terminal", Status: "launched", Target: initial.Target, SessionID: started.ID}}
	case <-done:
		cancel()
		return failed("TERMINAL_START_FAILED", "terminal", "The SSH terminal could not start.", "Check this computer's availability and saved SSH access, then retry.")
	case <-requestContext.Done():
		cancel()
		return failed("TERMINAL_START_CANCELLED", "terminal", "Terminal startup did not complete.", "Retry the connection.")
	}
}
