package actions

import (
	"context"
	"errors"
	"strings"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const maxActionOutput = 1 << 20

type ShellRunner interface {
	Run(context.Context, string, []string) (ShellRunResult, error)
}

type ShellRunResult struct {
	SessionID      string
	ExitCode       int
	Output         string
	ErrorOutput    string
	OutputExceeded bool
}

type Fault struct {
	Code        string
	Stage       string
	Summary     string
	Remediation string
}

func (f *Fault) Error() string { return f.Summary }

func (s Service) Connect(ctx context.Context, target string) (contracts.ShellActionResponse, error) {
	return s.runShell(ctx, "connect", target, []string{"exit"})
}

func (s Service) Exec(ctx context.Context, target string, command []string) (contracts.ShellActionResponse, error) {
	if len(command) == 0 {
		return contracts.ShellActionResponse{}, &Fault{Code: "INVALID_COMMAND", Stage: "exec", Summary: "No remote task was provided.", Remediation: "Describe one command for the selected computer."}
	}
	for _, argument := range command {
		if strings.TrimSpace(argument) == "" || len(argument) > 32<<10 {
			return contracts.ShellActionResponse{}, &Fault{Code: "INVALID_COMMAND", Stage: "exec", Summary: "The remote task contains an invalid argument.", Remediation: "Provide a bounded command without empty arguments."}
		}
	}
	return s.runShell(ctx, "exec", target, command)
}

func (s Service) runShell(ctx context.Context, action, input string, command []string) (contracts.ShellActionResponse, error) {
	inspected, err := s.Inspect(input)
	if err != nil {
		return contracts.ShellActionResponse{}, &Fault{Code: "TARGET_NOT_FOUND", Stage: "resolve", Summary: "The selected computer is unavailable.", Remediation: "Refresh the computer list and choose an available Shell target."}
	}
	if inspected.Target.Capability.Kind != contracts.CapabilityShell {
		return contracts.ShellActionResponse{}, &Fault{Code: "SHELL_REQUIRED", Stage: "authorize", Summary: "The selected capability is not remote work.", Remediation: "Choose the Automation action for this computer."}
	}
	if !strings.EqualFold(inspected.Target.Capability.State, "available") {
		return contracts.ShellActionResponse{}, &Fault{Code: "CAPABILITY_SETUP_REQUIRED", Stage: "authorize", Summary: "Remote work is still being prepared for this computer.", Remediation: "Wait until PF Remote shows this action as available, then try again."}
	}
	if s.Shell == nil {
		return contracts.ShellActionResponse{}, &Fault{Code: "SHELL_NOT_READY", Stage: action, Summary: "Remote work is not ready on this computer.", Remediation: "Finish Shell setup for this computer and try again."}
	}
	result, err := s.Shell.Run(ctx, inspected.Target.Canonical, append([]string(nil), command...))
	if err != nil {
		var fault *Fault
		if errors.As(err, &fault) {
			return contracts.ShellActionResponse{}, fault
		}
		return contracts.ShellActionResponse{}, &Fault{Code: "REMOTE_ACTION_FAILED", Stage: action, Summary: "PF Remote could not complete the remote task.", Remediation: "Check the visible computer status and try again."}
	}
	if result.OutputExceeded || len(result.Output) > maxActionOutput || len(result.ErrorOutput) > maxActionOutput {
		return contracts.ShellActionResponse{}, &Fault{Code: "OUTPUT_LIMIT_EXCEEDED", Stage: action, Summary: "The remote result is too large to display safely.", Remediation: "Run a narrower task that produces less output."}
	}
	response := contracts.ShellActionResponse{SchemaVersion: contracts.ShellActionSchema, Action: action, Status: "completed",
		Target: inspected.Target, SessionID: result.SessionID, ExitCode: result.ExitCode, Output: result.Output, ErrorOutput: result.ErrorOutput}
	if s.Activity != nil {
		s.Activity.Record(result.SessionID, inspected.Target.Canonical, action, "completed")
	}
	return response, nil
}
