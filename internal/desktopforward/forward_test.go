package desktopforward

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/executor/openssh"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type signer struct {
	private ed25519.PrivateKey
	id      string
}

func (s signer) DeviceID() string              { return s.id }
func (s signer) PublicKey() ed25519.PublicKey  { return s.private.Public().(ed25519.PublicKey) }
func (s signer) Sign(b []byte) ([]byte, error) { return ed25519.Sign(s.private, b), nil }

type targets struct {
	sync.Mutex
	values map[string]contracts.Target
}

func (t *targets) ResolveAuthorized(_, name string) (contracts.Target, error) {
	t.Lock()
	defer t.Unlock()
	v, ok := t.values[name]
	if !ok {
		return v, errors.New("missing")
	}
	return v, nil
}

type acquisition struct {
	value  route.Candidate
	closed *atomic.Int32
}

func (a acquisition) Candidate() route.Candidate { return a.value }
func (a acquisition) Close() error               { a.closed.Add(1); return nil }

type gateway struct {
	calls       atomic.Int32
	closed      atomic.Int32
	adapter     string
	unavailable bool
	request     route.Request
}

func (g *gateway) Acquire(_ context.Context, request route.Request) (route.Acquisition, error) {
	g.request = request
	g.calls.Add(1)
	if g.unavailable {
		return nil, errors.New("unavailable")
	}
	return acquisition{route.Candidate{ID: "fixture-relay", Adapter: g.adapter, Network: "tcp", Address: "127.0.0.1", Port: 2222}, &g.closed}, nil
}

func TestMappedGatewayRetainsExistingIdentityAndLegacyRoutes(t *testing.T) {
	f, e, _, g, _ := fixture(t)
	request := route.Request{SubjectDeviceID: e.Session.SubjectDeviceID, CanonicalTarget: e.Session.CanonicalTarget, AuthorizationExpiry: e.Session.AuthorizationExpiry}
	acquired, err := f.Config.GatewayProvider(g).Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = acquired.Close()
	if g.request.CanonicalTarget != f.Config.Targets[0].ShellTarget || g.request.SubjectDeviceID != request.SubjectDeviceID || !g.request.AuthorizationExpiry.Equal(request.AuthorizationExpiry) {
		t.Fatal("mapped route lost its target/authorization binding")
	}
	request.CanonicalTarget = f.Config.Targets[0].ShellTarget
	acquired, err = f.Config.GatewayProvider(g).Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = acquired.Close()
	if g.request.CanonicalTarget != request.CanonicalTarget {
		t.Fatal("legacy Shell route changed")
	}
	if !f.Config.HasTarget(e.Session.CanonicalTarget) || f.Config.HasTarget(request.CanonicalTarget) {
		t.Fatal("route availability included an unmapped target")
	}
}

// This substitutes only native process execution. The real Shell coordinator,
// signed binding, OpenSSH argument construction and loopback listener are used.
type vncRunner struct {
	t           *testing.T
	wrongBanner bool
	calls       atomic.Int32
}

func (r *vncRunner) Run(ctx context.Context, _ string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	r.calls.Add(1)
	joined := strings.Join(args, " ")
	for _, required := range []string{"StrictHostKeyChecking=yes", "ProxyCommand=none", "ForwardAgent=no", "-W 127.0.0.1:5900", "HostKeyAlias=pfremote-"} {
		if !strings.Contains(joined, required) {
			r.t.Errorf("missing SSH restriction %q", required)
		}
	}
	if r.wrongBanner {
		_, _ = io.WriteString(stdout, "NOT A VNC!\n!")
		return 0, nil
	}
	_, _ = io.WriteString(stdout, "RFB 003.008\n")
	_, _ = io.Copy(stdout, stdin)
	return 0, ctx.Err()
}

