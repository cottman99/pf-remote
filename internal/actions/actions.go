package actions

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/cottman99/pf-remote/internal/activity"
	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type Service struct {
	Catalog            catalog.Catalog
	DeviceID           string
	Shell              ShellRunner
	Desktop            DesktopRunner
	DesktopCredentials DesktopCredentialWriter
	AdditionalChecks   []contracts.Check
	Activity           *activity.Store
	RouteOptions       func(string) []contracts.RouteOption
}

type DesktopCredentialWriter interface {
	Save(string, string) error
	Configured(string) bool
}

func New() Service { return Service{Catalog: catalog.Synthetic()} }

func (s Service) List() contracts.CatalogResponse {
	targets := s.Catalog.List()
	routeOptions := make([][]contracts.RouteOption, len(targets))
	if s.RouteOptions != nil {
		var wait sync.WaitGroup
		limit := make(chan struct{}, 8)
		for index := range targets {
			if targets[index].Capability.Kind != contracts.CapabilityDesktop {
				continue
			}
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				limit <- struct{}{}
				defer func() { <-limit }()
				routeOptions[index] = append([]contracts.RouteOption(nil), s.RouteOptions(targets[index].Canonical)...)
			}(index)
		}
		wait.Wait()
	}
	for index := range targets {
		if routeOptions[index] != nil {
			targets[index].RouteOptions = routeOptions[index]
		}
		targets[index] = s.withDesktopSetupState(targets[index])
	}
	return contracts.CatalogResponse{
		SchemaVersion: contracts.CatalogSchema, FabricID: s.Catalog.FabricID,
		Targets: targets, Authorization: s.Catalog.Authorization(), RecentSessions: s.Activity.Snapshot(),
	}
}

func (s Service) Inspect(input string) (contracts.InspectResponse, error) {
	target, err := s.Catalog.Resolve(input)
	if err != nil {
		return contracts.InspectResponse{}, err
	}
	return contracts.InspectResponse{SchemaVersion: contracts.InspectSchema, Target: s.withLocalSetupState(target)}, nil
}

func (s Service) withLocalSetupState(target contracts.Target) contracts.Target {
	if s.RouteOptions != nil && target.Capability.Kind == contracts.CapabilityDesktop {
		target.RouteOptions = append([]contracts.RouteOption(nil), s.RouteOptions(target.Canonical)...)
	}
	return s.withDesktopSetupState(target)
}

func (s Service) withDesktopSetupState(target contracts.Target) contracts.Target {
	profile := target.Capability.DesktopProfile
	if profile == nil || profile.Protocol != "vnc" || profile.Authentication != "legacy-vnc-password" {
		return target
	}
	target.LocalSetupState = "setup-required"
	if s.DesktopCredentials != nil && s.DesktopCredentials.Configured(target.Canonical) {
		target.LocalSetupState = "ready"
	}
	return target
}

func (s Service) Context(input, task string, constraints []string) (contracts.ContextResponse, error) {
	inspected, err := s.Inspect(input)
	if err != nil {
		return contracts.ContextResponse{}, err
	}
	t := inspected.Target
	envelope := strings.Join([]string{
		"PF_REMOTE_TARGET/1",
		"target: " + t.Canonical,
		"alias: " + t.Alias,
		"kind: " + string(t.Capability.Kind),
		"verify: pfremote inspect " + t.Canonical + " --json",
	}, "\n")
	return contracts.ContextResponse{
		SchemaVersion: contracts.ContextSchema, Target: t, Envelope: envelope,
		Task: strings.TrimSpace(task), Constraints: constraints,
	}, nil
}

func (s Service) Doctor() contracts.DoctorResponse {
	identityCheck := contracts.Check{Name: "identity", Status: "pending", Summary: "Device identity begins in milestone M2"}
	if s.DeviceID != "" {
		identityCheck = contracts.Check{Name: "identity", Status: "pass", Summary: "Protected Device identity is available"}
	}
	catalogAuthorization := s.Catalog.Authorization()
	catalogCheck := contracts.Check{Name: "catalog", Status: "pass", Summary: fmt.Sprintf("Authorized catalog contains %d targets", len(s.Catalog.List()))}
	if catalogAuthorization.Status == "expired" {
		catalogCheck = contracts.Check{Name: "catalog", Status: "fail", Summary: "Cached authorization expired; refresh is required"}
	}
	checks := []contracts.Check{
		{Name: "runtime", Status: "pass", Summary: fmt.Sprintf("Go runtime %s on %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)},
		catalogCheck,
		identityCheck,
		{Name: "routes", Status: "pending", Summary: "No real route adapters are enabled in the clean-room slice"},
	}
	checks = append(checks, s.AdditionalChecks...)
	return contracts.DoctorResponse{SchemaVersion: contracts.DoctorSchema, Overall: "development", Checks: checks}
}
