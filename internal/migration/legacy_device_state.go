package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const LegacyDeviceStateSchema = "pfremote.legacy-device-state/v1"

type LegacyDeviceStateOverride struct {
	LegacyDeviceID string `json:"legacy_device_id"`
	State          string `json:"state"`
}

type LegacyDeviceStateOverrides struct {
	SchemaVersion string                      `json:"schema_version"`
	Devices       []LegacyDeviceStateOverride `json:"devices"`
}

func LegacyDeviceStatePathForState(statePath string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(statePath)), "legacy-device-state-v1.json")
}

func LoadLegacyDeviceStateOverrides(path string) (LegacyDeviceStateOverrides, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return LegacyDeviceStateOverrides{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxInputBytes {
		return LegacyDeviceStateOverrides{}, errors.New("legacy Device state file is unsafe")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return LegacyDeviceStateOverrides{}, errors.New("legacy Device state file could not be read")
	}
	var overrides LegacyDeviceStateOverrides
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&overrides); err != nil || decoder.Decode(&struct{}{}) != io.EOF || validateLegacyDeviceStateOverrides(overrides) != nil {
		return LegacyDeviceStateOverrides{}, errors.New("legacy Device state file is invalid")
	}
	return overrides, nil
}

func ApplyLegacyDeviceStateOverrides(candidate LegacyCandidate, overrides LegacyDeviceStateOverrides) (LegacyCandidate, error) {
	if err := validateLegacyDeviceStateOverrides(overrides); err != nil {
		return LegacyCandidate{}, err
	}
	states := make(map[string]string, len(overrides.Devices))
	for _, override := range overrides.Devices {
		states[override.LegacyDeviceID] = override.State
	}
	known := make(map[string]struct{}, len(candidate.Snapshot.Devices))
	for _, device := range candidate.Snapshot.Devices {
		known[device.ID] = struct{}{}
	}
	for deviceID := range states {
		if _, exists := known[deviceID]; !exists {
			return LegacyCandidate{}, errors.New("legacy Device state references an unknown Device")
		}
	}

	updated := candidate
	updated.Snapshot.Devices = append([]contracts.Device(nil), candidate.Snapshot.Devices...)
	updated.Snapshot.Capabilities = append([]contracts.Capability(nil), candidate.Snapshot.Capabilities...)
	for index := range updated.Snapshot.Devices {
		if state, exists := states[updated.Snapshot.Devices[index].ID]; exists {
			updated.Snapshot.Devices[index].State = state
		}
	}
	for index := range updated.Snapshot.Capabilities {
		if _, exists := states[updated.Snapshot.Capabilities[index].DeviceID]; exists {
			updated.Snapshot.Capabilities[index].State = "setup-required"
		}
	}
	if err := updated.Snapshot.Validate(); err != nil {
		return LegacyCandidate{}, errors.New("legacy Device state produced an invalid snapshot")
	}
	return updated, nil
}

func validateLegacyDeviceStateOverrides(overrides LegacyDeviceStateOverrides) error {
	if overrides.SchemaVersion != LegacyDeviceStateSchema || len(overrides.Devices) == 0 || len(overrides.Devices) > 1024 {
		return errors.New("legacy Device state metadata is invalid")
	}
	seen := make(map[string]struct{}, len(overrides.Devices))
	for _, override := range overrides.Devices {
		if strings.TrimSpace(override.LegacyDeviceID) == "" || override.State != "offline" {
			return errors.New("legacy Device state override is invalid")
		}
		if _, duplicate := seen[override.LegacyDeviceID]; duplicate {
			return errors.New("legacy Device state override is duplicated")
		}
		seen[override.LegacyDeviceID] = struct{}{}
	}
	return nil
}
