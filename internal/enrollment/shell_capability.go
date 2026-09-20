package enrollment

import (
	"math"
	"sort"

	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const maxPublishedShellBindings = 4096

func (m *Manager) PublishShellCapability(request ShellCapabilityPublishRequest) (ShellCapabilityPublishResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	if err := validateRequestID(request.RequestID); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	device := m.devices[request.DeviceID]
	if device == nil {
		return ShellCapabilityPublishResponse{}, fault("DEVICE_NOT_FOUND", "shell-capability-publish", "The Device is not registered in this Fabric.", "Activate the Device before confirming its Shell capability.")
	}
	if device.status != "active" {
		return ShellCapabilityPublishResponse{}, fault("DEVICE_REVOKED", "shell-capability-publish", "The Device identity has been revoked.", "Activate a new Device identity before confirming Shell capabilities.")
	}
	if err := verifySignature(device.publicKey, ShellCapabilityPublishMessage(request), request.Signature, "shell-capability-publish"); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	if m.deviceRequestIDs[request.DeviceID] == nil {
		m.deviceRequestIDs[request.DeviceID] = make(map[string]struct{})
	}
	if _, exists := m.deviceRequestIDs[request.DeviceID][request.RequestID]; exists {
		return ShellCapabilityPublishResponse{}, replayFault("shell-capability-publish")
	}
	binding := request.Binding
	if binding.DeviceID != request.DeviceID || binding.FabricID != m.fabricID || !validIdentifier(binding.CapabilityID) {
		return ShellCapabilityPublishResponse{}, fault("CAPABILITY_IDENTITY_MISMATCH", "shell-capability-publish", "The Shell confirmation does not match this Device and Fabric.", "Refresh the Device directory before confirming the capability again.")
	}
	publicDevice := contracts.Device{ID: device.deviceID, Alias: "verified-device", State: "online", IdentityPublicKey: EncodePublicKey(device.publicKey)}
	publicCapability := contracts.Capability{ID: binding.CapabilityID, DeviceID: device.deviceID, Alias: "verified-shell", Kind: contracts.CapabilityShell, State: "available", SSHBinding: &binding}
	if err := shellbinding.Verify(publicDevice, publicCapability, m.fabricID); err != nil {
		return ShellCapabilityPublishResponse{}, fault("INVALID_CAPABILITY_BINDING", "shell-capability-publish", "The Shell confirmation signature is invalid.", "Let the controlled computer confirm its Shell identity again.")
	}
	key := shellBindingKey(binding.DeviceID, binding.CapabilityID)
	existing, exists := m.shellBindings[key]
	if exists && binding.BindingVersion < existing.BindingVersion {
		return ShellCapabilityPublishResponse{}, fault("CAPABILITY_BINDING_STALE", "shell-capability-publish", "The Shell confirmation is older than the active version.", "Refresh the controlled computer before publishing again.")
	}
	if exists && binding.BindingVersion == existing.BindingVersion && binding.Signature != existing.Signature {
		return ShellCapabilityPublishResponse{}, fault("CAPABILITY_BINDING_CONFLICT", "shell-capability-publish", "The same Shell confirmation version contains different identity data.", "Increase the binding version after an authorized host-key rotation.")
	}
	changed := !exists || binding.BindingVersion > existing.BindingVersion
	if changed {
		if !exists && len(m.shellBindings) >= maxPublishedShellBindings {
			return ShellCapabilityPublishResponse{}, fault("CAPABILITY_CAPACITY_REACHED", "shell-capability-publish", "The Gateway has reached its Shell capability limit.", "Remove obsolete Devices before publishing another capability.")
		}
		if m.directoryVersion == math.MaxUint64 {
			return ShellCapabilityPublishResponse{}, fault("DIRECTORY_VERSION_EXHAUSTED", "shell-capability-publish", "The Device directory version is exhausted.", "Restore the Fabric from a reviewed recovery state.")
		}
	}
	m.recordDeviceRequestID(request.DeviceID, request.RequestID)
	if changed {
		m.shellBindings[key] = cloneShellBinding(binding)
		m.directoryVersion++
	}
	if err := m.persistLocked(); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	return ShellCapabilityPublishResponse{SchemaVersion: SchemaVersion, DeviceID: binding.DeviceID, CapabilityID: binding.CapabilityID, DirectoryVersion: m.directoryVersion, Changed: changed}, nil
}

func (m *Manager) ListShellCapabilities(request ShellCapabilityListRequest) (ShellCapabilityListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return ShellCapabilityListResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return ShellCapabilityListResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return ShellCapabilityListResponse{}, err
	}
	if err := m.verifyOwnerAction(request.OwnerDeviceID, request.RequestID, ShellCapabilityListMessage(request), request.Signature, "shell-capability-list"); err != nil {
		return ShellCapabilityListResponse{}, err
	}
	claims := make([]contracts.ShellCapabilityClaim, 0, len(m.shellBindings))
	for _, binding := range m.shellBindings {
		if device := m.devices[binding.DeviceID]; device != nil && device.status == "active" {
			claims = append(claims, contracts.ShellCapabilityClaim{DeviceID: binding.DeviceID, DevicePublicKey: EncodePublicKey(device.publicKey), Binding: cloneShellBinding(binding)})
		}
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].DeviceID != claims[j].DeviceID {
			return claims[i].DeviceID < claims[j].DeviceID
		}
		return claims[i].Binding.CapabilityID < claims[j].Binding.CapabilityID
	})
	if err := m.persistLocked(); err != nil {
		return ShellCapabilityListResponse{}, err
	}
	return ShellCapabilityListResponse{SchemaVersion: SchemaVersion, FabricID: m.fabricID, DirectoryVersion: m.directoryVersion, Capabilities: claims}, nil
}

func shellBindingKey(deviceID, capabilityID string) string { return deviceID + "\x00" + capabilityID }

func cloneShellBinding(value contracts.SSHCapabilityBinding) contracts.SSHCapabilityBinding {
	clone := value
	clone.HostKeys = append([]contracts.SSHHostKey(nil), value.HostKeys...)
	return clone
}

func (m *Manager) removeShellBindingsForDevice(deviceID string) {
	for key, binding := range m.shellBindings {
		if binding.DeviceID == deviceID {
			delete(m.shellBindings, key)
		}
	}
}
