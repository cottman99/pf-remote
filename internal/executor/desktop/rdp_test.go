package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	session "github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/route"
)

func TestRDPProfileExplicitAudioAndExactWorkspace(t *testing.T) {
	for _, capture := range []bool{false, true} {
		command := &captureCommand{}
		ctx := session.WithAudioCapture(context.Background(), capture)
		execution := session.Execution{Session: session.Session{Protocol: "rdp", Authentication: "windows-sso", SessionSelector: "workspace-2"}, Route: route.Candidate{Address: "192.0.2.10", Port: 3389}}
		if err := (Executor{Command: command, Lookup: identityLookup, GOOS: "windows"}).Open(ctx, execution); err != nil {
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
