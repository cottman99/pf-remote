package migration

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
	opensshexecutor "github.com/cottman99/pf-remote/internal/executor/openssh"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
)

var legacyRemoteUserPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// LegacyShellConnections binds one private legacy account to one exact Shell
// target. Accounts and routes never enter outward JSON or Agent context.
type LegacyShellConnections struct {
	users map[string]string
}

func (s *LegacyShellConnections) bind(target string, connection LegacyCenterConnection) bool {
	if connection.Protocol != "ssh" || !legacyRemoteUserPattern.MatchString(connection.Username) {
		return false
	}
	if s.users == nil {
		s.users = make(map[string]string)
	}
	s.users[target] = connection.Username
	return true
}

func (s LegacyShellConnections) HasTarget(target string) bool {
	_, exists := s.users[target]
	return exists
}

func (s LegacyShellConnections) rekeyTargets(targets map[string]string) LegacyShellConnections {
	result := LegacyShellConnections{users: make(map[string]string, len(s.users))}
	for target, user := range s.users {
		if replacement, exists := targets[target]; exists {
			target = replacement
		}
		result.users[target] = user
	}
	return result
}

func (s LegacyShellConnections) Runner(subjectDeviceID string, resolver session.Resolver, routes LegacyEndpointRoutes, identityFile string) actions.ShellRunner {
	return legacyShellRunner{subjectDeviceID: subjectDeviceID, resolver: resolver, routes: routes, users: s.users, identityFile: identityFile}
}

type legacyShellRunner struct {
	subjectDeviceID string
	resolver        session.Resolver
	routes          LegacyEndpointRoutes
	users           map[string]string
	identityFile    string
}

func (r legacyShellRunner) Run(ctx context.Context, target string, command []string) (actions.ShellRunResult, error) {
	user, exists := r.users[target]
	if !exists {
		return actions.ShellRunResult{}, &actions.Fault{Code: "SHELL_NOT_READY", Stage: "session", Summary: "Remote work is not configured for this computer.", Remediation: "Finish Shell setup for this computer and try again."}
	}
	if r.resolver == nil || r.subjectDeviceID == "" {
		return actions.ShellRunResult{}, errors.New("legacy Shell coordinator is unavailable")
	}
	routeProvider := route.Selector{Providers: []route.NamedProvider{
		{Adapter: "tailscale", Provider: route.ReachableProvider{Provider: r.routes.Provider("tailscale"), Timeout: 2 * time.Second}},
		{Adapter: "lan", Provider: route.ReachableProvider{Provider: r.routes.Provider("lan"), Timeout: 750 * time.Millisecond}},
	}}
	runner := actions.SessionShell{RemoteUser: user, IdentityFile: r.identityFile, Coordinator: session.Coordinator{
		SubjectDeviceID: r.subjectDeviceID,
		Resolver:        r.resolver,
		RouteProvider:   routeProvider,
		Executor:        opensshexecutor.Executor{},
	}}
	return runner.Run(ctx, target, command)
}

var _ actions.ShellRunner = legacyShellRunner{}
