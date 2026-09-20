// Package session coordinates one authorized, route-pinned Shell attempt.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

var (
	ErrAuthorizationExpired = errors.New("session authorization expired")
	ErrAuthorizationRevoked = errors.New("session authorization was revoked")
)

type Resolver interface {
	ResolveAuthorized(subjectDeviceID, target string) (contracts.Target, error)
}

type Executor interface {
	Execute(context.Context, Execution) (int, error)
}

// AuthorizationWatcher reports an observed revocation or binding invalidation.
// It returns nil when the supplied context ends normally.
type AuthorizationWatcher interface {
	Wait(context.Context, Session) error
}

type Request struct {
	Target       string
	Route        route.Candidate
	RemoteUser   string
	IdentityFile string
	Command      []string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
}

type Session struct {
	ID                  string
	SubjectDeviceID     string
	CanonicalTarget     string
	FabricID            string
	TargetDeviceID      string
	CapabilityID        string
	AuthorizationExpiry time.Time
	RouteID             string
	RouteAdapter        string
	RouteDiagnostics    []route.Diagnostic
	Executor            string
	BindingVersion      uint64
	BindingSignature    string
	HostKeyFingerprints []string
	StartedAt           time.Time
}

type Execution struct {
	Session      Session
	Route        route.Candidate
	Binding      contracts.SSHCapabilityBinding
	RemoteUser   string
	IdentityFile string
	Command      []string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
}

type Result struct {
	Session  Session
	ExitCode int
}

// Fault is the secret-free internal error boundary later local APIs can map to
// the public pfremote.error/v1 envelope without parsing process errors.
type Fault struct {
	Code        string
	Stage       string
	Summary     string
	Remediation string
	cause       error
}

func (f *Fault) Error() string { return f.Summary }
func (f *Fault) Unwrap() error { return f.cause }

type Coordinator struct {
	SubjectDeviceID           string
	Resolver                  Resolver
	RouteProvider             route.Provider
	Executor                  Executor
	Watcher                   AuthorizationWatcher
	AuthorizationPollInterval time.Duration
	Now                       func() time.Time
	NewID                     func() (string, error)
}

