package desktopcredential

import (
	"bytes"
	"encoding/json"
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
