// Package desktop coordinates one authorized, route-pinned Desktop session.
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

type Resolver interface {
	ResolveAuthorized(subjectDeviceID, target string) (contracts.Target, error)
}

type Executor interface {
	Open(context.Context, Execution) error
}

type AuthorizationWatcher interface {
	Wait(context.Context, Session) error
}

type Request struct {
	Target string
}

type Session struct {
	ID                   string
	SubjectDeviceID      string
	CanonicalTarget      string
	TargetDeviceID       string
	CapabilityID         string
	AuthorizationExpiry  time.Time
	RouteID              string
	RouteAdapter         string
	Protocol             string
	RenderingEnvironment string
	Authentication       string
	VisualEffectsPolicy  string
}

type Execution struct {
	Session Session
	Route   route.Candidate
}

type Result struct {
	Session Session
}

type Fault struct {
	Code        string
	Stage       string
	Summary     string
	Remediation string
}

func (f *Fault) Error() string { return f.Summary }

// ExecutorFault lets an existing protocol executor return a stable, safe
// user-facing setup failure without exposing protocol or credential details.
type ExecutorFault struct {
	Code        string
	Summary     string
	Remediation string
}

func (f *ExecutorFault) Error() string { return f.Summary }

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
	if c.Resolver == nil || c.RouteProvider == nil || c.Executor == nil || strings.TrimSpace(c.SubjectDeviceID) == "" {
		return Result{}, fault("DESKTOP_NOT_CONFIGURED", "desktop", "Remote desktop is not configured.", "Finish Desktop setup and try again.")
	}
	target, err := c.Resolver.ResolveAuthorized(c.SubjectDeviceID, request.Target)
	if err != nil {
		return Result{}, fault("TARGET_NOT_FOUND", "resolve", "The authorized Desktop target could not be resolved.", "Refresh the computer list and try again.")
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	if !target.Granted || target.Authorization.Status != "active" || !now.Before(target.Authorization.ValidUntil) {
		return Result{}, fault("AUTHORIZATION_EXPIRED", "authorize", "The Desktop authorization is not active.", "Refresh authorization before opening the desktop.")
	}
	if strings.EqualFold(target.Device.State, "revoked") || target.Capability.Kind != contracts.CapabilityDesktop || target.Capability.State != "available" {
		return Result{}, fault("DESKTOP_UNAVAILABLE", "authorize", "The selected Desktop is unavailable.", "Choose an available authorized Desktop capability.")
	}
	profile := target.Capability.DesktopProfile
	if profile == nil || !validProfile(*profile) {
		return Result{}, fault("DESKTOP_PROFILE_INVALID", "target-auth", "The Desktop capability profile is invalid.", "Refresh the authenticated catalog before retrying.")
	}
	reference, parseErr := targetref.Parse(target.Canonical)
	if parseErr != nil || reference.DeviceID != target.Device.ID || reference.CapabilityID != target.Capability.ID {
		return Result{}, fault("TARGET_IDENTITY_INVALID", "target-auth", "The resolved Desktop target identity is inconsistent.", "Refresh the authenticated catalog before retrying.")
	}
	acquired, err := c.RouteProvider.Acquire(ctx, route.Request{SubjectDeviceID: c.SubjectDeviceID, CanonicalTarget: target.Canonical, AuthorizationExpiry: target.Authorization.ValidUntil.UTC()})
	if err != nil || acquired == nil {
		return Result{}, fault("ROUTE_UNAVAILABLE", "route", "A route to the Desktop could not be acquired.", "Check the computer connection and try again.")
	}
	defer acquired.Close()
	candidate := acquired.Candidate()
	if err := candidate.Validate(); err != nil {
		return Result{}, fault("INVALID_ROUTE", "route", "The selected Desktop route is invalid.", "Refresh route status and try again.")
	}
	sessionID, err := c.newID()
	if err != nil {
		return Result{}, fault("SESSION_ID_UNAVAILABLE", "desktop", "A fresh Desktop session could not be created.", "Try opening the desktop again.")
	}
	session := Session{
		ID: sessionID, SubjectDeviceID: c.SubjectDeviceID, CanonicalTarget: target.Canonical,
		TargetDeviceID: target.Device.ID, CapabilityID: target.Capability.ID,
		AuthorizationExpiry: target.Authorization.ValidUntil.UTC(), RouteID: candidate.ID, RouteAdapter: candidate.Adapter,
		Protocol: profile.Protocol, RenderingEnvironment: profile.RenderingEnvironment,
		Authentication: profile.Authentication, VisualEffectsPolicy: effectiveVisualEffectsPolicy(*profile),
	}
	deadlineContext, cancel := context.WithDeadline(ctx, session.AuthorizationExpiry)
	defer cancel()
	executionContext, cancelExecution := context.WithCancelCause(deadlineContext)
	defer cancelExecution(nil)
	watcher := c.Watcher
	if watcher == nil {
		interval := c.AuthorizationPollInterval
		if interval <= 0 {
			interval = time.Second
		}
		watcher = pollingAuthorizationWatcher{resolver: c.Resolver, interval: interval}
	}
	go func() {
		if watchErr := watcher.Wait(executionContext, session); watchErr != nil {
			cancelExecution(watchErr)
		}
	}()
	if err := c.Executor.Open(executionContext, Execution{Session: session, Route: candidate}); err != nil {
		cause := context.Cause(executionContext)
		if errors.Is(cause, context.DeadlineExceeded) {
			return Result{Session: session}, fault("AUTHORIZATION_EXPIRED", "authorize", "The Desktop authorization expired.", "Refresh authorization before opening it again.")
		}
		if errors.Is(cause, errAuthorizationChanged) {
			return Result{Session: session}, fault("AUTHORIZATION_REVOKED", "authorize", "The Desktop authorization changed during the session.", "Refresh authorization before opening it again.")
		}
		if errors.Is(cause, context.Canceled) {
			return Result{Session: session}, fault("SESSION_CANCELED", "desktop", "The Desktop session was canceled.", "Open a new Desktop session if it is still needed.")
		}
		var executorFault *ExecutorFault
		if errors.As(err, &executorFault) {
			return Result{Session: session}, fault(executorFault.Code, "executor", executorFault.Summary, executorFault.Remediation)
		}
		return Result{Session: session}, fault("DESKTOP_CLIENT_FAILED", "executor", "The desktop application could not complete the session.", "Install or repair the required desktop application and try again.")
	}
	return Result{Session: session}, nil
}

