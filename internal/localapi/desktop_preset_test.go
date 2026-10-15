package localapi

import (
	"encoding/json"
	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"strings"
	"testing"
)

type testPresets struct{ writes int }

func (s *testPresets) ReadPreset(string, string, string) (contracts.DesktopPreset, error) {
	return contracts.DesktopPreset{SchemaVersion: "pfremote.desktop-preset/v1", FullScreen: true}, nil
}
func (s *testPresets) SavePreset(string, string, string, contracts.DesktopPresetUpdate) (contracts.DesktopPreset, error) {
	s.writes++
	return contracts.DesktopPreset{PasswordSaved: true}, nil
}
func TestPresetSaveIsLocalAuthorizedAndDoesNotOpen(t *testing.T) {
	s := actions.New()
	store := &testPresets{}
	s.DesktopPresets = store
	h := Handler{Service: s}
	u := &contracts.DesktopPresetUpdate{Username: "example", Password: strings.Join([]string{"never", "echo", "this"}, "-")}
	r := h.Handle(Request{SchemaVersion: SchemaVersion, Action: "desktop-preset", Target: "compute-node/desktop", DesktopPreset: u})
	b, _ := json.Marshal(r)
	if r.Error != nil || store.writes != 1 || strings.Contains(string(b), u.Password) {
		t.Fatal("preset response")
	}
	r = h.Handle(Request{SchemaVersion: SchemaVersion, Action: "desktop-preset", Target: "compute-node/shell", DesktopPreset: u})
	if r.Error == nil || store.writes != 1 {
		t.Fatal("shell accepted")
	}
	r = h.Handle(Request{SchemaVersion: SchemaVersion, Action: "desktop-preset", Target: "pfremote://other/devices/other/capabilities/other", DesktopPreset: u})
	if r.Error == nil || store.writes != 1 {
		t.Fatal("unknown identity accepted")
	}
	h.Gate = &ActionGate{}
	h.Gate.BeginUpdate()
	r = h.Handle(Request{SchemaVersion: SchemaVersion, Action: "desktop-preset", Target: "compute-node/desktop", DesktopPreset: u})
	if r.Error == nil || r.Error.Code != "UPDATE_IN_PROGRESS" || store.writes != 1 {
		t.Fatal("settings write admitted during upgrade")
	}
	r = h.Handle(Request{SchemaVersion: SchemaVersion, Action: "desktop-preset", Target: "compute-node/desktop"})
	if r.Error != nil {
		t.Fatal("read-only settings should remain visible during upgrade")
	}
}