func (c Coordinator) Run(ctx context.Context, request Request) (Result, error) {
	if c.Resolver == nil || c.Executor == nil {
		return Result{}, fault("SESSION_NOT_CONFIGURED", "session", "The Shell session coordinator is not configured.", "Configure an authorized resolver and OpenSSH executor.", nil)
	}
	if strings.TrimSpace(c.SubjectDeviceID) == "" || strings.TrimSpace(request.Target) == "" {
		return Result{}, fault("INVALID_SESSION_REQUEST", "session", "The Shell session subject and target are required.", "Provide the local subject Device and an immutable target or alias.", nil)
	}
	if c.RouteProvider != nil && request.Route != (route.Candidate{}) {
		return Result{}, fault("INVALID_ROUTE", "route", "The Shell request contains conflicting route sources.", "Let the configured route provider select the Gateway route.", nil)
	}
	if c.RouteProvider == nil {
		if routeErr := request.Route.Validate(); routeErr != nil {
			return Result{}, fault("INVALID_ROUTE", "route", "The selected Shell route is invalid.", "Select a valid route candidate and retry as a new Session.", routeErr)
		}
	}

	target, err := c.Resolver.ResolveAuthorized(c.SubjectDeviceID, request.Target)
	if err != nil {
		return Result{}, fault("TARGET_NOT_FOUND", "resolve", "The authorized Shell target could not be resolved.", "Refresh authorized targets and retry.", nil)
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	if !target.Granted || target.Authorization.Status != "active" || !now.Before(target.Authorization.ValidUntil) {
		return Result{}, fault("AUTHORIZATION_EXPIRED", "authorize", "The Shell authorization is not active.", "Refresh authorization before starting a new Session.", ErrAuthorizationExpired)
	}
	if target.Capability.Kind != contracts.CapabilityShell || target.Capability.State != "available" {
		return Result{}, fault("SHELL_UNAVAILABLE", "authorize", "The target is not an available Shell Capability.", "Select an available authorized Shell Capability.", nil)
	}
	if strings.EqualFold(target.Device.State, "revoked") {
		return Result{}, fault("DEVICE_REVOKED", "authorize", "A Device in the Shell authorization boundary is revoked.", "Activate a valid Device identity before retrying.", ErrAuthorizationRevoked)
	}
	reference, err := targetref.Parse(target.Canonical)
	if err != nil || reference.DeviceID != target.Device.ID || reference.CapabilityID != target.Capability.ID {
		return Result{}, fault("TARGET_IDENTITY_INVALID", "target-auth", "The resolved Shell target identity is inconsistent.", "Refresh the authenticated catalog before retrying.", nil)
	}
	if err := shellbinding.Verify(target.Device, target.Capability, reference.FabricID); err != nil {
		return Result{}, fault("TARGET_IDENTITY_INVALID", "target-auth", "The Shell target identity binding is invalid.", "Refresh the Device-signed SSH binding before retrying.", nil)
	}

	binding := cloneBinding(*target.Capability.SSHBinding)
	fingerprints := make([]string, 0, len(binding.HostKeys))
	for _, key := range binding.HostKeys {
		fingerprint, err := shellbinding.Fingerprint(key)
		if err != nil {
			return Result{}, fault("TARGET_IDENTITY_INVALID", "target-auth", "The Shell target identity binding is invalid.", "Refresh the Device-signed SSH binding before retrying.", nil)
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	selectedRoute := request.Route
	var acquired route.Acquisition
	if c.RouteProvider != nil {
		acquired, err = c.RouteProvider.Acquire(ctx, route.Request{
			SubjectDeviceID: c.SubjectDeviceID, CanonicalTarget: target.Canonical,
			AuthorizationExpiry: target.Authorization.ValidUntil.UTC(),
		})
		if err != nil || acquired == nil {
			return Result{}, fault("ROUTE_UNAVAILABLE", "route", "The Gateway FRP route could not be acquired.", "Check Gateway and FRP availability, then retry as a new Session.", nil)
		}
		defer acquired.Close()
		selectedRoute = acquired.Candidate()
		if err := selectedRoute.Validate(); err != nil {
			return Result{}, fault("INVALID_ROUTE", "route", "The Gateway returned an invalid Shell route.", "Inspect redacted Gateway route diagnostics before retrying.", nil)
		}
	}
	var routeDiagnostics []route.Diagnostic
	if diagnostic, ok := acquired.(interface{ Diagnostics() []route.Diagnostic }); ok {
		routeDiagnostics = diagnostic.Diagnostics()
	}
	sessionID, err := c.newID()
	if err != nil || strings.TrimSpace(sessionID) == "" || len(sessionID) > 128 {
		return Result{}, fault("SESSION_ID_UNAVAILABLE", "session", "A fresh Shell Session ID could not be created.", "Retry after restoring operating-system entropy.", nil)
	}
	current := Session{
		ID: sessionID, SubjectDeviceID: c.SubjectDeviceID,
		CanonicalTarget: target.Canonical, FabricID: binding.FabricID,
		TargetDeviceID: target.Device.ID, CapabilityID: target.Capability.ID,
		AuthorizationExpiry: target.Authorization.ValidUntil.UTC(),
		RouteID:             selectedRoute.ID, RouteAdapter: selectedRoute.Adapter,
		RouteDiagnostics: append([]route.Diagnostic(nil), routeDiagnostics...),
		Executor:         "openssh", BindingVersion: binding.BindingVersion, BindingSignature: binding.Signature,
		HostKeyFingerprints: append([]string(nil), fingerprints...),
		StartedAt:           now,
	}

	deadlineContext, cancelDeadline := context.WithDeadlineCause(ctx, current.AuthorizationExpiry, ErrAuthorizationExpired)
	defer cancelDeadline()
	executionContext, cancelCause := context.WithCancelCause(deadlineContext)
	defer cancelCause(nil)
	watcher := c.Watcher
	if watcher == nil {
		interval := c.AuthorizationPollInterval
		if interval <= 0 {
			interval = time.Second
		}
		watcher = pollingAuthorizationWatcher{resolver: c.Resolver, interval: interval}
	}
	if watcher != nil {
		go func() {
			if watchErr := watcher.Wait(executionContext, current); watchErr != nil {
				cancelCause(ErrAuthorizationRevoked)
			}
		}()
	}
	exitCode, executeErr := c.Executor.Execute(executionContext, Execution{
		Session: current, Route: selectedRoute, Binding: binding,
		RemoteUser: request.RemoteUser, IdentityFile: request.IdentityFile,
		Command: append([]string(nil), request.Command...),
		Stdin:   request.Stdin, Stdout: request.Stdout, Stderr: request.Stderr,
	})
	if cause := context.Cause(executionContext); cause != nil {
		return Result{Session: current, ExitCode: exitCode}, cancellationFault(cause)
	}
	if executeErr != nil {
		return Result{Session: current, ExitCode: exitCode}, fault("EXECUTOR_FAILED", "executor", "The OpenSSH executor could not complete the Shell Session.", "Check route, SSH authentication, and target availability before retrying.", nil)
	}
	return Result{Session: current, ExitCode: exitCode}, nil
}

type pollingAuthorizationWatcher struct {
	resolver Resolver
	interval time.Duration
}

func (w pollingAuthorizationWatcher) Wait(ctx context.Context, pinned Session) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			target, err := w.resolver.ResolveAuthorized(pinned.SubjectDeviceID, pinned.CanonicalTarget)
			if err != nil || !target.Granted || target.Authorization.Status != "active" ||
				strings.EqualFold(target.Device.State, "revoked") || target.Capability.Kind != contracts.CapabilityShell ||
				target.Capability.State != "available" || target.Canonical != pinned.CanonicalTarget {
				return ErrAuthorizationRevoked
			}
			reference, err := targetref.Parse(target.Canonical)
			if err != nil || reference.FabricID != pinned.FabricID || reference.DeviceID != pinned.TargetDeviceID ||
				reference.CapabilityID != pinned.CapabilityID || shellbinding.Verify(target.Device, target.Capability, reference.FabricID) != nil ||
				target.Capability.SSHBinding.BindingVersion != pinned.BindingVersion || target.Capability.SSHBinding.Signature != pinned.BindingSignature {
				return ErrAuthorizationRevoked
			}
		}
	}
}