func fixture(t *testing.T) (Forwarder, desktop.Execution, *targets, *gateway, *vncRunner) {
	t.Helper()
	s := signer{private: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x52}, 32))}
	s.id, _ = identity.DeviceIDFromPublicKey(s.PublicKey())
	var host bytes.Buffer
	_ = binary.Write(&host, binary.BigEndian, uint32(len("ssh-ed25519")))
	host.WriteString("ssh-ed25519")
	_ = binary.Write(&host, binary.BigEndian, uint32(32))
	host.Write(bytes.Repeat([]byte{0x36}, 32))
	binding, err := shellbinding.Sign("fabric-fixture", "shell-fixture", 1, []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: base64.StdEncoding.EncodeToString(host.Bytes())}}, s)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "pfremote://fabric-fixture/devices/" + s.id + "/capabilities/"
	device := contracts.Device{ID: s.id, State: "online", IdentityPublicKey: base64.RawURLEncoding.EncodeToString(s.PublicKey())}
	authorization := contracts.Authorization{Status: "active", ValidUntil: time.Now().Add(time.Hour)}
	d := contracts.Target{Canonical: prefix + "desktop-fixture", Device: device, Granted: true, Authorization: authorization, Capability: contracts.Capability{ID: "desktop-fixture", DeviceID: s.id, Kind: contracts.CapabilityDesktop, State: "available", DesktopProfile: &contracts.DesktopProfile{Protocol: "vnc", RenderingEnvironment: "physical", Authentication: "legacy-vnc-password", VisualEffectsPolicy: "system"}}}
	sh := contracts.Target{Canonical: prefix + "shell-fixture", Device: device, Granted: true, Authorization: authorization, Capability: contracts.Capability{ID: "shell-fixture", DeviceID: s.id, Kind: contracts.CapabilityShell, State: "available", SSHBinding: binding}}
	resolver := &targets{values: map[string]contracts.Target{d.Canonical: d, sh.Canonical: sh}}
	g := &gateway{adapter: "frp"}
	runner := &vncRunner{t: t}
	stateDir := t.TempDir()
	f := Forwarder{Config: Config{Schema: "pfremote.desktop-ssh/v1", Targets: []Mapping{{DesktopTarget: d.Canonical, ShellTarget: sh.Canonical, Port: 5900}}}, Resolver: resolver, SubjectDeviceID: "device-controller", Gateway: g, RemoteUser: func(string) (string, bool) { return "fixture-user", true }, IdentityFile: filepath.Join(stateDir, "identity-fixture"), Factory: func(port uint16, ready chan<- route.Candidate) session.Executor {
		return openssh.ForwardExecutor{Port: port, Ready: ready, Executor: openssh.Executor{Binary: os.Args[0], StateDir: stateDir, Runner: runner}}
	}}
	execution := desktop.Execution{Session: desktop.Session{SubjectDeviceID: f.SubjectDeviceID, CanonicalTarget: d.Canonical, Protocol: "vnc", TargetIdentity: device.IdentityPublicKey, AuthorizationExpiry: authorization.ValidUntil}, Route: route.Candidate{ID: "fixture-desktop", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: 2222}}
	return f, execution, resolver, g, runner
}

