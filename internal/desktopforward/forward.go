// Package desktopforward maps a private Desktop to the authorized Shell on the
// same Device. Native OpenSSH carries VNC end to end through the Gateway.
package desktopforward

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/executor/openssh"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

type Mapping struct {
	DesktopTarget string `json:"desktop_target"`
	ShellTarget   string `json:"shell_target"`
	Port          uint16 `json:"loopback_port"`
}

type Config struct {
	Targets []Mapping `json:"targets"`
	Schema  string    `json:"schema_version"`
}

// Load is optional for older deployments; a malformed mapping fails closed.
func Load(statePath string) (Config, error) {
	var config Config
	path := filepath.Join(filepath.Dir(statePath), "desktop-ssh-v1.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return config, errors.New("private Desktop SSH mapping is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return config, errors.New("private Desktop SSH mapping is unavailable")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(&struct{}{}) != io.EOF || config.Schema != "pfremote.desktop-ssh/v1" || len(config.Targets) == 0 || len(config.Targets) > 64 {
		return Config{}, errors.New("private Desktop SSH mapping is invalid")
	}
	seen := map[string]bool{}
	for _, entry := range config.Targets {
		d, de := targetref.Parse(entry.DesktopTarget)
		s, se := targetref.Parse(entry.ShellTarget)
		if de != nil || se != nil || d.FabricID != s.FabricID || d.DeviceID != s.DeviceID || d.CapabilityID == s.CapabilityID || entry.Port == 0 || seen[entry.DesktopTarget] {
			return Config{}, errors.New("private Desktop SSH mapping crosses an identity boundary")
		}
		seen[entry.DesktopTarget] = true
	}
	return config, nil
}

type Forwarder struct {
	Config          Config
	Resolver        session.Resolver
	SubjectDeviceID string
	Gateway         route.Provider
	RemoteUser      func(string) (string, bool)
	IdentityFile    string
	// Factory is an in-process test seam, never runtime configuration.
	Factory func(uint16, chan<- route.Candidate) session.Executor
}

func (c Config) HasTarget(target string) bool {
	for _, entry := range c.Targets {
		if entry.DesktopTarget == target {
			return true
		}
	}
	return false
}

// GatewayProvider reuses the existing same-Device Shell route without changing
// the legacy inventory from which stable Fabric and target identities derive.
func (c Config) GatewayProvider(provider route.Provider) route.Provider {
	return mappedGateway{config: c, provider: provider}
}

type mappedGateway struct {
	config   Config
	provider route.Provider
}

func (m mappedGateway) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	if m.provider == nil {
		return nil, errors.New("Desktop Gateway is unavailable")
	}
	for _, entry := range m.config.Targets {
		if entry.DesktopTarget == request.CanonicalTarget {
			request.CanonicalTarget = entry.ShellTarget
			break
		}
	}
	return m.provider.Acquire(ctx, request)
}

