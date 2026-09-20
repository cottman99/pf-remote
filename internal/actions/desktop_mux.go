package actions

import "context"

// TargetedDesktopRunner handles an explicitly bound subset of canonical
// Desktop targets. It must never select a target by alias or display name.
type TargetedDesktopRunner interface {
	DesktopRunner
	HasTarget(string) bool
}

// DesktopMux keeps private compatibility executors separate from routed
// RDP/VNC sessions while preserving the shared resolve/authorize action core.
type DesktopMux struct {
	Override TargetedDesktopRunner
	Fallback DesktopRunner
}

func (m DesktopMux) Run(ctx context.Context, target string) (DesktopRunResult, error) {
	if m.Override != nil && m.Override.HasTarget(target) {
		return m.Override.Run(ctx, target)
	}
	if m.Fallback == nil {
		return DesktopRunResult{}, &Fault{Code: "DESKTOP_NOT_READY", Stage: "open", Summary: "Remote desktop is not ready on this computer.", Remediation: "Finish Desktop setup for this computer and try again."}
	}
	return m.Fallback.Run(ctx, target)
}

func (m DesktopMux) RunVia(ctx context.Context, target, adapter string) (DesktopRunResult, error) {
	if adapter == "legacy-external" && m.Override != nil && m.Override.HasTarget(target) {
		result, err := m.Override.Run(ctx, target)
		result.RouteAdapter = adapter
		return result, err
	}
	if routed, ok := m.Fallback.(RoutedDesktopRunner); ok {
		return routed.RunVia(ctx, target, adapter)
	}
	return DesktopRunResult{}, &Fault{Code: "ROUTE_NOT_SELECTABLE", Stage: "route", Summary: "This connection path is not available for this desktop.", Remediation: "Choose Smart connect or another available path."}
}

var _ DesktopRunner = DesktopMux{}
var _ RoutedDesktopRunner = DesktopMux{}
