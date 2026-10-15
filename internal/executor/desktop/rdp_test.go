package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	session "github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/desktopcredential"
	"github.com/cottman99/pf-remote/internal/desktopdisplay"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type presetCredentials struct {
	preset                     desktopcredential.RDPPreset
	err                        error
	target, identity, selector string
}

func TestDisplayModesAreExplicitAndRetainResources(t *testing.T) {
	for _, mode := range []string{desktopdisplay.Window, desktopdisplay.Single, desktopdisplay.All, desktopdisplay.Selected} {
		d := contracts.DesktopDisplaySettings{Mode: mode}
		wantScreen, wantMulti := "2", "0"
		if mode == desktopdisplay.Window {
			wantScreen = "1"
		}
		if mode == desktopdisplay.All || mode == desktopdisplay.Selected {
			wantMulti = "1"
		}
		wantIDs := ""
		if mode == desktopdisplay.Selected {
			d.MonitorIDs = []int{2, 0}
			d.LayoutID = strings.Repeat("a", 64)
			wantIDs = "2,0"
		}
		text, err := (RDPOptions{Endpoint: "192.0.2.10:3389", Display: &d}).content()
		for _, setting := range []string{"screen mode id:i:" + wantScreen, "use multimon:i:" + wantMulti, "selectedmonitors:s:" + wantIDs, "span monitors:i:0", "audiomode:i:0", "audiocapturemode:i:1", "redirectclipboard:i:1", "authentication level:i:2"} {
			if err != nil || !strings.Contains(text, setting+"\r\n") {
				t.Fatalf("%s lost %s", mode, setting)
			}
		}
	}
}

func TestChangedScreensStopBeforeNativeClientAndKeepExactTarget(t *testing.T) {
	id := strings.Repeat("a", 64)
	store := &presetCredentials{preset: desktopcredential.RDPPreset{FullScreen: true, Display: &contracts.DesktopDisplaySettings{Mode: desktopdisplay.Selected, MonitorIDs: []int{2, 0}, LayoutID: id}}}
	layout := contracts.DesktopMonitorLayout{ID: id, Monitors: []contracts.DesktopMonitor{{ID: 0, Right: 1920, Bottom: 1080}, {ID: 2, Left: 1920, Right: 3840, Bottom: 1080}}}
	command := &captureCommand{}
	e := Executor{Credentials: store, Command: command, GOOS: "windows", Lookup: identityLookup, Displays: func() (contracts.DesktopMonitorLayout, error) { return layout, nil }}
	x := session.Execution{Session: session.Session{Protocol: "rdp", Authentication: "windows-sso", CanonicalTarget: "pfremote://fabric-example/devices/device-example/capabilities/desktop", TargetIdentity: "example-key"}, Route: route.Candidate{Address: "192.0.2.10", Port: 3389}}
	if err := e.Open(context.Background(), x); err != nil || command.rdp == nil || command.rdp.Display.MonitorIDs[0] != 2 {
		t.Fatal("valid screens rejected", err)
	}
	for _, changed := range []contracts.DesktopMonitorLayout{{}, {ID: strings.Repeat("b", 64), Monitors: layout.Monitors}, {ID: id, Monitors: layout.Monitors[:1]}} {
		command.rdp = nil
		layout = changed
		err := e.Open(context.Background(), x)
		var fault *session.ExecutorFault
		if !errors.As(err, &fault) || fault.Code != "DESKTOP_DISPLAY_CHANGED" || command.rdp != nil {
			t.Fatal("stale selection opened native client", err)
		}
		if store.target != x.Session.CanonicalTarget || store.identity != x.Session.TargetIdentity {
			t.Fatal("display choice changed identity")
		}
	}
}

