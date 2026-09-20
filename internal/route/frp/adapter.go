// Package frp adapts a short-lived Gateway route lease to a loopback STCP
// visitor owned by one Shell Session.
package frp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/routelease"
)

const LeaseSchema = routelease.SchemaVersion

type Lease = routelease.Lease

var opaquePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)

// Lease is secret-bearing, short-lived adapter input returned by the Gateway.
// It must never be copied into a TargetReference, event, or diagnostic export.
type LeaseClient interface {
	Acquire(context.Context, route.Request) (Lease, error)
	Release(context.Context, string) error
}

type Process interface {
	Stop() error
}

type Starter interface {
	Start(context.Context, string, string) (Process, error)
}

type Adapter struct {
	Binary     string
	StateDir   string
	AuthToken  string
	Client     LeaseClient
	Starter    Starter
	Now        func() time.Time
	NewPort    func() (uint16, error)
	Probe      func(context.Context, string) error
	ReadyAfter time.Duration
}

func (a Adapter) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	if a.Client == nil || strings.TrimSpace(a.Binary) == "" || strings.TrimSpace(a.StateDir) == "" {
		return nil, errors.New("frp adapter is not configured")
	}
	if request.SubjectDeviceID == "" || request.CanonicalTarget == "" || request.AuthorizationExpiry.IsZero() {
		return nil, errors.New("frp route request is incomplete")
	}
	if a.AuthToken != "" && !opaquePattern.MatchString(a.AuthToken) {
		return nil, errors.New("frp adapter authentication is invalid")
	}
	lease, err := a.Client.Acquire(ctx, request)
	if err != nil {
		return nil, errors.New("gateway route lease unavailable")
	}
	releaseLease := true
	defer func() {
		if releaseLease {
			releaseContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = a.Client.Release(releaseContext, lease.ID)
		}
	}()
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	if err := validateLease(lease, request, now); err != nil {
		return nil, err
	}
	port, err := a.newPort()
	if err != nil || port == 0 {
		return nil, errors.New("frp loopback endpoint unavailable")
	}
	stateDir := filepath.Clean(a.StateDir)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, errors.New("frp session state unavailable")
	}
	configFile, err := os.CreateTemp(stateDir, ".frpc-visitor-*.toml")
	if err != nil {
		return nil, errors.New("frp session configuration unavailable")
	}
	configPath := configFile.Name()
	removeConfig := true
	defer func() {
		if removeConfig {
			_ = os.Remove(configPath)
		}
	}()
	if err := configFile.Chmod(0o600); err != nil {
		_ = configFile.Close()
		return nil, errors.New("frp session configuration protection failed")
	}
	content := visitorConfig(lease, a.AuthToken, port)
	if _, err := configFile.Write(content); err != nil {
		clear(content)
		_ = configFile.Close()
		return nil, errors.New("frp session configuration unavailable")
	}
	clear(content)
	if err := configFile.Sync(); err != nil {
		_ = configFile.Close()
		return nil, errors.New("frp session configuration unavailable")
	}
	if err := configFile.Close(); err != nil {
		return nil, errors.New("frp session configuration unavailable")
	}
	starter := a.Starter
	if starter == nil {
		starter = commandStarter{}
	}
	processContext, cancelProcess := context.WithCancel(ctx)
	process, err := starter.Start(processContext, a.Binary, configPath)
	if err != nil {
		cancelProcess()
		return nil, errors.New("frp visitor could not start")
	}
	result := &acquisition{
		candidate: route.Candidate{ID: lease.ID, Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: port},
		client:    a.Client, leaseID: lease.ID, process: process, cancel: cancelProcess, configPath: configPath,
	}
	// Ownership moves before readiness probing so a failed probe releases the
	// lease through the acquisition exactly once.
	releaseLease = false
	if err := a.waitReady(ctx, result.candidate); err != nil {
		_ = result.Close()
		return nil, errors.New("frp visitor did not become ready")
	}
	removeConfig = false
	return result, nil
}

