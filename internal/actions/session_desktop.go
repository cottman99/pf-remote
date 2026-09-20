package actions

import (
	"context"
	"errors"

	"github.com/cottman99/pf-remote/internal/desktop"
)

type DesktopCoordinator interface {
	Run(context.Context, desktop.Request) (desktop.Result, error)
}

type SessionDesktop struct {
	Coordinator       DesktopCoordinator
	RouteCoordinators map[string]DesktopCoordinator
}

func (s SessionDesktop) Run(ctx context.Context, target string) (DesktopRunResult, error) {
	if s.Coordinator == nil {
		return DesktopRunResult{}, &Fault{Code: "DESKTOP_NOT_READY", Stage: "open", Summary: "Remote desktop is not ready on this computer.", Remediation: "Finish Desktop setup for this computer and try again."}
	}
	result, err := s.Coordinator.Run(ctx, desktop.Request{Target: target})
	if err != nil {
		var source *desktop.Fault
		if errors.As(err, &source) {
			return DesktopRunResult{}, &Fault{Code: source.Code, Stage: source.Stage, Summary: source.Summary, Remediation: source.Remediation}
		}
		return DesktopRunResult{}, err
	}
	return DesktopRunResult{SessionID: result.Session.ID, Protocol: result.Session.Protocol, RenderingEnvironment: result.Session.RenderingEnvironment, RouteAdapter: result.Session.RouteAdapter}, nil
}

func (s SessionDesktop) RunVia(ctx context.Context, target, adapter string) (DesktopRunResult, error) {
	coordinator := s.RouteCoordinators[adapter]
	if coordinator == nil {
		return DesktopRunResult{}, &Fault{Code: "ROUTE_NOT_SELECTABLE", Stage: "route", Summary: "This connection path is not available for this desktop.", Remediation: "Choose Smart connect or another available path."}
	}
	return (SessionDesktop{Coordinator: coordinator}).Run(ctx, target)
}

var _ DesktopRunner = SessionDesktop{}
var _ RoutedDesktopRunner = SessionDesktop{}