func (f Forwarder) Prepare(ctx context.Context, execution desktop.Execution) (route.Candidate, func(), error) {
	var selected *Mapping
	for i := range f.Config.Targets {
		if f.Config.Targets[i].DesktopTarget == execution.Session.CanonicalTarget {
			selected = &f.Config.Targets[i]
			break
		}
	}
	if selected == nil {
		return execution.Route, func() {}, nil
	}
	fail := func() (route.Candidate, func(), error) {
		return route.Candidate{}, nil, errors.New("encrypted Desktop route is unavailable")
	}
	if execution.Session.Protocol != "vnc" || execution.Route.Adapter != "frp" || f.Resolver == nil || f.Gateway == nil || f.RemoteUser == nil || execution.Session.SubjectDeviceID != f.SubjectDeviceID {
		return fail()
	}
	d, de := f.Resolver.ResolveAuthorized(f.SubjectDeviceID, selected.DesktopTarget)
	s, se := f.Resolver.ResolveAuthorized(f.SubjectDeviceID, selected.ShellTarget)
	now := time.Now()
	active := func(t contracts.Target) bool {
		return t.Granted && t.Authorization.Status == "active" && now.Before(t.Authorization.ValidUntil) && t.Device.State != "revoked"
	}
	if de != nil || se != nil || !active(d) || !active(s) || d.Canonical != selected.DesktopTarget || s.Canonical != selected.ShellTarget || d.Device.ID != s.Device.ID || d.Device.IdentityPublicKey != s.Device.IdentityPublicKey || d.Device.IdentityPublicKey != execution.Session.TargetIdentity || d.Capability.Kind != contracts.CapabilityDesktop || d.Capability.State != "available" || d.Capability.DesktopProfile == nil || d.Capability.DesktopProfile.Protocol != "vnc" || s.Capability.Kind != contracts.CapabilityShell {
		return fail()
	}
	user, ok := f.RemoteUser(selected.ShellTarget)
	if !ok {
		return fail()
	}
	// The viewer, rather than the short local API request, owns this lifetime.
	deadline := execution.Session.AuthorizationExpiry
	if d.Authorization.ValidUntil.Before(deadline) {
		deadline = d.Authorization.ValidUntil
	}
	forwardContext, cancel := context.WithDeadline(context.Background(), deadline)
	var once sync.Once
	cleanup := func() { once.Do(cancel) }
	ready := make(chan route.Candidate, 1)
	done := make(chan error, 1)
	var executor session.Executor = openssh.ForwardExecutor{Port: selected.Port, Ready: ready, Executor: openssh.Executor{}}
	if f.Factory != nil {
		executor = f.Factory(selected.Port, ready)
	}
	coordinator := session.Coordinator{SubjectDeviceID: f.SubjectDeviceID, Resolver: f.Resolver, RouteProvider: requiredGateway{f.Gateway}, Executor: executor}
	go func() {
		_, err := coordinator.Run(forwardContext, session.Request{Target: selected.ShellTarget, RemoteUser: user, IdentityFile: f.IdentityFile})
		done <- err
		cleanup()
	}()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-forwardContext.Done():
				return
			case <-ticker.C:
				current, err := f.Resolver.ResolveAuthorized(f.SubjectDeviceID, selected.DesktopTarget)
				if err != nil || !current.Granted || current.Authorization.Status != "active" || current.Canonical != d.Canonical || current.Device.IdentityPublicKey != d.Device.IdentityPublicKey || current.Device.State == "revoked" || current.Capability.State != "available" || current.Capability.DesktopProfile == nil || current.Capability.DesktopProfile.Protocol != "vnc" || current.Authorization.ValidUntil.Before(deadline) {
					cleanup()
					return
				}
			}
		}
	}()
	var candidate route.Candidate
	select {
	case candidate = <-ready:
	case <-done:
		cleanup()
		return fail()
	case <-ctx.Done():
		cleanup()
		return fail()
	case <-forwardContext.Done():
		cleanup()
		return fail()
	}
	// Authenticate SSH and check the real VNC backend without ClientInit, screen
	// capture, input, or changing an existing desktop connection.
	probeContext, stopProbe := context.WithTimeout(ctx, 15*time.Second)
	defer stopProbe()
	connection, err := (&net.Dialer{}).DialContext(probeContext, "tcp", net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port))))
	if err != nil {
		cleanup()
		return fail()
	}
	probeDeadline, _ := probeContext.Deadline()
	_ = connection.SetDeadline(probeDeadline)
	banner := make([]byte, 12)
	_, err = io.ReadFull(connection, banner)
	_ = connection.Close()
	if err != nil || !strings.HasPrefix(string(banner), "RFB 003.") || banner[11] != '\n' {
		cleanup()
		return fail()
	}
	select {
	case <-forwardContext.Done():
		cleanup()
		return fail()
	default:
	}
	return candidate, cleanup, nil
}

type requiredGateway struct{ route.Provider }

func (p requiredGateway) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	value, err := p.Provider.Acquire(ctx, request)
	if err != nil || value == nil {
		return nil, errors.New("Desktop Gateway is unavailable")
	}
	if value.Candidate().Adapter != "frp" {
		_ = value.Close()
		return nil, errors.New("Desktop Gateway is required")
	}
	return value, nil
}