func (s *presetCredentials) Load(string) ([]byte, error) { return nil, desktopcredential.ErrNotFound }
func (s *presetCredentials) LoadRDP(target, identity, selector string) (desktopcredential.RDPPreset, error) {
	s.target, s.identity, s.selector = target, identity, selector
	return s.preset, s.err
}
func TestPresetProjectionPinsIdentityAndNeverWritesSecretToProfile(t *testing.T) {
	store := &presetCredentials{preset: desktopcredential.RDPPreset{Username: "example-account", Password: strings.Join([]string{"synthetic", "secret"}, "-"), FullScreen: false}}
	execution := session.Execution{Session: session.Session{CanonicalTarget: "pfremote://fabric-example/devices/device-example/capabilities/desktop", TargetIdentity: "example-key", SessionSelector: "shared-2"}}
	options, err := (Executor{Credentials: store}).rdpOptions(execution, "192.0.2.10:3389")
	if err != nil || store.target != execution.Session.CanonicalTarget || store.identity != "example-key" || store.selector != "shared-2" {
		t.Fatal("preset lookup lost identity binding")
	}
	profile, err := options.content()
	public, _ := json.Marshal(options)
	if err != nil || !strings.Contains(profile, "username:s:shared-2\\example-account\r\n") || !strings.Contains(profile, "screen mode id:i:1\r\n") || strings.Contains(profile, store.preset.Password) || strings.Contains(string(public), store.preset.Password) {
		t.Fatal("invalid or leaking profile projection")
	}
	store.err = errors.New("identity changed")
	if _, err := (Executor{Credentials: store}).rdpOptions(execution, "192.0.2.10:3389"); err == nil {
		t.Fatal("changed identity reused a saved password")
	}
	store.err = desktopcredential.ErrNotFound
	options, err = (Executor{Credentials: store}).rdpOptions(execution, "192.0.2.10:3389")
	var fault *session.ExecutorFault
	if !errors.As(err, &fault) || fault.Code != "DESKTOP_PRESET_REQUIRED" {
		t.Fatal("missing workspace login must not reach the default xrdp desktop")
	}
	execution.Session.SessionSelector = ""
	options, err = (Executor{Credentials: store}).rdpOptions(execution, "192.0.2.10:3389")
	if err != nil || options.Password != "" || options.Windowed {
		t.Fatal("default connection changed")
	}
}

func TestRDPProfileExplicitAudioAndExactWorkspace(t *testing.T) {
	for _, capture := range []bool{false, true} {
		command := &captureCommand{}
		ctx := session.WithAudioCapture(context.Background(), capture)
		execution := session.Execution{Session: session.Session{Protocol: "rdp", Authentication: "windows-sso", SessionSelector: "workspace-2"}, Route: route.Candidate{Address: "192.0.2.10", Port: 3389}}
		store := &presetCredentials{preset: desktopcredential.RDPPreset{Username: "example-user", Password: strings.Join([]string{"synthetic", "secret"}, "-"), FullScreen: true}}
		if err := (Executor{Command: command, Credentials: store, Lookup: identityLookup, GOOS: "windows"}).Open(ctx, execution); err != nil {
			t.Fatal(err)
		}
		text, err := command.rdp.content()
		if err != nil || !strings.Contains(text, "domain:s:workspace-2\r\n") || !strings.Contains(text, "audiomode:i:0\r\n") || !strings.Contains(text, "audiocapturemode:i:1\r\n") {
			t.Fatalf("profile=%q err=%v", text, err)
		}
		if strings.Contains(text, "password") || strings.Contains(text, "authentication level:i:0") {
			t.Fatal("unsafe RDP profile")
		}
		for _, setting := range []string{"redirectclipboard:i:1\r\n", "screen mode id:i:2\r\n", "redirectprinters:i:1\r\n", "redirectcomports:i:1\r\n", "redirectsmartcards:i:1\r\n", "redirectwebauthn:i:1\r\n", "redirectposdevices:i:1\r\n", "redirectlocation:i:1\r\n", "drivestoredirect:s:*\r\n", "devicestoredirect:s:*\r\n", "usbdevicestoredirect:s:*\r\n", "camerastoredirect:s:*\r\n"} {
			if !strings.Contains(text, setting) {
				t.Fatalf("missing default desktop setting %q", setting)
			}
		}
	}
}