func TestOptionalMappingAndSchemaBoundaries(t *testing.T) {
	f, _, _, _, _ := fixture(t)
	statePath := filepath.Join(t.TempDir(), "state.db")
	path := filepath.Join(filepath.Dir(statePath), "desktop-ssh-v1.json")
	if c, err := Load(statePath); err != nil || len(c.Targets) != 0 {
		t.Fatal("absent mapping changed legacy behavior")
	}
	write := func(v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(f.Config)
	if _, err := Load(statePath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"duplicate", "cross-device", "cross-fabric", "same-capability", "zero-port", "schema"} {
		t.Run(name, func(t *testing.T) {
			c := f.Config
			c.Targets = append([]Mapping{}, c.Targets...)
			switch name {
			case "duplicate":
				c.Targets = append(c.Targets, c.Targets[0])
			case "cross-device":
				c.Targets[0].ShellTarget = strings.Replace(c.Targets[0].ShellTarget, "/devices/", "/devices/another-", 1)
			case "cross-fabric":
				c.Targets[0].ShellTarget = strings.Replace(c.Targets[0].ShellTarget, "fabric-fixture", "fabric-other", 1)
			case "same-capability":
				c.Targets[0].ShellTarget = c.Targets[0].DesktopTarget
			case "zero-port":
				c.Targets[0].Port = 0
			case "schema":
				c.Schema = "unknown"
			}
			write(c)
			if _, err := Load(statePath); err == nil {
				t.Fatal("unsafe mapping accepted")
			}
		})
	}
}

func TestForwardPinsIdentitySurvivesRequestAndClosesWithViewer(t *testing.T) {
	f, e, _, g, runner := fixture(t)
	request, cancelRequest := context.WithCancel(context.Background())
	candidate, cleanup, err := f.Prepare(request, e)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cancelRequest()
	connection, err := net.Dial("tcp", net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port))))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	banner := make([]byte, 12)
	if _, err = io.ReadFull(connection, banner); err != nil {
		t.Fatal("request completion killed forward", err)
	}
	message := []byte("synthetic-forward-proof")
	_, _ = connection.Write(message)
	got := make([]byte, len(message))
	if _, err = io.ReadFull(connection, got); err != nil || !bytes.Equal(got, message) {
		t.Fatal("forward payload failed", err)
	}
	cleanup()
	cleanup()
	if _, err = connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("viewer cleanup left transport open")
	}
	deadline := time.Now().Add(2 * time.Second)
	for g.closed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if g.calls.Load() != 1 || g.closed.Load() != 1 || runner.calls.Load() != 2 {
		t.Fatalf("lifecycle mismatch: acquire=%d close=%d process=%d", g.calls.Load(), g.closed.Load(), runner.calls.Load())
	}
}

func TestFailureNeverStartsAnUnverifiedOrDirectForward(t *testing.T) {
	for _, name := range []string{"binding", "identity", "authorization", "gateway", "direct", "banner"} {
		t.Run(name, func(t *testing.T) {
			f, e, resolver, g, runner := fixture(t)
			shellName := f.Config.Targets[0].ShellTarget
			sh := resolver.values[shellName]
			switch name {
			case "binding":
				b := *sh.Capability.SSHBinding
				b.Signature = "invalid"
				sh.Capability.SSHBinding = &b
				resolver.values[shellName] = sh
			case "identity":
				sh.Device.IdentityPublicKey = "changed"
				resolver.values[shellName] = sh
			case "authorization":
				sh.Granted = false
				resolver.values[shellName] = sh
			case "gateway":
				g.unavailable = true
			case "direct":
				g.adapter = "lan"
			case "banner":
				runner.wrongBanner = true
			}
			_, cleanup, err := f.Prepare(context.Background(), e)
			if cleanup != nil {
				cleanup()
			}
			if err == nil {
				t.Fatal("unsafe forward accepted")
			}
			if name != "banner" && runner.calls.Load() != 0 {
				t.Fatal("SSH started before authorization/route verification")
			}
		})
	}
}

func TestDesktopRevocationStopsDetachedForward(t *testing.T) {
	f, e, resolver, _, _ := fixture(t)
	candidate, cleanup, err := f.Prepare(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	connection, err := net.Dial("tcp", net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port))))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(4 * time.Second))
	_, _ = io.ReadFull(connection, make([]byte, 12))
	resolver.Lock()
	d := resolver.values[e.Session.CanonicalTarget]
	d.Granted = false
	resolver.values[d.Canonical] = d
	resolver.Unlock()
	if _, err = connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("revoked Desktop kept its SSH transport")
	}
}

func TestUnmappedDesktopPreservesLegacyCandidate(t *testing.T) {
	f, e, _, g, _ := fixture(t)
	f.Config = Config{}
	got, cleanup, err := f.Prepare(context.Background(), e)
	if err != nil || got != e.Route || g.calls.Load() != 0 {
		t.Fatal("legacy Desktop changed")
	}
	cleanup()
}
