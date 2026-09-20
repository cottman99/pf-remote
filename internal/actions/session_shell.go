package actions

import (
	"context"
	"errors"

	"github.com/cottman99/pf-remote/internal/session"
)

type SessionShell struct {
	Coordinator  SessionCoordinator
	RemoteUser   string
	IdentityFile string
}

type SessionCoordinator interface {
	Run(context.Context, session.Request) (session.Result, error)
}

func (s SessionShell) Run(ctx context.Context, target string, command []string) (ShellRunResult, error) {
	if s.Coordinator == nil {
		return ShellRunResult{}, &Fault{Code: "SHELL_NOT_READY", Stage: "session", Summary: "Remote work is not configured.", Remediation: "Finish Shell setup for this computer and try again."}
	}
	stdout := &boundedOutput{limit: maxActionOutput}
	stderr := &boundedOutput{limit: maxActionOutput}
	result, err := s.Coordinator.Run(ctx, session.Request{
		Target: target, RemoteUser: s.RemoteUser, IdentityFile: s.IdentityFile,
		Command: append([]string(nil), command...), Stdout: stdout, Stderr: stderr,
	})
	if err != nil {
		var sessionFault *session.Fault
		if errors.As(err, &sessionFault) {
			return ShellRunResult{}, &Fault{Code: sessionFault.Code, Stage: sessionFault.Stage, Summary: sessionFault.Summary, Remediation: sessionFault.Remediation}
		}
		return ShellRunResult{}, err
	}
	return ShellRunResult{SessionID: result.Session.ID, ExitCode: result.ExitCode, Output: stdout.String(), ErrorOutput: stderr.String(), OutputExceeded: stdout.exceeded || stderr.exceeded}, nil
}

type boundedOutput struct {
	value    []byte
	limit    int
	exceeded bool
}

func (w *boundedOutput) Write(value []byte) (int, error) {
	remaining := w.limit + 1 - len(w.value)
	if remaining > 0 {
		count := len(value)
		if count > remaining {
			count = remaining
		}
		w.value = append(w.value, value[:count]...)
	}
	if len(w.value) > w.limit || len(value) > remaining {
		w.exceeded = true
	}
	return len(value), nil
}

func (w *boundedOutput) String() string {
	if len(w.value) > w.limit {
		return string(w.value[:w.limit])
	}
	return string(w.value)
}

var _ ShellRunner = SessionShell{}
