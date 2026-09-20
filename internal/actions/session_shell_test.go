package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cottman99/pf-remote/internal/session"
)

type fakeSessionCoordinator struct {
	request session.Request
	result  session.Result
	err     error
	write   string
}

func (c *fakeSessionCoordinator) Run(_ context.Context, request session.Request) (session.Result, error) {
	c.request = request
	if c.write != "" {
		_, _ = request.Stdout.Write([]byte(c.write))
	}
	return c.result, c.err
}

func TestSessionShellKeepsInfrastructureOutOfCallerRequest(t *testing.T) {
	coordinator := &fakeSessionCoordinator{result: session.Result{Session: session.Session{ID: "session-agent-01"}, ExitCode: 0}, write: "complete\n"}
	runner := SessionShell{Coordinator: coordinator, RemoteUser: "operator", IdentityFile: "daemon-owned"}
	result, err := runner.Run(context.Background(), "pfremote://fabric-test/devices/device-target/capabilities/shell-main", []string{"echo", "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "session-agent-01" || result.Output != "complete\n" || coordinator.request.RemoteUser != "operator" || coordinator.request.IdentityFile != "daemon-owned" {
		t.Fatalf("result=%#v request=%#v", result, coordinator.request)
	}
}

func TestSessionShellBoundsOutputAndPreservesSafeFault(t *testing.T) {
	coordinator := &fakeSessionCoordinator{write: strings.Repeat("x", maxActionOutput+2)}
	result, err := (SessionShell{Coordinator: coordinator}).Run(context.Background(), "target", []string{"task"})
	if err != nil || !result.OutputExceeded || len(result.Output) != maxActionOutput {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	coordinator.write = ""
	coordinator.err = &session.Fault{Code: "ROUTE_UNAVAILABLE", Stage: "route", Summary: "The route is unavailable.", Remediation: "Try again."}
	_, err = (SessionShell{Coordinator: coordinator}).Run(context.Background(), "target", []string{"task"})
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "ROUTE_UNAVAILABLE" {
		t.Fatalf("err=%#v", err)
	}
}
