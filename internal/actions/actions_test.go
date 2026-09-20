package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/activity"
	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type actionTestShell struct {
	target  string
	command []string
	result  ShellRunResult
	err     error
}

func (s *actionTestShell) Run(_ context.Context, target string, command []string) (ShellRunResult, error) {
	s.target, s.command = target, append([]string(nil), command...)
	return s.result, s.err
}

func TestContextEnvelopeContainsNoNetworkLocation(t *testing.T) {
	got, err := New().Context("compute-node/shell", "inspect the workspace", []string{"read-only"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"192.0.2.", "pass" + "word", "token" + "="} {
		if strings.Contains(strings.ToLower(got.Envelope), strings.ToLower(forbidden)) {
			t.Fatalf("envelope contains forbidden value %q", forbidden)
		}
	}
	if !strings.HasPrefix(got.Envelope, "PF_REMOTE_TARGET/1\n") {
		t.Fatalf("unexpected envelope: %q", got.Envelope)
	}
}

func TestListAndDoctor_ReportExpiredCachedAuthorization(t *testing.T) {
	service := New()
	service.Catalog.CapturedAt = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	service.Catalog.Now = func() time.Time { return service.Catalog.CapturedAt.Add(catalog.DefaultAuthorizationTTL) }
	response := service.List()
	if response.Authorization.Status != "expired" || response.Authorization.RemainingSeconds != 0 || len(response.Targets) != 0 {
		t.Fatalf("expired catalog response = %#v", response)
	}
	for _, check := range service.Doctor().Checks {
		if check.Name == "catalog" && check.Status != "fail" {
			t.Fatalf("expired catalog doctor check = %#v", check)
		}
	}
}

func TestListChecksIndependentDesktopRoutesConcurrently(t *testing.T) {
	service := New()
	started := make(chan string, 2)
	release := make(chan struct{})
	service.RouteOptions = func(target string) []contracts.RouteOption {
		started <- target
		<-release
		return []contracts.RouteOption{{Adapter: "tailscale", Status: "available", Order: 1}}
	}
	done := make(chan contracts.CatalogResponse, 1)
	go func() { done <- service.List() }()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("independent Desktop route checks were serialized")
		}
	}
	close(release)
	response := <-done
	for _, target := range response.Targets {
		if target.Capability.Kind == contracts.CapabilityDesktop && len(target.RouteOptions) != 1 {
			t.Fatalf("route options missing for %s: %#v", target.Canonical, target.RouteOptions)
		}
	}
}

func TestDoctor_WithDeviceIdentity_ReportsProtectedIdentity(t *testing.T) {
	service := New()
	service.DeviceID = "device-synthetic"
	response := service.Doctor()
	for _, check := range response.Checks {
		if check.Name == "identity" {
			if check.Status != "pass" {
				t.Fatalf("identity status = %q", check.Status)
			}
			return
		}
	}
	t.Fatal("identity doctor check is missing")
}

func TestExecResolvesVisibleTargetAndReturnsAgentSafeResult(t *testing.T) {
	runner := &actionTestShell{result: ShellRunResult{SessionID: "session-action-01", ExitCode: 0, Output: "done\n"}}
	service := New()
	service.Shell = runner
	response, err := service.Exec(context.Background(), "compute-node/shell", []string{"echo", "done"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "completed" || response.Target.Canonical != runner.target || response.Output != "done\n" || len(runner.command) != 2 {
		t.Fatalf("response=%#v runner=%#v", response, runner)
	}
}

func TestSuccessfulAgentActionAppearsInSharedRecentSessionsWithoutCommandContent(t *testing.T) {
	runner := &actionTestShell{result: ShellRunResult{SessionID: "session-agent-activity", ExitCode: 0, Output: "done\n"}}
	service := New()
	service.Shell = runner
	service.Activity = activity.New(20)
	if _, err := service.Exec(context.Background(), "compute-node/shell", []string{"synthetic-private-command"}); err != nil {
		t.Fatal(err)
	}
	recent := service.List().RecentSessions
	if len(recent) != 1 || recent[0].Action != "exec" || strings.Contains(fmt.Sprint(recent), "synthetic-private-command") {
		t.Fatalf("recent sessions = %#v", recent)
	}
}

func TestShellActionsFailClearlyBeforeSetup(t *testing.T) {
	service := New()
	_, err := service.Connect(context.Background(), "compute-node/shell")
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "SHELL_NOT_READY" {
		t.Fatalf("err=%#v", err)
	}
}

func TestShellActionRejectsSetupRequiredTargetBeforeRunner(t *testing.T) {
	runner := &actionTestShell{}
	service := New()
	service.Shell = runner
	for index := range service.Catalog.Capabilities {
		if service.Catalog.Capabilities[index].ID == "shell-main" {
			service.Catalog.Capabilities[index].State = "setup-required"
		}
	}
	_, err := service.Exec(context.Background(), "compute-node/shell", []string{"echo", "blocked"})
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "CAPABILITY_SETUP_REQUIRED" || runner.target != "" {
		t.Fatalf("err=%#v runner=%#v", err, runner)
	}
}
