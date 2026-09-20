package actions

import (
	"context"
	"errors"
	"strings"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

type DesktopRunner interface {
	Run(context.Context, string) (DesktopRunResult, error)
}

type RoutedDesktopRunner interface {
	DesktopRunner
	RunVia(context.Context, string, string) (DesktopRunResult, error)
}

type DesktopRunResult struct {
	SessionID            string
	Protocol             string
	RenderingEnvironment string
	RouteAdapter         string
}

func (s Service) Open(ctx context.Context, input string) (contracts.DesktopActionResponse, error) {
	return s.OpenVia(ctx, input, "")
}

func (s Service) OpenVia(ctx context.Context, input, routeAdapter string) (contracts.DesktopActionResponse, error) {
	inspected, err := s.Inspect(input)
	if err != nil {
		return contracts.DesktopActionResponse{}, &Fault{Code: "TARGET_NOT_FOUND", Stage: "resolve", Summary: "The selected computer is unavailable.", Remediation: "Refresh the computer list and choose an available Desktop target."}
	}
	target := inspected.Target
	if target.Capability.Kind != contracts.CapabilityDesktop {
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_REQUIRED", Stage: "authorize", Summary: "The selected capability is not a desktop.", Remediation: "Choose a visible Desktop capability for this computer."}
	}
	if !strings.EqualFold(target.Capability.State, "available") {
		return contracts.DesktopActionResponse{}, &Fault{Code: "CAPABILITY_SETUP_REQUIRED", Stage: "authorize", Summary: "This desktop is still being prepared.", Remediation: "Wait until PF Remote shows this desktop as available, then try again."}
	}
	if target.Capability.DesktopProfile == nil {
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_PROFILE_MISSING", Stage: "authorize", Summary: "This desktop is not ready to open.", Remediation: "Refresh its Desktop capability settings and try again."}
	}
	if s.Desktop == nil {
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_NOT_READY", Stage: "open", Summary: "Remote desktop is not ready on this computer.", Remediation: "Finish Desktop setup for this computer and try again."}
	}
	var result DesktopRunResult
	if routeAdapter == "" {
		result, err = s.Desktop.Run(ctx, target.Canonical)
	} else if routed, ok := s.Desktop.(RoutedDesktopRunner); ok {
		result, err = routed.RunVia(ctx, target.Canonical, routeAdapter)
	} else {
		return contracts.DesktopActionResponse{}, &Fault{Code: "ROUTE_NOT_SELECTABLE", Stage: "route", Summary: "This connection path cannot be selected for this desktop.", Remediation: "Choose Smart connect or another available path."}
	}
	if err != nil {
		var fault *Fault
		if errors.As(err, &fault) {
			return contracts.DesktopActionResponse{}, fault
		}
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_OPEN_FAILED", Stage: "open", Summary: "PF Remote could not open the remote desktop.", Remediation: "Check the visible computer status and try again."}
	}
	response := contracts.DesktopActionResponse{
		SchemaVersion: contracts.DesktopActionSchema, Action: "open", Status: "closed",
		Target: target, SessionID: result.SessionID, Protocol: result.Protocol,
		RenderingEnvironment: result.RenderingEnvironment, RouteAdapter: result.RouteAdapter,
	}
	if s.Activity != nil {
		s.Activity.Record(result.SessionID, target.Canonical, "open", "opened")
	}
	return response, nil
}

func (s Service) SaveDesktopCredentialAndOpen(ctx context.Context, input, credential string) (contracts.DesktopActionResponse, error) {
	return s.SaveDesktopCredentialAndOpenVia(ctx, input, credential, "")
}

func (s Service) SaveDesktopCredentialAndOpenVia(ctx context.Context, input, credential, routeAdapter string) (contracts.DesktopActionResponse, error) {
	inspected, err := s.Inspect(input)
	if err != nil || inspected.Target.Capability.Kind != contracts.CapabilityDesktop {
		return contracts.DesktopActionResponse{}, &Fault{Code: "TARGET_NOT_FOUND", Stage: "resolve", Summary: "The selected computer is unavailable.", Remediation: "Refresh the computer list and choose an available Desktop target."}
	}
	profile := inspected.Target.Capability.DesktopProfile
	if profile == nil || profile.Protocol != "vnc" || profile.Authentication != "legacy-vnc-password" {
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_CREDENTIAL_UNSUPPORTED", Stage: "authorize", Summary: "This desktop does not use a saved password.", Remediation: "Open it normally from PF Remote."}
	}
	if s.DesktopCredentials == nil || strings.TrimSpace(credential) == "" {
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_CREDENTIAL_REQUIRED", Stage: "setup", Summary: "Enter the desktop password to finish one-time setup.", Remediation: "Enter the existing password and try again."}
	}
	if err := s.DesktopCredentials.Save(inspected.Target.Canonical, credential); err != nil {
		return contracts.DesktopActionResponse{}, &Fault{Code: "DESKTOP_CREDENTIAL_SAVE_FAILED", Stage: "setup", Summary: "PF Remote could not save this desktop password.", Remediation: "Check the current Windows account and try again."}
	}
	return s.OpenVia(ctx, inspected.Target.Canonical, routeAdapter)
}
