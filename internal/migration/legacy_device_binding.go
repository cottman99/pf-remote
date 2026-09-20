package migration

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

const LegacyDeviceBindingsSchema = "pfremote.legacy-device-bindings/v1"

type LegacyDeviceBinding struct {
	LegacyDeviceID    string `json:"legacy_device_id"`
	DeviceID          string `json:"device_id"`
	IdentityPublicKey string `json:"identity_public_key"`
}

type LegacyDeviceBindings struct {
	SchemaVersion string                `json:"schema_version"`
	Devices       []LegacyDeviceBinding `json:"devices"`
}

func LegacyDeviceBindingsPathForState(statePath string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(statePath)), "legacy-device-bindings-v1.json")
}

func LoadLegacyDeviceBindings(path string) (LegacyDeviceBindings, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return LegacyDeviceBindings{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxInputBytes {
		return LegacyDeviceBindings{}, errors.New("legacy Device bindings file is unsafe")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return LegacyDeviceBindings{}, errors.New("legacy Device bindings could not be read")
	}
	var bindings LegacyDeviceBindings
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bindings); err != nil || decoder.Decode(&struct{}{}) != io.EOF || validateLegacyDeviceBindings(bindings) != nil {
		return LegacyDeviceBindings{}, errors.New("legacy Device bindings are invalid")
	}
	return bindings, nil
}

func ApplyLegacyDeviceBindings(candidate LegacyCandidate, bindings LegacyDeviceBindings) (LegacyCandidate, error) {
	if err := validateLegacyDeviceBindings(bindings); err != nil {
		return LegacyCandidate{}, err
	}
	byLegacy := make(map[string]LegacyDeviceBinding, len(bindings.Devices))
	for _, binding := range bindings.Devices {
		byLegacy[binding.LegacyDeviceID] = binding
	}
	knownDevices := make(map[string]struct{}, len(candidate.Snapshot.Devices))
	deviceByID := make(map[string]contracts.Device, len(candidate.Snapshot.Devices))
	for _, device := range candidate.Snapshot.Devices {
		knownDevices[device.ID] = struct{}{}
		deviceByID[device.ID] = device
	}
	for legacyID := range byLegacy {
		if _, exists := knownDevices[legacyID]; !exists {
			return LegacyCandidate{}, errors.New("legacy Device binding references an unknown Device")
		}
	}
	capabilitiesByDevice := make(map[string]int, len(candidate.Snapshot.Devices))
	for _, capability := range candidate.Snapshot.Capabilities {
		capabilitiesByDevice[capability.DeviceID]++
	}
	collapsedControllers := make(map[string]struct{})
	for legacyID, binding := range byLegacy {
		if legacyID == binding.DeviceID {
			continue
		}
		existing, collision := deviceByID[binding.DeviceID]
		if !collision {
			continue
		}
		if _, targetIsAlsoLegacy := byLegacy[binding.DeviceID]; targetIsAlsoLegacy || existing.Alias != "owner-controller" || capabilitiesByDevice[binding.DeviceID] != 0 {
			return LegacyCandidate{}, errors.New("bound Device identity collides with an existing Device")
		}
		collapsedControllers[binding.DeviceID] = struct{}{}
	}

	rekeyed := candidate
	rekeyed.Snapshot.Devices = make([]contracts.Device, 0, len(candidate.Snapshot.Devices))
	for _, device := range candidate.Snapshot.Devices {
		if _, collapse := collapsedControllers[device.ID]; collapse {
			continue
		}
		rekeyed.Snapshot.Devices = append(rekeyed.Snapshot.Devices, device)
	}
	rekeyed.Snapshot.Capabilities = append([]contracts.Capability(nil), candidate.Snapshot.Capabilities...)
	rekeyed.Snapshot.Grants = append([]contracts.Grant(nil), candidate.Snapshot.Grants...)
	deviceIDs := make(map[string]string, len(bindings.Devices))
	canonicalTargets := make(map[string]string)
	for index := range rekeyed.Snapshot.Devices {
		device := &rekeyed.Snapshot.Devices[index]
		binding, exists := byLegacy[device.ID]
		if !exists {
			continue
		}
		deviceIDs[device.ID] = binding.DeviceID
		device.ID = binding.DeviceID
		device.IdentityPublicKey = binding.IdentityPublicKey
	}
	for index := range rekeyed.Snapshot.Capabilities {
		capability := &rekeyed.Snapshot.Capabilities[index]
		newDeviceID, exists := deviceIDs[capability.DeviceID]
		if !exists {
			continue
		}
		oldTarget := targetref.Reference{FabricID: rekeyed.Snapshot.FabricID, DeviceID: capability.DeviceID, CapabilityID: capability.ID}.String()
		capability.DeviceID = newDeviceID
		capability.SSHBinding = nil
		newTarget := targetref.Reference{FabricID: rekeyed.Snapshot.FabricID, DeviceID: capability.DeviceID, CapabilityID: capability.ID}.String()
		canonicalTargets[oldTarget] = newTarget
	}
	for index := range rekeyed.Snapshot.Grants {
		if newSubject, exists := deviceIDs[rekeyed.Snapshot.Grants[index].SubjectDeviceID]; exists {
			rekeyed.Snapshot.Grants[index].SubjectDeviceID = newSubject
		}
	}
	if err := rekeyed.Snapshot.Validate(); err != nil {
		return LegacyCandidate{}, errors.New("bound legacy candidate snapshot is invalid")
	}
	rekeyed.Routes = candidate.Routes.rekeyTargets(canonicalTargets)
	rekeyed.Gateway = candidate.Gateway.rekeyTargets(canonicalTargets)
	rekeyed.External = candidate.External.rekeyTargets(canonicalTargets)
	rekeyed.Shell = candidate.Shell.rekeyTargets(canonicalTargets)
	return rekeyed, nil
}

func validateLegacyDeviceBindings(bindings LegacyDeviceBindings) error {
	if bindings.SchemaVersion != LegacyDeviceBindingsSchema || len(bindings.Devices) == 0 || len(bindings.Devices) > 1024 {
		return errors.New("legacy Device bindings metadata is invalid")
	}
	legacyIDs := make(map[string]struct{}, len(bindings.Devices))
	deviceIDs := make(map[string]struct{}, len(bindings.Devices))
	for _, binding := range bindings.Devices {
		if strings.TrimSpace(binding.LegacyDeviceID) == "" || strings.TrimSpace(binding.DeviceID) == "" || strings.TrimSpace(binding.IdentityPublicKey) == "" {
			return errors.New("legacy Device binding is incomplete")
		}
		publicKey, err := base64.RawURLEncoding.Strict().DecodeString(binding.IdentityPublicKey)
		if err != nil || len(publicKey) != ed25519.PublicKeySize {
			return errors.New("legacy Device binding public identity is invalid")
		}
		derived, err := identity.DeviceIDFromPublicKey(ed25519.PublicKey(publicKey))
		if err != nil || derived != binding.DeviceID {
			return errors.New("legacy Device binding identity does not match")
		}
		if _, duplicate := legacyIDs[binding.LegacyDeviceID]; duplicate {
			return errors.New("legacy Device binding is duplicated")
		}
		if _, duplicate := deviceIDs[binding.DeviceID]; duplicate {
			return errors.New("bound Device identity is duplicated")
		}
		legacyIDs[binding.LegacyDeviceID] = struct{}{}
		deviceIDs[binding.DeviceID] = struct{}{}
	}
	return nil
}
