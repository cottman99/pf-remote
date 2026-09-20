package state

import (
	"encoding/base64"
	"errors"
	"math"

	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

// BindLocalShellCapabilities lets a controlled node attest its own OpenSSH
// public host keys with its protected Device identity. It cannot bind another
// Device and it never learns a key from a network route.
func BindLocalShellCapabilities(snapshot Snapshot, signer shellbinding.DeviceSigner, hostKeys []contracts.SSHHostKey) (Snapshot, int, error) {
	if signer == nil || len(hostKeys) == 0 {
		return snapshot, 0, nil
	}
	deviceIndex := -1
	for index := range snapshot.Devices {
		if snapshot.Devices[index].ID == signer.DeviceID() {
			deviceIndex = index
			break
		}
	}
	if deviceIndex < 0 {
		return snapshot, 0, errors.New("local Device is absent from the state snapshot")
	}
	publicKey := base64.RawURLEncoding.EncodeToString(signer.PublicKey())
	if existing := snapshot.Devices[deviceIndex].IdentityPublicKey; existing != "" && existing != publicKey {
		return snapshot, 0, errors.New("local Device identity does not match the state snapshot")
	}
	if snapshot.DirectoryVersion == math.MaxUint64 {
		return snapshot, 0, errors.New("state directory version is exhausted")
	}
	updated := snapshot
	updated.Devices = append([]contracts.Device(nil), snapshot.Devices...)
	updated.Capabilities = append([]contracts.Capability(nil), snapshot.Capabilities...)
	updated.Devices[deviceIndex].IdentityPublicKey = publicKey
	bound := 0
	for index := range updated.Capabilities {
		capability := &updated.Capabilities[index]
		if capability.DeviceID != signer.DeviceID() || capability.Kind != contracts.CapabilityShell || capability.SSHBinding != nil {
			continue
		}
		binding, err := shellbinding.Sign(updated.FabricID, capability.ID, 1, hostKeys, signer)
		if err != nil {
			return snapshot, 0, err
		}
		capability.SSHBinding = binding
		bound++
	}
	if bound == 0 {
		return snapshot, 0, nil
	}
	updated.DirectoryVersion++
	if err := updated.Validate(); err != nil {
		return snapshot, 0, errors.New("local Shell identity update is invalid")
	}
	return updated, bound, nil
}