func validProfile(profile contracts.DesktopProfile) bool {
	protocolAuthentication := profile.Protocol == "rdp" && (profile.Authentication == "windows-sso" || profile.Authentication == "tailscale-device") ||
		profile.Protocol == "vnc" && (profile.Authentication == "x509-route-grant" || profile.Authentication == "tailscale-device" || profile.Authentication == "legacy-vnc-password")
	return protocolAuthentication && effectiveVisualEffectsPolicy(profile) != ""
}

func effectiveVisualEffectsPolicy(profile contracts.DesktopProfile) string {
	policy := strings.TrimSpace(profile.VisualEffectsPolicy)
	if policy == "" {
		if profile.RenderingEnvironment == "virtual" {
			return "automatic"
		}
		if profile.RenderingEnvironment == "physical" {
			return "system"
		}
		return ""
	}
	if profile.RenderingEnvironment == "physical" && policy == "system" {
		return policy
	}
	if profile.RenderingEnvironment == "virtual" && (policy == "automatic" || policy == "reduced" || policy == "full") {
		return policy
	}
	return ""
}

var errAuthorizationChanged = errors.New("desktop authorization changed")

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
				strings.EqualFold(target.Device.State, "revoked") || target.Capability.Kind != contracts.CapabilityDesktop ||
				target.Capability.State != "available" || target.Canonical != pinned.CanonicalTarget ||
				target.Device.ID != pinned.TargetDeviceID || target.Capability.ID != pinned.CapabilityID ||
				target.Capability.DesktopProfile == nil || !validProfile(*target.Capability.DesktopProfile) ||
				target.Capability.DesktopProfile.Protocol != pinned.Protocol ||
				target.Capability.DesktopProfile.RenderingEnvironment != pinned.RenderingEnvironment ||
				target.Capability.DesktopProfile.Authentication != pinned.Authentication ||
				effectiveVisualEffectsPolicy(*target.Capability.DesktopProfile) != pinned.VisualEffectsPolicy ||
				target.Authorization.ValidUntil.Before(pinned.AuthorizationExpiry) {
				return errAuthorizationChanged
			}
		}
	}
}

func fault(code, stage, summary, remediation string) *Fault {
	return &Fault{Code: code, Stage: stage, Summary: summary, Remediation: remediation}
}

func (c Coordinator) newID() (string, error) {
	if c.NewID != nil {
		return c.NewID()
	}
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "desktop-" + hex.EncodeToString(value), nil
}
