package state

import (
	"errors"

	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const maxShellCapabilityClaims = 4096

// ApplyShellCapabilityClaims merges Device-authenticated Gateway claims into
// an existing named catalog. A claim may confirm only an already-declared
// Shell Capability on the exact Device; it cannot create or rename targets.
func ApplyShellCapabilityClaims(snapshot Snapshot, fabricID string, directoryVersion uint64, claims []contracts.ShellCapabilityClaim) (Snapshot, int, error) {
	if fabricID != snapshot.FabricID || directoryVersion < snapshot.DirectoryVersion || len(claims) > maxShellCapabilityClaims {
		return snapshot, 0, errors.New("Shell capability directory does not match the local catalog")
	}
	updated := snapshot
	updated.Devices = append([]contracts.Device(nil), snapshot.Devices...)
	updated.Capabilities = append([]contracts.Capability(nil), snapshot.Capabilities...)
	deviceIndexes := make(map[string]int, len(updated.Devices))
	capabilityIndexes := make(map[string]int, len(updated.Capabilities))
	for index, device := range updated.Devices {
		deviceIndexes[device.ID] = index
	}
	for index, capability := range updated.Capabilities {
		capabilityIndexes[capability.ID] = index
	}
	seen := make(map[string]struct{}, len(claims))
	changed := 0
	for _, claim := range claims {
		binding := claim.Binding
		deviceIndex, deviceExists := deviceIndexes[claim.DeviceID]
		capabilityIndex, capabilityExists := capabilityIndexes[binding.CapabilityID]
		if !deviceExists || !capabilityExists || claim.DeviceID != binding.DeviceID || claim.DevicePublicKey == "" {
			return snapshot, 0, errors.New("Shell capability claim references an unknown target")
		}
		capability := &updated.Capabilities[capabilityIndex]
		if capability.DeviceID != claim.DeviceID || capability.Kind != contracts.CapabilityShell {
			return snapshot, 0, errors.New("Shell capability claim does not match the declared target")
		}
		key := claim.DeviceID + "\x00" + binding.CapabilityID
		if _, duplicate := seen[key]; duplicate {
			return snapshot, 0, errors.New("Shell capability claim is duplicated")
		}
		seen[key] = struct{}{}
		device := &updated.Devices[deviceIndex]
		if device.IdentityPublicKey != "" && device.IdentityPublicKey != claim.DevicePublicKey {
			return snapshot, 0, errors.New("Shell capability claim conflicts with the Device identity")
		}
		candidateDevice := *device
		candidateDevice.IdentityPublicKey = claim.DevicePublicKey
		candidateCapability := *capability
		candidateCapability.SSHBinding = &binding
		candidateCapability.State = "available"
		if err := shellbinding.Verify(candidateDevice, candidateCapability, snapshot.FabricID); err != nil {
			return snapshot, 0, errors.New("Shell capability claim signature is invalid")
		}
		if capability.SSHBinding != nil {
			if binding.BindingVersion < capability.SSHBinding.BindingVersion {
				return snapshot, 0, errors.New("Shell capability claim is older than the local binding")
			}
			if binding.BindingVersion == capability.SSHBinding.BindingVersion && binding.Signature != capability.SSHBinding.Signature {
				return snapshot, 0, errors.New("Shell capability claim conflicts with the local binding")
			}
		}
		if capability.SSHBinding == nil || capability.SSHBinding.Signature != binding.Signature || capability.State != "available" || device.IdentityPublicKey == "" {
			*device = candidateDevice
			candidateBinding := binding
			candidateBinding.HostKeys = append([]contracts.SSHHostKey(nil), binding.HostKeys...)
			candidateCapability.SSHBinding = &candidateBinding
			*capability = candidateCapability
			changed++
		}
	}
	if changed == 0 {
		return snapshot, 0, nil
	}
	if directoryVersion <= snapshot.DirectoryVersion {
		return snapshot, 0, errors.New("Shell capability directory version did not advance")
	}
	updated.DirectoryVersion = directoryVersion
	if err := updated.Validate(); err != nil {
		return snapshot, 0, errors.New("Shell capability directory update is invalid")
	}
	return updated, changed, nil
}
