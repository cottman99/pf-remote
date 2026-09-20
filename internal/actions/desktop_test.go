package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/cottman99/pf-remote/internal/activity"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type actionTestDesktop struct {
	target string
	route  string
	result DesktopRunResult
	err    error
}

func TestTailscaleVNCIsReadyWithoutAProtocolPassword(t *testing.T) {
	service := New()
	for index := range service.Catalog.Capabilities {
		if service.Catalog.Capabilities[index].ID == "desktop-main" {
			service.Catalog.Capabilities[index].DesktopProfile = &contracts.DesktopProfile{Protocol: "vnc", RenderingEnvironment: "physical", Authentication: "tailscale-device", VisualEffectsPolicy: "system"}
		}
	}
	target, err := service.Inspect("compute-node/desktop")
	if err != nil {
		t.Fatal(err)
	}
	if target.Target.LocalSetupState == "setup-required" {
		t.Fatal("Tailscale-authorized VNC must not ask for a second protocol password")
	}
	if _, err := service.SaveDesktopCredentialAndOpen(context.Background(), "compute-node/desktop", "unneeded"); err == nil {
		t.Fatal("Tailscale-authorized VNC must reject protocol password setup")
	}
}

func (r *actionTestDesktop) RunVia(ctx context.Context, target, route string) (DesktopRunResult, error) {
	r.target = target
	r.route = route
	r.result.RouteAdapter = route
	return r.result, r.err
}

func TestSuccessfulDesktopAppearsInSharedRecentSessions(t *testing.T) {
	runner := &actionTestDesktop{result: DesktopRunResult{SessionID: "desktop-session-activity", Protocol: "rdp", RenderingEnvironment: "virtual"}}
	service := New()
	service.Desktop = runner
	service.Activity = activity.New(20)
	if _, err := service.Open(context.Background(), "compute-node/desktop"); err != nil {
		t.Fatal(err)
	}
	recent := service.List().RecentSessions
	if len(recent) != 1 || recent[0].SessionID != "desktop-session-activity" || recent[0].Action != "open" || recent[0].CanonicalTarget != runner.target {
		t.Fatalf("recent sessions = %#v", recent)
	}
}

func (r *actionTestDesktop) Run(_ context.Context, target string) (DesktopRunResult, error) {
	r.target = target
	return r.result, r.err
}

func TestOpenResolvesDesktopAndReturnsSafeResult(t *testing.T) {
	runner := &actionTestDesktop{result: DesktopRunResult{SessionID: "desktop-session-01", Protocol: "rdp", RenderingEnvironment: "virtual"}}
	service := New()
	service.Desktop = runner
	response, err := service.Open(context.Background(), "compute-node/desktop")
	if err != nil {
		t.Fatal(err)
	}
	if response.SchemaVersion == "" || response.Status != "closed" || response.Protocol != "rdp" || runner.target != response.Target.Canonical {
		t.Fatalf("response=%#v target=%q", response, runner.target)
	}
}

func TestOpenViaPinsTheRequestedRouteForOneAction(t *testing.T) {
	runner := &actionTestDesktop{result: DesktopRunResult{SessionID: "desktop-session-route", Protocol: "rdp", RenderingEnvironment: "virtual"}}
	service := New()
	service.Desktop = runner
	response, err := service.OpenVia(context.Background(), "compute-node/desktop", "lan")
	if err != nil || runner.route != "lan" || response.RouteAdapter != "lan" {
		t.Fatalf("response=%#v runner=%#v err=%v", response, runner, err)
	}
}

func TestOpenRejectsShellAndUnavailableDesktop(t *testing.T) {
	service := New()
	if _, err := service.Open(context.Background(), "compute-node/shell"); err == nil {
		t.Fatal("expected non-Desktop target to fail")
	}
	_, err := service.Open(context.Background(), "compute-node/desktop")
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "DESKTOP_NOT_READY" {
		t.Fatalf("error=%#v", err)
	}
}

func TestOpenRejectsSetupRequiredDesktopBeforeRunner(t *testing.T) {
	runner := &actionTestDesktop{}
	service := New()
	service.Desktop = runner
	for index := range service.Catalog.Capabilities {
		if service.Catalog.Capabilities[index].ID == "desktop-main" {
			service.Catalog.Capabilities[index].State = "setup-required"
		}
	}
	_, err := service.Open(context.Background(), "compute-node/desktop")
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "CAPABILITY_SETUP_REQUIRED" || runner.target != "" {
		t.Fatalf("err=%#v runner=%#v", err, runner)
	}
}
