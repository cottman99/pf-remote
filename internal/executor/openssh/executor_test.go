package openssh

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type captureRunner struct {
	args       []string
	knownHosts []byte
	path       string
	calls      int
}

func (r *captureRunner) Run(_ context.Context, _ string, args []string, _ io.Reader, _, _ io.Writer) (int, error) {
	r.calls++
	r.args = append([]string(nil), args...)
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "-o" && strings.HasPrefix(args[index+1], "UserKnownHostsFile=") {
			r.path = strings.Trim(strings.TrimPrefix(args[index+1], "UserKnownHostsFile="), `"`)
			r.knownHosts, _ = os.ReadFile(r.path)
		}
	}
	return 7, nil
}

func TestOpenSSHOptionPathQuotesWhitespace(t *testing.T) {
	path := filepath.Join("parent directory", "known hosts")
	formatted := openSSHOptionPath(path)
	if !strings.HasPrefix(formatted, `"`) || !strings.HasSuffix(formatted, `"`) || !strings.Contains(formatted, "parent directory") {
		t.Fatalf("formatted path = %q", formatted)
	}
}

func TestExecuteUsesIsolatedStrictHostKeyPolicyAndCleansFile(t *testing.T) {
	runner := &captureRunner{}
	executor := Executor{Binary: filepath.Join(t.TempDir(), "ssh-test"), StateDir: t.TempDir(), Runner: runner}
	key := testHostKey("ssh-ed25519", 0x42)
	execution := session.Execution{
		Session: session.Session{CanonicalTarget: "pfremote://fabric-example/devices/device-example/capabilities/shell-main"},
		Route:   route.Candidate{ID: "route-local-1", Adapter: "local", Network: "tcp", Address: "127.0.0.1", Port: 2222},
		Binding: contracts.SSHCapabilityBinding{
			SchemaVersion: contracts.SSHCapabilityBindingSchema,
			FabricID:      "fabric-example", DeviceID: "device-example", CapabilityID: "shell-main",
			BindingVersion: 1, Signature: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 64)),
			HostKeys: []contracts.SSHHostKey{key},
		},
		RemoteUser: "operator", IdentityFile: filepath.Join(t.TempDir(), "identity"),
		Command: []string{"printf", "safe-output"},
	}
	exitCode, err := executor.Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 7 || runner.calls != 1 {
		t.Fatalf("exit=%d calls=%d", exitCode, runner.calls)
	}
	alias := HostKeyAlias(execution.Session.CanonicalTarget)
	wantLine := alias + " " + key.Algorithm + " " + key.PublicKey + "\n"
	if string(runner.knownHosts) != wantLine {
		t.Fatalf("known_hosts = %q", runner.knownHosts)
	}
	for _, option := range []string{
		"BatchMode=yes", "StrictHostKeyChecking=yes",
		"GlobalKnownHostsFile=none", "HostKeyAlias=" + alias,
		"UpdateHostKeys=no", "VerifyHostKeyDNS=no", "ProxyCommand=none",
		"ClearAllForwardings=yes", "ForwardAgent=no", "ForwardX11=no",
		"ControlMaster=no", "PermitLocalCommand=no", "IdentitiesOnly=yes",
	} {
		if !hasOption(runner.args, option) {
			t.Errorf("missing option %q in %#v", option, runner.args)
		}
	}
	if len(runner.args) < 2 || runner.args[0] != "-F" || runner.args[1] != "none" {
		t.Fatalf("configuration was not isolated: %#v", runner.args)
	}
	if !slices.Equal(runner.args[len(runner.args)-3:], []string{"127.0.0.1", "printf", "safe-output"}) {
		t.Fatalf("destination/command tail = %#v", runner.args)
	}
	if _, err := os.Stat(runner.path); !os.IsNotExist(err) {
		t.Fatalf("known_hosts was not removed: %v", err)
	}
}

func TestExecuteRejectsInvalidUserBeforeRunner(t *testing.T) {
	runner := &captureRunner{}
	executor := Executor{Binary: "unused", StateDir: t.TempDir(), Runner: runner}
	_, err := executor.Execute(context.Background(), session.Execution{
		Session:    session.Session{CanonicalTarget: "pfremote://fabric-example/devices/device-example/capabilities/shell-main"},
		Route:      route.Candidate{ID: "route-local-1", Adapter: "local", Network: "tcp", Address: "127.0.0.1", Port: 2222},
		Binding:    contracts.SSHCapabilityBinding{Signature: "synthetic"},
		RemoteUser: "-oProxyCommand=bad",
	})
	if err == nil || runner.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, runner.calls)
	}
}

func TestHostKeyAliasIsStableAndAddressFree(t *testing.T) {
	canonical := "pfremote://fabric-example/devices/device-example/capabilities/shell-main"
	first, second := HostKeyAlias(canonical), HostKeyAlias(canonical)
	if first != second || !strings.HasPrefix(first, "pfremote-") || strings.Contains(first, "device-example") {
		t.Fatalf("alias = %q, %q", first, second)
	}
}

func hasOption(args []string, want string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "-o" && args[index+1] == want {
			return true
		}
	}
	return false
}

func testHostKey(algorithm string, fill byte) contracts.SSHHostKey {
	var blob bytes.Buffer
	_ = binary.Write(&blob, binary.BigEndian, uint32(len(algorithm)))
	blob.WriteString(algorithm)
	_ = binary.Write(&blob, binary.BigEndian, uint32(32))
	blob.Write(bytes.Repeat([]byte{fill}, 32))
	return contracts.SSHHostKey{Algorithm: algorithm, PublicKey: base64.StdEncoding.EncodeToString(blob.Bytes())}
}
