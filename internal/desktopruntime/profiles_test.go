package desktopruntime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

func TestProfilesPromoteOnlyExactExistingDesktop(t *testing.T) {
	snapshot := state.SyntheticSnapshot("device-owner-test", time.Now())
	var capability contracts.Capability
	for _, candidate := range snapshot.Capabilities {
		if candidate.Kind == contracts.CapabilityDesktop {
			capability = candidate
			break
		}
	}
	canonical := (targetref.Reference{FabricID: snapshot.FabricID, DeviceID: capability.DeviceID, CapabilityID: capability.ID}).String()
	path := filepath.Join(t.TempDir(), "desktop-profiles-v1.json")
	data := `{"schema_version":"pfremote.desktop-profiles/v1","targets":[{"canonical_target":"` + canonical + `","protocol":"vnc","rendering_environment":"physical","authentication":"tailscale-device","visual_effects_policy":"system"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := LoadProfiles(path)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := profiles.Apply(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range updated.Capabilities {
		if candidate.ID == capability.ID && candidate.State == "available" && candidate.DesktopProfile != nil && candidate.DesktopProfile.RenderingEnvironment == "physical" {
			return
		}
	}
	t.Fatal("updated Desktop is missing")
}

func TestProfilesRejectPartialProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop-profiles-v1.json")
	data := `{"schema_version":"pfremote.desktop-profiles/v1","targets":[{"canonical_target":"pfremote://fabric-test/devices/device-compute/capabilities/desktop-main","protocol":"vnc","rendering_environment":"physical","authentication":"tailscale-device"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfiles(path); err == nil {
		t.Fatal("expected partial profile to fail")
	}
}