func validateLease(lease Lease, request route.Request, now time.Time) error {
	server := route.Candidate{ID: "lease-endpoint", Adapter: "frp", Network: "tcp", Address: lease.ServerAddress, Port: lease.ServerPort}
	if lease.SchemaVersion != LeaseSchema || lease.Adapter != "frp" || !opaquePattern.MatchString(lease.ID) ||
		lease.CanonicalTarget != request.CanonicalTarget || !now.Before(lease.ExpiresAt) ||
		lease.ExpiresAt.After(request.AuthorizationExpiry) || server.Validate() != nil ||
		!opaquePattern.MatchString(lease.ServerName) || (lease.ServerUser != "" && !opaquePattern.MatchString(lease.ServerUser)) ||
		!opaquePattern.MatchString(lease.SecretKey) {
		return errors.New("gateway returned an invalid frp route lease")
	}
	return nil
}

func (a Adapter) newPort() (uint16, error) {
	if a.NewPort != nil {
		return a.NewPort()
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return uint16(listener.Addr().(*net.TCPAddr).Port), nil
}

func (a Adapter) waitReady(ctx context.Context, candidate route.Candidate) error {
	deadline := a.ReadyAfter
	if deadline <= 0 {
		deadline = 5 * time.Second
	}
	readyContext, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	address := net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port)))
	probe := a.Probe
	if probe == nil {
		probe = func(ctx context.Context, address string) error {
			dialer := net.Dialer{Timeout: 100 * time.Millisecond}
			connection, err := dialer.DialContext(ctx, "tcp", address)
			if err == nil {
				_ = connection.Close()
			}
			return err
		}
	}
	for {
		if err := probe(readyContext, address); err == nil {
			return nil
		}
		select {
		case <-readyContext.Done():
			return readyContext.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func visitorConfig(lease Lease, authToken string, port uint16) []byte {
	var builder strings.Builder
	fmt.Fprintf(&builder, "serverAddr = %s\nserverPort = %d\nloginFailExit = true\n", strconv.Quote(lease.ServerAddress), lease.ServerPort)
	if authToken != "" {
		fmt.Fprintf(&builder, "auth.method = \"token\"\nauth.token = %s\n", strconv.Quote(authToken))
	}
	builder.WriteString("transport.tls.enable = true\nlog.to = \"console\"\nlog.level = \"error\"\n\n[[visitors]]\n")
	fmt.Fprintf(&builder, "name = %s\ntype = \"stcp\"\nserverName = %s\n", strconv.Quote(lease.ID), strconv.Quote(lease.ServerName))
	if lease.ServerUser != "" {
		fmt.Fprintf(&builder, "serverUser = %s\n", strconv.Quote(lease.ServerUser))
	}
	fmt.Fprintf(&builder, "secretKey = %s\nbindAddr = \"127.0.0.1\"\nbindPort = %d\ntransport.useEncryption = true\n", strconv.Quote(lease.SecretKey), port)
	return []byte(builder.String())
}

type acquisition struct {
	once       sync.Once
	candidate  route.Candidate
	client     LeaseClient
	leaseID    string
	process    Process
	cancel     context.CancelFunc
	configPath string
	err        error
}

func (a *acquisition) Candidate() route.Candidate { return a.candidate }

func (a *acquisition) Close() error {
	a.once.Do(func() {
		a.cancel()
		var failures []error
		if a.process != nil {
			if err := a.process.Stop(); err != nil {
				failures = append(failures, errors.New("stop frp visitor"))
			}
		}
		if err := os.Remove(a.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, errors.New("remove frp session configuration"))
		}
		releaseContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := a.client.Release(releaseContext, a.leaseID); err != nil {
			failures = append(failures, errors.New("release gateway route lease"))
		}
		a.err = errors.Join(failures...)
	})
	return a.err
}

type commandStarter struct{}

func (commandStarter) Start(ctx context.Context, binary, configPath string) (Process, error) {
	command := exec.CommandContext(ctx, binary, "-c", configPath)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &commandProcess{command: command}, nil
}

type commandProcess struct {
	once    sync.Once
	command *exec.Cmd
	err     error
}

func (p *commandProcess) Stop() error {
	p.once.Do(func() {
		if p.command.Process != nil {
			_ = p.command.Process.Kill()
		}
		p.err = p.command.Wait()
		if _, ok := p.err.(*exec.ExitError); ok {
			p.err = nil
		}
	})
	return p.err
}

var _ route.Provider = Adapter{}