func TestWorkspaceAccountPinsNativeCredentialDomain(t *testing.T) {
	for _, selector := range []string{"workspace-2", "workspace-3", "workspace-4"} {
		o := RDPOptions{Endpoint: "192.0.2.10:3389", SessionSelector: selector, Username: "example-user"}
		account, err := o.nativeUsername()
		if err != nil || account != selector+"\\example-user" {
			t.Fatal("native credential lost selected workspace")
		}
		profile, err := o.content()
		if err != nil || !strings.Contains(profile, "username:s:"+account+"\r\n") {
			t.Fatal("profile and native credential use different accounts")
		}
	}
	for _, username := range []string{"", "other\\example-user", "example-user@other"} {
		store := &presetCredentials{preset: desktopcredential.RDPPreset{Username: username, Password: strings.Join([]string{"synthetic", "secret"}, "-")}}
		execution := session.Execution{Session: session.Session{SessionSelector: "workspace-2"}}
		if _, err := (Executor{Credentials: store}).rdpOptions(execution, "192.0.2.10:3389"); err == nil {
			t.Fatal("ambiguous account can override the selected workspace")
		}
	}
	for _, provider := range []CredentialProvider{nil, &presetCredentials{preset: desktopcredential.RDPPreset{Username: "example-user"}}} {
		command := &captureCommand{}
		execution := session.Execution{Session: session.Session{Protocol: "rdp", Authentication: "windows-sso", SessionSelector: "workspace-2"}, Route: route.Candidate{Address: "192.0.2.10", Port: 3389}}
		err := (Executor{Credentials: provider, Command: command, Lookup: identityLookup, GOOS: "windows"}).Open(context.Background(), execution)
		var fault *session.ExecutorFault
		if !errors.As(err, &fault) || fault.Code != "DESKTOP_PRESET_REQUIRED" || command.rdp != nil {
			t.Fatal("missing credential must stop before starting the native client")
		}
	}
	ordinary := RDPOptions{Username: "example-domain\\example-user"}
	if account, err := ordinary.nativeUsername(); err != nil || account != ordinary.Username {
		t.Fatal("ordinary Windows domain account changed")
	}
}

func TestRDPRejectsSettingInjectionAndCleansProfile(t *testing.T) {
	for _, selector := range []string{"bad\r\naudiocapturemode:i:1", "bad:domain", strings.Repeat("a", 65)} {
		if _, err := (RDPOptions{Endpoint: "192.0.2.10:3389", SessionSelector: selector}).content(); err == nil {
			t.Fatal("accepted unsafe selector")
		}
	}
	path, cleanup, err := writeRDPProfile(RDPOptions{Endpoint: "192.0.2.10:3389"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "audiocapturemode:i:1") {
		t.Fatal("microphone forwarding must default on")
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("profile directory survived cleanup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (OSCommand{}).Run(ctx, Launch{Name: "missing-client", RDP: &RDPOptions{Endpoint: "192.0.2.10:3389"}}); err == nil {
		t.Fatal("canceled launch succeeded")
	}
}

func TestFailedClientStartRemovesRDPProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	t.Setenv("TMPDIR", root)
	err := (OSCommand{SignRDP: func(context.Context, string) error { return nil }}).Run(context.Background(), Launch{Name: filepath.Join(root, "missing-client.exe"), RDP: &RDPOptions{Endpoint: "192.0.2.10:3389"}})
	if err == nil {
		t.Fatal("missing client started")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary settings leaked: %v %v", entries, err)
	}
}

func TestSigningFailureRemovesProfileBeforeClientLaunch(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TEMP", root)
	t.Setenv("TMP", root)
	t.Setenv("TMPDIR", root)
	called := false
	err := (OSCommand{SignRDP: func(_ context.Context, path string) error {
		called = true
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		return errors.New("synthetic signing failure")
	}}).Run(context.Background(), Launch{Name: "must-not-launch", RDP: &RDPOptions{Endpoint: "192.0.2.10:3389"}})
	if err == nil || !called {
		t.Fatal("signing failure must stop launch")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsigned profile survived failure")
	}
}
