package openssh

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/route"
	frproute "github.com/cottman99/pf-remote/internal/route/frp"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type integrationSigner struct {
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func (s integrationSigner) DeviceID() string             { return s.id }
func (s integrationSigner) PublicKey() ed25519.PublicKey { return s.public }
func (s integrationSigner) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(s.private, message), nil
}

type integrationResolver struct{ target contracts.Target }

func (r integrationResolver) ResolveAuthorized(string, string) (contracts.Target, error) {
	return r.target, nil
}

type synchronizedCapture struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

type syntheticLeaseClient struct {
	mu       sync.Mutex
	sequence int
	releases []string
}

func (c *syntheticLeaseClient) Acquire(_ context.Context, request route.Request) (frproute.Lease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	return frproute.Lease{
		SchemaVersion: frproute.LeaseSchema, ID: fmt.Sprintf("lease-openssh-%02d", c.sequence),
		CanonicalTarget: request.CanonicalTarget, Adapter: "frp", ServerAddress: "relay.example.com", ServerPort: 7000,
		ServerUser: "fabric-e2e", ServerName: "proxy-openssh-01", SecretKey: "fx_" + "openssh_secret_0123456789", ExpiresAt: request.AuthorizationExpiry,
	}, nil
}

func (c *syntheticLeaseClient) Release(_ context.Context, leaseID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releases = append(c.releases, leaseID)
	return nil
}

type syntheticFRPStarter struct {
	upstream string
	capture  *synchronizedCapture
}

var bindPortPattern = regexp.MustCompile(`(?m)^bindPort = ([0-9]+)$`)

func (s syntheticFRPStarter) Start(ctx context.Context, _, configPath string) (frproute.Process, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	match := bindPortPattern.FindStringSubmatch(string(content))
	if len(match) != 2 {
		return nil, errors.New("synthetic visitor config has no bind port")
	}
	port, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	process := &syntheticFRPProcess{listener: listener}
	go func() {
		<-ctx.Done()
		_ = process.Stop()
	}()
	go func() {
		for {
			downstream, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			upstream, dialErr := net.Dial("tcp", s.upstream)
			if dialErr != nil {
				_ = downstream.Close()
				continue
			}
			go relayConnection(downstream, upstream, s.capture)
		}
	}()
	return process, nil
}

type syntheticFRPProcess struct {
	once     sync.Once
	listener net.Listener
}

func (p *syntheticFRPProcess) Stop() error {
	var err error
	p.once.Do(func() { err = p.listener.Close() })
	return err
}

func (c *synchronizedCapture) Write(value []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffer.Write(value)
}

func (c *synchronizedCapture) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buffer.Bytes()...)
}

