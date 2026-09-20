package desktopruntime

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

const ProfilesSchemaVersion = "pfremote.desktop-profiles/v1"

type ProfileFile struct {
	SchemaVersion string         `json:"schema_version"`
	Targets       []ProfileEntry `json:"targets"`
}

type ProfileEntry struct {
	CanonicalTarget      string `json:"canonical_target"`
	Protocol             string `json:"protocol"`
	RenderingEnvironment string `json:"rendering_environment"`
	Authentication       string `json:"authentication"`
	VisualEffectsPolicy  string `json:"visual_effects_policy,omitempty"`
}

type Profiles struct {
	entries map[string]ProfileEntry
}

func ProfilesPathForState(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "desktop-profiles-v1.json")
}

func LoadProfiles(path string) (Profiles, error) {
	file, err := os.Open(path)
	if err != nil {
		return Profiles{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var value ProfileFile
	if err := decoder.Decode(&value); err != nil {
		return Profiles{}, errors.New("decode Desktop profile configuration")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Profiles{}, errors.New("Desktop profile configuration has trailing content")
	}
	if value.SchemaVersion != ProfilesSchemaVersion || len(value.Targets) == 0 || len(value.Targets) > 256 {
		return Profiles{}, errors.New("Desktop profile configuration metadata is invalid")
	}
	profiles := Profiles{entries: make(map[string]ProfileEntry, len(value.Targets))}
	for _, entry := range value.Targets {
		if _, err := targetref.Parse(entry.CanonicalTarget); err != nil || !validProfile(entry.profile()) {
			return Profiles{}, errors.New("Desktop profile configuration contains an invalid target")
		}
		if _, exists := profiles.entries[entry.CanonicalTarget]; exists {
			return Profiles{}, errors.New("Desktop profile configuration contains a duplicate target")
		}
		profiles.entries[entry.CanonicalTarget] = entry
	}
	return profiles, nil
}

func (p Profiles) Targets() []string {
	result := make([]string, 0, len(p.entries))
	for target := range p.entries {
		result = append(result, target)
	}
	return result
}

func (p Profiles) Apply(snapshot state.Snapshot) (state.Snapshot, error) {
	updated := snapshot
	updated.Capabilities = append([]contracts.Capability(nil), snapshot.Capabilities...)
	for canonical, entry := range p.entries {
		reference, err := targetref.Parse(canonical)
		if err != nil || reference.FabricID != snapshot.FabricID {
			return snapshot, errors.New("Desktop profile does not match the catalog")
		}
		found := false
		for index := range updated.Capabilities {
			capability := &updated.Capabilities[index]
			if capability.ID != reference.CapabilityID || capability.DeviceID != reference.DeviceID {
				continue
			}
			if capability.Kind != contracts.CapabilityDesktop {
				return snapshot, errors.New("Desktop profile references a non-Desktop target")
			}
			profile := entry.profile()
			capability.DesktopProfile = &profile
			capability.State = "available"
			found = true
			break
		}
		if !found {
			return snapshot, errors.New("Desktop profile references an unknown target")
		}
	}
	if err := updated.Validate(); err != nil {
		return snapshot, errors.New("Desktop profile produced an invalid catalog")
	}
	return updated, nil
}

func (e ProfileEntry) profile() contracts.DesktopProfile {
	return contracts.DesktopProfile{Protocol: e.Protocol, RenderingEnvironment: e.RenderingEnvironment, Authentication: e.Authentication, VisualEffectsPolicy: e.VisualEffectsPolicy}
}

func validProfile(profile contracts.DesktopProfile) bool {
	if profile.RenderingEnvironment != "virtual" && profile.RenderingEnvironment != "physical" {
		return false
	}
	if profile.RenderingEnvironment == "physical" && profile.VisualEffectsPolicy != "system" {
		return false
	}
	if profile.RenderingEnvironment == "virtual" && profile.VisualEffectsPolicy != "" && profile.VisualEffectsPolicy != "automatic" && profile.VisualEffectsPolicy != "reduced" && profile.VisualEffectsPolicy != "full" {
		return false
	}
	return profile.Protocol == "rdp" && (profile.Authentication == "windows-sso" || profile.Authentication == "tailscale-device") ||
		profile.Protocol == "vnc" && (profile.Authentication == "x509-route-grant" || profile.Authentication == "tailscale-device" || profile.Authentication == "legacy-vnc-password")
}
