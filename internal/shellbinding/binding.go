// Package shellbinding binds an immutable PF Remote Shell Capability to the
// OpenSSH host keys authorized by the target Device identity.
package shellbinding

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

const (
	signingDomain     = "PFREMOTE-SSH-CAPABILITY-BINDING-V1"
	maxHostKeys       = 4
	maxHostKeyBlob    = 16 << 10
	maxAliasLength    = 512
	maxBindingVersion = uint64(1<<63 - 1)
)

var (
	algorithmPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@._+-]{0,63}$`)
	aliasPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@._+:/-]{0,511}$`)
)

// DeviceSigner is the minimum protected Device identity surface needed to
// create a Capability binding. identity.Identity implements this interface.
type DeviceSigner interface {
	DeviceID() string
	PublicKey() ed25519.PublicKey
	Sign([]byte) ([]byte, error)
}

// Sign creates a canonical, Device-signed SSH Capability binding. The input
// key slice and signer-owned key material are never retained.
func Sign(fabricID, capabilityID string, bindingVersion uint64, keys []contracts.SSHHostKey, signer DeviceSigner) (*contracts.SSHCapabilityBinding, error) {
	if signer == nil {
		return nil, errors.New("Device signer is required")
	}
	deviceID := signer.DeviceID()
	publicKey := append(ed25519.PublicKey(nil), signer.PublicKey()...)
	derivedDeviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		return nil, errors.New("Device signer has an invalid public key")
	}
	if deviceID != derivedDeviceID {
		return nil, errors.New("Device signer ID does not match its public key")
	}
	if err := validateTuple(fabricID, deviceID, capabilityID); err != nil {
		return nil, err
	}
	if bindingVersion == 0 || bindingVersion > maxBindingVersion {
		return nil, errors.New("SSH binding version must be between one and the signed 64-bit maximum")
	}
	canonicalKeys, err := canonicalHostKeys(keys)
	if err != nil {
		return nil, err
	}

	binding := &contracts.SSHCapabilityBinding{
		SchemaVersion:  contracts.SSHCapabilityBindingSchema,
		FabricID:       fabricID,
		DeviceID:       deviceID,
		CapabilityID:   capabilityID,
		BindingVersion: bindingVersion,
		HostKeys:       canonicalKeys,
	}
	message := signingInput(binding)
	signature, err := signer.Sign(message)
	if err != nil {
		return nil, errors.New("sign SSH Capability binding with Device identity")
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, errors.New("Device signer returned an invalid signature")
	}
	binding.Signature = base64.RawURLEncoding.EncodeToString(append([]byte(nil), signature...))
	return binding, nil
}

// Verify proves that a catalog Device public identity authorized the exact
// Fabric/Device/Capability tuple and host-key list carried by capability.
func Verify(device contracts.Device, capability contracts.Capability, fabricID string) error {
	if capability.Kind != contracts.CapabilityShell {
		return errors.New("SSH binding requires a Shell Capability")
	}
	if capability.DeviceID != device.ID {
		return errors.New("Capability Device ID does not match the target Device")
	}
	if capability.SSHBinding == nil {
		return errors.New("Shell Capability has no SSH binding")
	}
	binding := capability.SSHBinding
	if binding.SchemaVersion != contracts.SSHCapabilityBindingSchema {
		return errors.New("SSH binding uses an unsupported schema")
	}
	if err := validateTuple(fabricID, device.ID, capability.ID); err != nil {
		return err
	}
	if binding.FabricID != fabricID || binding.DeviceID != device.ID || binding.CapabilityID != capability.ID {
		return errors.New("SSH binding does not match the target tuple")
	}
	if binding.BindingVersion == 0 || binding.BindingVersion > maxBindingVersion {
		return errors.New("SSH binding version must be between one and the signed 64-bit maximum")
	}
	canonicalKeys, err := canonicalHostKeys(binding.HostKeys)
	if err != nil {
		return err
	}

	publicKey, err := decodeDevicePublicKey(device.IdentityPublicKey)
	if err != nil {
		return err
	}
	derivedDeviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil || derivedDeviceID != device.ID {
		return errors.New("Device ID does not match its identity public key")
	}
	signature, err := decodeSignature(binding.Signature)
	if err != nil {
		return err
	}
	canonical := *binding
	canonical.HostKeys = canonicalKeys
	if !identity.Verify(publicKey, signingInput(&canonical), signature) {
		return errors.New("SSH binding signature is invalid")
	}
	return nil
}

// Fingerprint returns the OpenSSH-style SHA256 fingerprint of one public-key
// blob without exposing the key itself in errors.
func Fingerprint(key contracts.SSHHostKey) (string, error) {
	blob, err := decodeHostKey(key)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]), nil
}