func fault(code, stage, summary, remediation string, cause error) *Fault {
	return &Fault{Code: code, Stage: stage, Summary: summary, Remediation: remediation, cause: cause}
}

func cancellationFault(cause error) error {
	switch {
	case errors.Is(cause, ErrAuthorizationExpired):
		return fault("AUTHORIZATION_EXPIRED", "authorize", "The Shell authorization expired during the Session.", "Refresh authorization before starting a new Session.", ErrAuthorizationExpired)
	case errors.Is(cause, ErrAuthorizationRevoked):
		return fault("AUTHORIZATION_REVOKED", "authorize", "The Shell authorization was revoked during the Session.", "Refresh authorization and target identity before retrying.", ErrAuthorizationRevoked)
	case errors.Is(cause, context.DeadlineExceeded):
		return fault("SESSION_DEADLINE", "session", "The Shell Session deadline elapsed.", "Retry with an active authorization and sufficient deadline.", context.DeadlineExceeded)
	default:
		return fault("SESSION_CANCELED", "session", "The Shell Session was canceled.", "Start a new Session if Shell access is still required.", context.Canceled)
	}
}

func (c Coordinator) newID() (string, error) {
	if c.NewID != nil {
		return c.NewID()
	}
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "session-" + hex.EncodeToString(value), nil
}

func cloneBinding(value contracts.SSHCapabilityBinding) contracts.SSHCapabilityBinding {
	value.HostKeys = append([]contracts.SSHHostKey(nil), value.HostKeys...)
	return value
}
