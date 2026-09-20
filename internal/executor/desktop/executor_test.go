package desktop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	session "github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/route"
)

func TestOSCommandReturnsAfterDesktopClientStarts(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := (OSCommand{}).Run(context.Background(), Launch{Name: executable, Arguments: []string{"-test.run=^TestOSCommandDesktopClientHelper$", "--", "pfremote-desktop-client-helper"}}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("desktop launch waited for client exit: %v", elapsed)
	}
	// Let the helper finish before the Go tool removes the test binary.
	time.Sleep(2200 * time.Millisecond)
}

func TestOSCommandDesktopClientHelper(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "pfremote-desktop-client-helper" {
		return
	}
	time.Sleep(2 * time.Second)
}

type captureCommand struct {
	name        string
	args        []string
	environment map[string]string
}

type fixedTrust struct{ path string }

func (t fixedTrust) VNCTrustFile(string) (string, error) { return t.path, nil }

func (c *captureCommand) Run(_ context.Context, launch Launch) error {
	c.name = launch.Name
	c.args = append([]string(nil), launch.Arguments...)
	c.environment = launch.Environment
	return nil
}

type fixedCredential struct{ value []byte }

func (c fixedCredential) Load(string) ([]byte, error) { return append([]byte(nil), c.value...), nil }

func identityLookup(name string) (string, error) { return name, nil }

func TestWindowsRDPClientUsesTheStableSystemPath(t *testing.T) {
	windowsDirectory := t.TempDir()
	systemDirectory := filepath.Join(windowsDirectory, "System32")
	if err := os.MkdirAll(systemDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(systemDirectory, "mstsc.exe")
	if err := os.WriteFile(client, []byte("synthetic client"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINDIR", windowsDirectory)
	resolved, err := lookupWindowsClient("mstsc.exe")
	if err != nil || resolved != client {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
}

func TestExecutorUsesExistingRDPClientWithoutCredentials(t *testing.T) {
	command := &captureCommand{}
	execution := session.Execution{Session: session.Session{Protocol: "rdp", Authentication: "windows-sso"}, Route: route.Candidate{Address: "127.0.0.1", Port: 3389}}
	if err := (Executor{Command: command, Credentials: fixedCredential{value: []byte("synthetic-secret")}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if command.name != "mstsc.exe" || !reflect.DeepEqual(command.args, []string{"/v:127.0.0.1:3389", "/public"}) {
		t.Fatalf("command=%q args=%q", command.name, command.args)
	}
}

func TestExecutorUsesClientManagedRDPOnlyForVerifiedTailscaleRoute(t *testing.T) {
	command := &captureCommand{}
	execution := session.Execution{
		Session: session.Session{Protocol: "rdp", Authentication: "tailscale-device"},
		Route:   route.Candidate{Adapter: "tailscale", Address: "192.0.2.40", Port: 3389},
	}
	if err := (Executor{Command: command, Credentials: fixedCredential{value: []byte("synthetic-secret")}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if command.name != "mstsc.exe" || !reflect.DeepEqual(command.args, []string{"/v:192.0.2.40:3389"}) {
		t.Fatalf("command=%q args=%q", command.name, command.args)
	}
	execution.Route.Adapter = "lan"
	if err := (Executor{Command: command, Credentials: fixedCredential{value: []byte("synthetic-secret")}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err == nil {
		t.Fatal("expected non-Tailscale route to fail")
	}
}

func TestExecutorUsesExistingVNCClientAndRejectsUnsupportedProtocol(t *testing.T) {
	command := &captureCommand{}
	trustFile := filepath.Join(t.TempDir(), "desktop-ca.pem")
	if err := os.WriteFile(trustFile, []byte("synthetic certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	execution := session.Execution{Session: session.Session{Protocol: "vnc", Authentication: "x509-route-grant"}, Route: route.Candidate{Address: "::1", Port: 5901}}
	if err := (Executor{Command: command, Trust: fixedTrust{path: trustFile}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if command.name != "vncviewer.exe" || !reflect.DeepEqual(command.args, []string{"-SecurityTypes=X509None", "-X509CA=" + trustFile, "[::1]::5901"}) {
		t.Fatalf("command=%q args=%q", command.name, command.args)
	}
	execution.Session.Protocol = "spice"
	if err := (Executor{Command: command, Trust: fixedTrust{path: trustFile}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err == nil {
		t.Fatal("expected unsupported protocol to fail")
	}
}

func TestExecutorUsesTightVNCOnlyForVerifiedTailscaleRoute(t *testing.T) {
	command := &captureCommand{}
	execution := session.Execution{
		Session: session.Session{Protocol: "vnc", Authentication: "tailscale-device"},
		Route:   route.Candidate{Adapter: "tailscale", Address: "192.0.2.40", Port: 5900},
	}
	if err := (Executor{Command: command, Credentials: fixedCredential{value: []byte("synthetic-secret")}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if command.name != "vncviewer.exe" || !reflect.DeepEqual(command.args, []string{"192.0.2.40::5900"}) {
		t.Fatalf("command=%q args=%q", command.name, command.args)
	}
	if len(command.environment) != 0 {
		t.Fatal("Tailscale-authenticated VNC unexpectedly received a password")
	}
	execution.Route.Adapter = "lan"
	if err := (Executor{Command: command, Credentials: fixedCredential{value: []byte("synthetic-secret")}, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err == nil {
		t.Fatal("expected non-Tailscale route to fail")
	}
}

func TestExecutorUsesTailscaleIdentityWithoutOneTimePasswordSetup(t *testing.T) {
	command := &captureCommand{}
	execution := session.Execution{
		Session: session.Session{CanonicalTarget: "pfremote://fabric-example/devices/device-a/capabilities/desktop-a", Protocol: "vnc", Authentication: "tailscale-device"},
		Route:   route.Candidate{Adapter: "tailscale", Address: "192.0.2.40", Port: 5900},
	}
	if err := (Executor{Command: command, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if command.name != "vncviewer.exe" || len(command.environment) != 0 {
		t.Fatalf("command=%q environment=%#v", command.name, command.environment)
	}
}
