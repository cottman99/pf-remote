package desktop

import (
	"context"
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
		if err != nil || !strings.Contains(text, "domain:s:workspace-2\r\n") || !strings.Contains(text, "audiomode:i:0\r\n") || command.rdp.AudioCapture != capture {
			t.Fatalf("profile=%q err=%v", text, err)
		}
		if strings.Contains(text, "password") || strings.Contains(text, "authentication level:i:0") {
			t.Fatal("unsafe RDP profile")
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
	if err != nil || !strings.Contains(string(data), "audiocapturemode:i:0") {
		t.Fatal("microphone must default off")
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
	err := (OSCommand{}).Run(context.Background(), Launch{Name: filepath.Join(root, "missing-client.exe"), RDP: &RDPOptions{Endpoint: "192.0.2.10:3389"}})
	if err == nil {
		t.Fatal("missing client started")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary settings leaked: %v %v", entries, err)
	}
}
