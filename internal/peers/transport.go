package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/executor/openssh"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

// Endpoint is private local configuration, never replicated in the journal.
// It reuses OpenSSH and the Device-signed host binding, not SSH TOFU.
type Endpoint struct {
	Target       string          `json:"target"`
	Route        route.Candidate `json:"route"`
	User         string          `json:"user"`
	IdentityFile string          `json:"identity_file,omitempty"`
	Binary       string          `json:"binary"`
}
type TransportConfig struct {
	Schema string     `json:"schema"`
	Peers  []Endpoint `json:"peers"`
}

var safeBinary = regexp.MustCompile(`^[A-Za-z0-9_./:-]+$`)
var safeUser = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func TransportPath(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "peer-transports-v1.json")
}
func LoadTransports(path string) (TransportConfig, error) {
	var cfg TransportConfig
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return cfg, nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return cfg, ErrInvalid
	}
	b, e := os.ReadFile(path)
	if e != nil || Decode(b, &cfg) != nil || cfg.Schema != "pfremote.peer-transports/v1" || len(cfg.Peers) > 32 {
		return cfg, ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range cfg.Peers {
		if _, e := targetref.Parse(p.Target); e != nil || seen[p.Target] || p.Route.Validate() != nil || !safeUser.MatchString(p.User) || !safeBinary.MatchString(p.Binary) || p.Binary[0] == '-' {
			return TransportConfig{}, ErrInvalid
		}
		seen[p.Target] = true
		if p.IdentityFile != "" {
			info, e := os.Lstat(p.IdentityFile)
			if !filepath.IsAbs(p.IdentityFile) || e != nil || !info.Mode().IsRegular() {
				return TransportConfig{}, ErrInvalid
			}
		}
	}
	return cfg, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("peer response exceeded size limit")
	}
	return b.Buffer.Write(p)
}

// Sync uses membership for the fixed peer-exchange command. It does not confer
// arbitrary Shell authority and works even when application grants have expired.
// Both sides still require ordinary OS SSH authorization to their daemon user.
func (s Service) Sync(ctx context.Context, endpoint Endpoint, executor session.Executor) (result error) {
	defer func() { s.recordSync(endpoint.Target, result == nil) }()
	raw, _ := json.Marshal(RequestData{Operation: "export"})
	value, e := s.Handle(ctx, raw)
	if e != nil {
		return e
	}
	j, ok := value.(Journal)
	if !ok {
		return ErrInvalid
	}
	snap, e := j.Snapshot(s.clock())
	if e != nil {
		return e
	}
	ref, e := targetref.Parse(endpoint.Target)
	if e != nil || ref.FabricID != j.Fabric {
		return ErrInvalid
	}
	var d contracts.Device
	var c contracts.Capability
	localOK := false
	for _, v := range snap.Devices {
		if v.ID == ref.DeviceID {
			d = v
		}
		if v.ID == s.Signer.DeviceID() && v.State != "revoked" {
			localOK = true
		}
	}
	for _, v := range snap.Capabilities {
		if v.ID == ref.CapabilityID {
			c = v
		}
	}
	if !localOK || d.State == "revoked" || c.DeviceID != d.ID || c.Kind != contracts.CapabilityShell || shellbinding.Verify(d, c, j.Fabric) != nil {
		return ErrInvalid
	}
	if !safeBinary.MatchString(endpoint.Binary) || endpoint.Binary[0] == '-' || endpoint.Route.Validate() != nil {
		return ErrInvalid
	}
	request, _ := json.Marshal(RequestData{Operation: "exchange", Journal: &j})
	if len(request) > 900000 {
		return errors.New("peer journal needs a bounded checkpoint before synchronization")
	}
	output := &limitedBuffer{limit: 900000}
	if executor == nil {
		executor = openssh.Executor{}
	}
	code, e := executor.Execute(ctx, session.Execution{Session: session.Session{CanonicalTarget: endpoint.Target}, Route: endpoint.Route, Binding: *c.SSHBinding, RemoteUser: endpoint.User, IdentityFile: endpoint.IdentityFile, Command: []string{endpoint.Binary, "doctor", "--peer-stdin", "--json"}, Stdin: bytes.NewReader(request), Stdout: output, Stderr: io.Discard})
	if e != nil || code != 0 {
		return errors.New("peer exchange unavailable")
	}
	var incoming Journal
	if Decode(output.Bytes(), &incoming) != nil {
		return ErrInvalid
	}
	raw, _ = json.Marshal(RequestData{Operation: "import", Journal: &incoming})
	_, e = s.Handle(ctx, raw)
	return e
}

type fixedRoute struct{ candidate route.Candidate }
type fixedAcquisition struct{ candidate route.Candidate }

func (p fixedRoute) Acquire(context.Context, route.Request) (route.Acquisition, error) {
	return fixedAcquisition{p.candidate}, nil
}
func (p fixedAcquisition) Candidate() route.Candidate { return p.candidate }
func (p fixedAcquisition) Close() error               { return nil }

// ShellRunner adds exact new peer endpoints without changing legacy routes.
type ShellRunner struct {
	Config   TransportConfig
	Catalog  catalog.Catalog
	Subject  string
	Fallback actions.ShellRunner
}

func (s ShellRunner) Run(ctx context.Context, target string, command []string) (actions.ShellRunResult, error) {
	for _, p := range s.Config.Peers {
		if p.Target == target {
			runner := actions.SessionShell{RemoteUser: p.User, IdentityFile: p.IdentityFile, Coordinator: session.Coordinator{SubjectDeviceID: s.Subject, Resolver: s.Catalog, RouteProvider: fixedRoute{p.Route}, Executor: openssh.Executor{}}}
			return runner.Run(ctx, target, command)
		}
	}
	if s.Fallback != nil {
		return s.Fallback.Run(ctx, target, command)
	}
	return actions.ShellRunResult{}, errors.New("peer Shell transport is not configured")
}

// RunSynchronization never changes host network settings or creates a listener.
func (s Service) RunSynchronization(ctx context.Context, configPath string) {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			cfg, e := LoadTransports(configPath)
			if e == nil && len(cfg.Peers) > 0 {
				renew, _ := json.Marshal(RequestData{Operation: "renew"})
				_, _ = s.Handle(ctx, renew)
				for _, p := range cfg.Peers {
					bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
					_ = s.Sync(bounded, p, nil)
					cancel()
					if ctx.Err() != nil {
						return
					}
				}
			}
			timer.Reset(time.Minute)
		}
	}
}
