package desktopcredential

import (
	"bytes"
	"encoding/json"
	"github.com/cottman99/pf-remote/internal/desktopdisplay"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresetsRemainBoundAcrossRestartAndSeparateDesktops(t *testing.T) {
	root := t.TempDir()
	s := newStore(root, testProtector{})
	a := "pfremote://fabric-example/devices/device-a/capabilities/desktop-a"
	b := "pfremote://fabric-example/devices/device-a/capabilities/desktop-b"
	u := contracts.DesktopPresetUpdate{Username: "example-user", Password: strings.Join([]string{"example", "secret"}, "-"), FullScreen: true, DefaultDesktop: true}
	v, err := s.SavePreset(a, "identity-a", "shared-a", u)
	if err != nil || !v.PasswordSaved {
		t.Fatal("save", err)
	}
	public, _ := json.Marshal(v)
	if bytes.Contains(public, []byte(u.Password)) {
		t.Fatal("password response leak")
	}
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		if bytes.Contains(raw, []byte(u.Password)) || bytes.Contains(raw, []byte(u.Username)) {
			t.Fatal("plaintext on disk")
		}
	}
	s = newStore(root, testProtector{})
	p, err := s.LoadRDP(a, "identity-a", "shared-a")
	if err != nil || p.Password != u.Password {
		t.Fatal("restart", err)
	}
	if _, err := s.LoadRDP(a, "identity-b", "shared-a"); err == nil {
		t.Fatal("identity swap accepted")
	}
	recovery, err := s.ReadPreset(a, "identity-b", "shared-a")
	if err != nil || !recovery.RequiresSetup || recovery.PasswordSaved || recovery.Username != "" {
		t.Fatal("changed identity must allow safe settings recovery without old account")
	}
	if _, err := s.LoadRDP(a, "identity-a", "shared-b"); err == nil {
		t.Fatal("workspace swap accepted")
	}
	if _, err := s.LoadRDP(b, "identity-a", "shared-a"); err == nil {
		t.Fatal("sibling secret reused")
	}
	u.Password = ""
	if _, err = s.SavePreset(a, "identity-a", "shared-a", u); err != nil {
		t.Fatal(err)
	}
	p, _ = s.LoadRDP(a, "identity-a", "shared-a")
	if p.Password != "example-secret" {
		t.Fatal("blank erased password")
	}
	if _, err = s.SavePreset(b, "identity-a", "shared-b", u); err != nil {
		t.Fatal(err)
	}
	v, _ = s.ReadPreset(a, "identity-a", "shared-a")
	if v.DefaultDesktop {
		t.Fatal("multiple default desktops")
	}
	u.ClearPassword = true
	v, err = s.SavePreset(a, "identity-a", "shared-a", u)
	if err != nil || v.PasswordSaved {
		t.Fatal("forget", err)
	}
	if _, err = s.SavePreset(a, "identity-a", "shared-a", contracts.DesktopPresetUpdate{Username: "x\r\nfull address:s:example.com"}); err == nil {
		t.Fatal("RDP injection accepted")
	}
}

func TestDisplayPresetMigrationPreservesAccountAndOldClientUpdates(t *testing.T) {
	root := t.TempDir()
	s := newStore(root, testProtector{})
	target := "pfremote://fabric-example/devices/device-example/capabilities/desktop"
	u := contracts.DesktopPresetUpdate{Username: "example-user", Password: strings.Join([]string{"synthetic", "secret"}, "-"), FullScreen: true, Display: &contracts.DesktopDisplaySettings{Mode: desktopdisplay.All}}
	if _, err := s.SavePreset(target, "identity", "", u); err != nil {
		t.Fatal(err)
	}
	s = newStore(root, testProtector{})
	u.Password = ""
	u.Display = nil
	v, err := s.SavePreset(target, "identity", "", u)
	if err != nil || v.Display.Mode != desktopdisplay.All || !v.PasswordSaved {
		t.Fatal("old account update destroyed display or password")
	}
	u.FullScreen = false
	v, err = s.SavePreset(target, "identity", "", u)
	if err != nil || v.Display.Mode != desktopdisplay.Window || v.FullScreen || !v.PasswordSaved {
		t.Fatal("legacy window choice ignored")
	}
	u.Display = &contracts.DesktopDisplaySettings{Mode: desktopdisplay.Selected, MonitorIDs: []int{2, 0}, LayoutID: strings.Repeat("a", 64)}
	v, err = s.SavePreset(target, "identity", "", u)
	if err != nil || !v.FullScreen {
		t.Fatal("explicit selected mode did not override legacy flag")
	}
	p, err := newStore(root, testProtector{}).LoadRDP(target, "identity", "")
	if err != nil || p.Display.Mode != desktopdisplay.Selected || p.Display.MonitorIDs[0] != 2 || p.Password != "synthetic-secret" {
		t.Fatal("restart changed selected primary or saved login")
	}
	u.Display = &contracts.DesktopDisplaySettings{Mode: "invalid"}
	if _, err = s.SavePreset(target, "identity", "", u); err == nil {
		t.Fatal("invalid display accepted")
	}
	after, _ := s.LoadRDP(target, "identity", "")
	if after.Display.Mode != desktopdisplay.Selected || after.Password != p.Password {
		t.Fatal("invalid update changed protected state")
	}
	// Materialize a pre-extension document and exercise migration on read.
	legacy := presetDocument{Schema: "pfremote.desktop-presets/v1", Device: "fabric-example/device-example", Profiles: map[string]RDPPreset{target: {Target: target, Identity: "identity", Username: "example-user", FullScreen: false}}}
	b, _ := json.Marshal(legacy)
	if err = s.Save("rdp-presets/"+legacy.Device, string(b)); err != nil {
		t.Fatal(err)
	}
	v, err = s.ReadPreset(target, "identity", "")
	if err != nil || v.Display.Mode != desktopdisplay.Window {
		t.Fatal("legacy file changed window mode")
	}
}