// KnownHosts renders deterministic strict-known-hosts input for an immutable
// OpenSSH HostKeyAlias. Callers must still Verify the binding first.
func KnownHosts(alias string, binding *contracts.SSHCapabilityBinding) ([]byte, error) {
	if len(alias) == 0 || len(alias) > maxAliasLength || !aliasPattern.MatchString(alias) || strings.ContainsAny(alias, "\r\n\t ") {
		return nil, errors.New("OpenSSH host-key alias is invalid")
	}
	if binding == nil {
		return nil, errors.New("SSH binding is required")
	}
	if binding.SchemaVersion != contracts.SSHCapabilityBindingSchema {
		return nil, errors.New("SSH binding uses an unsupported schema")
	}
	if err := validateTuple(binding.FabricID, binding.DeviceID, binding.CapabilityID); err != nil {
		return nil, err
	}
	if binding.BindingVersion == 0 || binding.BindingVersion > maxBindingVersion {
		return nil, errors.New("SSH binding version must be between one and the signed 64-bit maximum")
	}
	if _, err := decodeSignature(binding.Signature); err != nil {
		return nil, err
	}
	keys, err := canonicalHostKeys(binding.HostKeys)
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	for _, key := range keys {
		blob, err := decodeHostKey(key)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "%s %s %s\n", alias, key.Algorithm, base64.StdEncoding.EncodeToString(blob))
	}
	return output.Bytes(), nil
}

func validateTuple(fabricID, deviceID, capabilityID string) error {
	reference := targetref.Reference{FabricID: fabricID, DeviceID: deviceID, CapabilityID: capabilityID}
	if err := reference.Validate(); err != nil {
		return errors.New("SSH binding contains an invalid target identifier")
	}
	return nil
}

func canonicalHostKeys(keys []contracts.SSHHostKey) ([]contracts.SSHHostKey, error) {
	if len(keys) == 0 || len(keys) > maxHostKeys {
		return nil, errors.New("SSH binding must contain between one and four host keys")
	}
	canonical := append([]contracts.SSHHostKey(nil), keys...)
	type decodedKey struct {
		algorithm string
		blob      []byte
	}
	decoded := make([]decodedKey, len(canonical))
	for index, key := range canonical {
		blob, err := decodeHostKey(key)
		if err != nil {
			return nil, err
		}
		decoded[index] = decodedKey{algorithm: key.Algorithm, blob: blob}
	}
	for index := range decoded {
		for previous := 0; previous < index; previous++ {
			if decoded[index].algorithm == decoded[previous].algorithm && bytes.Equal(decoded[index].blob, decoded[previous].blob) {
				return nil, errors.New("SSH binding contains a duplicate host key")
			}
		}
	}
	sort.Slice(canonical, func(left, right int) bool {
		if canonical[left].Algorithm != canonical[right].Algorithm {
			return canonical[left].Algorithm < canonical[right].Algorithm
		}
		return canonical[left].PublicKey < canonical[right].PublicKey
	})
	return canonical, nil
}

func decodeHostKey(key contracts.SSHHostKey) ([]byte, error) {
	if !algorithmPattern.MatchString(key.Algorithm) {
		return nil, errors.New("SSH host-key algorithm is invalid")
	}
	blob, err := decodeStandardBase64(key.PublicKey)
	if err != nil || len(blob) > maxHostKeyBlob {
		return nil, errors.New("SSH host-key public blob is invalid or exceeds the size limit")
	}
	if len(blob) < 4 {
		return nil, errors.New("SSH host-key public blob is malformed")
	}
	algorithmLength := int(binary.BigEndian.Uint32(blob[:4]))
	if algorithmLength == 0 || algorithmLength > len(blob)-4 || string(blob[4:4+algorithmLength]) != key.Algorithm {
		return nil, errors.New("SSH host-key public blob does not match its algorithm")
	}
	if len(blob) == 4+algorithmLength {
		return nil, errors.New("SSH host-key public blob is malformed")
	}
	return blob, nil
}

func decodeStandardBase64(encoded string) ([]byte, error) {
	if encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(maxHostKeyBlob+1) || strings.ContainsAny(encoded, "\r\n\t ") {
		return nil, errors.New("invalid standard base64")
	}
	if strings.Contains(encoded, "=") {
		return base64.StdEncoding.Strict().DecodeString(encoded)
	}
	return base64.RawStdEncoding.Strict().DecodeString(encoded)
}

func decodeDevicePublicKey(encoded string) (ed25519.PublicKey, error) {
	if encoded == "" || strings.Contains(encoded, "=") || strings.ContainsAny(encoded, "\r\n\t ") {
		return nil, errors.New("Device identity public key is invalid")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("Device identity public key is invalid")
	}
	return ed25519.PublicKey(decoded), nil
}

func decodeSignature(encoded string) ([]byte, error) {
	if encoded == "" || strings.Contains(encoded, "=") || strings.ContainsAny(encoded, "\r\n\t ") {
		return nil, errors.New("SSH binding has an invalid signature encoding")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.SignatureSize {
		return nil, errors.New("SSH binding has an invalid signature encoding")
	}
	return decoded, nil
}

func signingInput(binding *contracts.SSHCapabilityBinding) []byte {
	var output bytes.Buffer
	output.WriteString(signingDomain)
	writeString(&output, binding.SchemaVersion)
	writeString(&output, binding.FabricID)
	writeString(&output, binding.DeviceID)
	writeString(&output, binding.CapabilityID)
	_ = binary.Write(&output, binary.BigEndian, binding.BindingVersion)
	_ = binary.Write(&output, binary.BigEndian, uint32(len(binding.HostKeys)))
	for _, key := range binding.HostKeys {
		writeString(&output, key.Algorithm)
		writeString(&output, key.PublicKey)
	}
	return output.Bytes()
}

func writeString(output *bytes.Buffer, value string) {
	_ = binary.Write(output, binary.BigEndian, uint32(len(value)))
	output.WriteString(value)
}