func TestRealOpenSSHEndToEndAuthenticatesPinnedHostKeyAndEncryptsCommand(t *testing.T) {
	sshPath := requireExecutable(t, "ssh")
	sshKeygenPath := requireExecutable(t, "ssh-keygen")
	sshdPath := requireExecutable(t, "sshd")

	dir := t.TempDir()
	hostKeyPath := filepath.Join(dir, "host_key")
	clientKeyPath := filepath.Join(dir, "client_key")
	wrongHostKeyPath := filepath.Join(dir, "wrong_host_key")
	for _, path := range []string{hostKeyPath, clientKeyPath, wrongHostKeyPath} {
		command := exec.Command(sshKeygenPath, "-q", "-t", "ed25519", "-N", "", "-f", path)
		if output, err := command.CombinedOutput(); err != nil {
			t.Skipf("ephemeral ssh-keygen is unavailable: %v (%s)", err, bytes.TrimSpace(output))
		}
	}
	authorizedKey, err := os.ReadFile(clientKeyPath + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "authorized_keys"), authorizedKey, 0o600); err != nil {
		t.Fatal(err)
	}

	sshdPort := reservePort(t)
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("current OS account is unavailable: %v", err)
	}
	login := currentUser.Username
	if slash := strings.LastIndexAny(login, `\`+"/"); slash >= 0 {
		login = login[slash+1:]
	}
	configPath := filepath.Join(dir, "sshd_config")
	config := fmt.Sprintf(`Port %d
ListenAddress 127.0.0.1
AddressFamily inet
HostKey %s
PidFile %s
AuthorizedKeysFile %s
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
StrictModes no
AllowUsers %s
LogLevel ERROR
`, sshdPort, slashPath(hostKeyPath), slashPath(filepath.Join(dir, "sshd.pid")), slashPath(filepath.Join(dir, "authorized_keys")), login)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(sshdPath, "-t", "-f", configPath).CombinedOutput(); err != nil {
		t.Skipf("ephemeral sshd configuration is unavailable: %v (%s)", err, bytes.TrimSpace(output))
	}
	var sshdLog bytes.Buffer
	sshd := exec.Command(sshdPath, "-D", "-e", "-f", configPath)
	sshd.Stdout, sshd.Stderr = &sshdLog, &sshdLog
	if err := sshd.Start(); err != nil {
		t.Skipf("ephemeral sshd could not start: %v", err)
	}
	t.Cleanup(func() {
		_ = sshd.Process.Kill()
		_ = sshd.Wait()
	})
	waitForTCP(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(sshdPort)), &sshdLog)

	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x73}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := integrationSigner{id: deviceID, public: public, private: private}
	target := signedTarget(t, signer, hostKeyPath+".pub")
	now := time.Now().UTC()
	target.Authorization = contracts.Authorization{Status: "active", ValidUntil: now.Add(time.Minute)}
	capture := &synchronizedCapture{}
	leaseClient := &syntheticLeaseClient{}
	adapter := frproute.Adapter{
		Binary: "synthetic-frpc", StateDir: filepath.Join(dir, "frp-sessions"), Client: leaseClient,
		Starter: syntheticFRPStarter{upstream: net.JoinHostPort("127.0.0.1", strconv.Itoa(sshdPort)), capture: capture},
		Now:     func() time.Time { return now }, NewPort: func() (uint16, error) { return uint16(reservePort(t)), nil },
	}
	coordinator := session.Coordinator{
		SubjectDeviceID: "device-controller",
		Resolver:        integrationResolver{target: target},
		RouteProvider:   adapter,
		Executor:        Executor{Binary: sshPath, StateDir: filepath.Join(dir, "sessions")},
		Now:             func() time.Time { return now },
		NewID:           func() (string, error) { return "session-openssh-e2e", nil },
	}
	testContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sentinel := "PFREMOTE_E2E_SENTINEL_8462"
	var stdout bytes.Buffer
	request := session.Request{
		Target:     target.Canonical,
		RemoteUser: login, IdentityFile: clientKeyPath, Command: []string{"echo", sentinel},
		Stdout: &stdout, Stderr: io.Discard,
	}
	result, err := coordinator.Run(testContext, request)
	if err != nil {
		t.Fatalf("real OpenSSH session failed: %v (sshd: %s)", err, bytes.TrimSpace(sshdLog.Bytes()))
	}
	if result.ExitCode != 0 || !strings.Contains(stdout.String(), sentinel) {
		t.Fatalf("exit=%d stdout=%q", result.ExitCode, stdout.String())
	}
	if bytes.Contains(capture.Bytes(), []byte(sentinel)) {
		t.Fatal("captured route bytes exposed Shell command plaintext")
	}
	if result.Session.RouteAdapter != "frp" || len(leaseClient.releases) != 1 {
		t.Fatalf("session route=%q releases=%v", result.Session.RouteAdapter, leaseClient.releases)
	}

	wrong := signedTarget(t, signer, wrongHostKeyPath+".pub")
	wrong.Authorization = target.Authorization
	coordinator.Resolver = integrationResolver{target: wrong}
	coordinator.NewID = func() (string, error) { return "session-openssh-wrong-host", nil }
	stdout.Reset()
	if _, err := coordinator.Run(testContext, request); err == nil {
		t.Fatal("wrong signed host key unexpectedly reached the Shell")
	}
	if strings.Contains(stdout.String(), sentinel) {
		t.Fatal("Shell command ran after host-key mismatch")
	}

	tamperRelay, tampered := startTamperRelay(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(sshdPort)))
	defer tamperRelay.Close()
	coordinator.RouteProvider = nil
	request.Route.ID = "route-local-tampered"
	request.Route.Adapter = "local"
	request.Route.Network = "tcp"
	request.Route.Address = "127.0.0.1"
	request.Route.Port = uint16(tamperRelay.Addr().(*net.TCPAddr).Port)
	coordinator.Resolver = integrationResolver{target: target}
	coordinator.NewID = func() (string, error) { return "session-openssh-tampered", nil }
	stdout.Reset()
	if _, err := coordinator.Run(testContext, request); err == nil {
		t.Fatal("tampered SSH transport unexpectedly reached the Shell")
	}
	if !tampered.Load() {
		t.Fatal("tamper relay did not mutate the SSH transport")
	}
	if strings.Contains(stdout.String(), sentinel) {
		t.Fatal("Shell command ran after SSH transport tampering")
	}
}

func signedTarget(t *testing.T, signer integrationSigner, publicKeyPath string) contracts.Target {
	t.Helper()
	publicLine, err := os.ReadFile(publicKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(publicLine))
	if len(fields) < 2 {
		t.Fatal("ssh-keygen produced an invalid public key")
	}
	binding, err := shellbinding.Sign("fabric-e2e", "shell-main", 1, []contracts.SSHHostKey{{Algorithm: fields[0], PublicKey: fields[1]}}, signer)
	if err != nil {
		t.Fatal(err)
	}
	device := contracts.Device{ID: signer.id, Alias: "local", State: "online", IdentityPublicKey: base64.RawURLEncoding.EncodeToString(signer.public)}
	capability := contracts.Capability{ID: "shell-main", DeviceID: signer.id, Alias: "shell", Kind: contracts.CapabilityShell, State: "available", SSHBinding: binding}
	canonical := "pfremote://fabric-e2e/devices/" + signer.id + "/capabilities/shell-main"
	return contracts.Target{Canonical: canonical, Device: device, Capability: capability, Granted: true}
}

func requireExecutable(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is unavailable", name)
	}
	return path
}

func slashPath(path string) string { return filepath.ToSlash(path) }

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForTCP(t *testing.T, address string, log *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("ephemeral sshd did not listen at %s: %s", address, bytes.TrimSpace(log.Bytes()))
}

func startCaptureRelay(t *testing.T, upstream string) (net.Listener, *synchronizedCapture) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	capture := &synchronizedCapture{}
	go func() {
		for {
			downstream, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			upstreamConnection, dialErr := net.Dial("tcp", upstream)
			if dialErr != nil {
				_ = downstream.Close()
				continue
			}
			go relayConnection(downstream, upstreamConnection, capture)
		}
	}()
	return listener, capture
}

func relayConnection(downstream, upstream net.Conn, capture *synchronizedCapture) {
	var once sync.Once
	closeBoth := func() {
		_ = downstream.Close()
		_ = upstream.Close()
	}
	copyCaptured := func(destination io.Writer, source io.Reader) {
		_, _ = io.Copy(destination, io.TeeReader(source, capture))
		once.Do(closeBoth)
	}
	go copyCaptured(upstream, downstream)
	copyCaptured(downstream, upstream)
}

func startTamperRelay(t *testing.T, upstreamAddress string) (net.Listener, *atomic.Bool) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tampered := &atomic.Bool{}
	go func() {
		downstream, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		upstream, dialErr := net.Dial("tcp", upstreamAddress)
		if dialErr != nil {
			_ = downstream.Close()
			return
		}
		go func() {
			forwardAndTamperAfterNewKeys(downstream, upstream, tampered)
			_ = downstream.Close()
			_ = upstream.Close()
		}()
		buffer := make([]byte, 4096)
		for {
			count, readErr := downstream.Read(buffer)
			if count > 0 {
				if _, writeErr := upstream.Write(buffer[:count]); writeErr != nil {
					break
				}
			}
			if readErr != nil {
				break
			}
		}
		_ = downstream.Close()
		_ = upstream.Close()
	}()
	return listener, tampered
}

func forwardAndTamperAfterNewKeys(destination io.Writer, source io.Reader, tampered *atomic.Bool) {
	reader := bufio.NewReader(source)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, writeErr := destination.Write(line); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
		if bytes.HasPrefix(line, []byte("SSH-")) {
			break
		}
	}
	for {
		header := make([]byte, 4)
		if _, err := io.ReadFull(reader, header); err != nil {
			return
		}
		packetLength := binary.BigEndian.Uint32(header)
		if packetLength < 2 || packetLength > 256*1024 {
			return
		}
		packet := make([]byte, packetLength)
		if _, err := io.ReadFull(reader, packet); err != nil {
			return
		}
		if _, err := destination.Write(append(header, packet...)); err != nil {
			return
		}
		if packet[1] == 21 { // SSH_MSG_NEWKEYS; the next server packet is encrypted.
			break
		}
	}
	buffer := make([]byte, 4096)
	count, err := reader.Read(buffer)
	if count > 0 {
		for index := 0; index < count; index++ {
			buffer[index] ^= 0x5a
		}
		tampered.Store(true)
		if _, writeErr := destination.Write(buffer[:count]); writeErr != nil {
			return
		}
	}
	if err == nil {
		_, _ = io.Copy(destination, reader)
	}
}
