package shellbinding

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type testSigner struct {
	deviceID  string
	publicKey ed25519.PublicKey
	private   ed25519.PrivateKey
	signature []byte
	signErr   bool
	message   []byte
}

func newTestSigner(seedByte byte) *testSigner {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seedByte}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	deviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		panic(err)
	}
	return &testSigner{deviceID: deviceID, publicKey: publicKey, private: privateKey}
}

func (s *testSigner) DeviceID() string { return s.deviceID }

func (s *testSigner) PublicKey() ed25519.PublicKey { return s.publicKey }

func (s *testSigner) Sign(message []byte) ([]byte, error) {
	s.message = append([]byte(nil), message...)
	if s.signErr {
		return nil, testError("synthetic signer failure with key material")
	}
	if s.signature != nil {
		return s.signature, nil
	}
	return ed25519.Sign(s.private, message), nil
}

func TestSign_UsesDomainSeparatedLengthPrefixedContract(t *testing.T) {
	signer := newTestSigner(0x41)
	key := validHostKey("ssh-ed25519", 0x19)
	if _, err := Sign("fabric-test", "shell-main", 0x0102030405060708, []contracts.SSHHostKey{key}, signer); err != nil {
		t.Fatal(err)
	}

	var expected bytes.Buffer
	expected.WriteString("PFREMOTE-SSH-CAPABILITY-BINDING-V1")
	writeContractString := func(value string) {
		_ = binary.Write(&expected, binary.BigEndian, uint32(len(value)))
		expected.WriteString(value)
	}
	writeContractString(contracts.SSHCapabilityBindingSchema)
	writeContractString("fabric-test")
	writeContractString(signer.deviceID)
	writeContractString("shell-main")
	_ = binary.Write(&expected, binary.BigEndian, uint64(0x0102030405060708))
	_ = binary.Write(&expected, binary.BigEndian, uint32(1))
	writeContractString(key.Algorithm)
	writeContractString(key.PublicKey)
	if !bytes.Equal(signer.message, expected.Bytes()) {
		t.Fatalf("signing input does not match the v1 binary contract\n got: %x\nwant: %x", signer.message, expected.Bytes())
	}
}

type testError string

func (e testError) Error() string { return string(e) }

func validHostKey(algorithm string, payload byte) contracts.SSHHostKey {
	var blob bytes.Buffer
	_ = binary.Write(&blob, binary.BigEndian, uint32(len(algorithm)))
	blob.WriteString(algorithm)
	_ = binary.Write(&blob, binary.BigEndian, uint32(32))
	blob.Write(bytes.Repeat([]byte{payload}, 32))
	return contracts.SSHHostKey{Algorithm: algorithm, PublicKey: base64.RawStdEncoding.EncodeToString(blob.Bytes())}
}

func validFixture(t *testing.T) (*testSigner, contracts.Device, contracts.Capability, *contracts.SSHCapabilityBinding) {
	t.Helper()
	signer := newTestSigner(0x42)
	binding, err := Sign("fabric-test", "shell-main", 7, []contracts.SSHHostKey{
		validHostKey("ssh-rsa", 0x22),
		validHostKey("ssh-ed25519", 0x11),
	}, signer)
	if err != nil {
		t.Fatal(err)
	}
	device := contracts.Device{
		ID:                signer.deviceID,
		IdentityPublicKey: base64.RawURLEncoding.EncodeToString(signer.publicKey),
	}
	capability := contracts.Capability{
		ID:         "shell-main",
		DeviceID:   signer.deviceID,
		Kind:       contracts.CapabilityShell,
		SSHBinding: binding,
	}
	return signer, device, capability, binding
}

func cloneBinding(binding *contracts.SSHCapabilityBinding) *contracts.SSHCapabilityBinding {
	cloned := *binding
	cloned.HostKeys = append([]contracts.SSHHostKey(nil), binding.HostKeys...)
	return &cloned
}

func TestSignAndVerify_ValidBinding(t *testing.T) {
	_, device, capability, binding := validFixture(t)
	if binding.SchemaVersion != contracts.SSHCapabilityBindingSchema || binding.BindingVersion != 7 {
		t.Fatalf("unexpected binding version: %#v", binding)
	}
	if strings.Contains(binding.Signature, "=") {
		t.Fatal("signature is not unpadded base64url")
	}
	if err := Verify(device, capability, "fabric-test"); err != nil {
		t.Fatalf("valid binding did not verify: %v", err)
	}
}

func TestSignAndVerify_AcceptsPaddedStandardBase64HostBlob(t *testing.T) {
	signer := newTestSigner(0x43)
	key := validHostKey("ssh-rsa", 0x21)
	blob, err := base64.RawStdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	key.PublicKey = base64.StdEncoding.EncodeToString(blob)
	if !strings.Contains(key.PublicKey, "=") {
		t.Fatal("test fixture did not produce padded base64")
	}
	binding, err := Sign("fabric-test", "shell-main", 1, []contracts.SSHHostKey{key}, signer)
	if err != nil {
		t.Fatalf("padded standard base64 host blob was rejected: %v", err)
	}
	device := contracts.Device{ID: signer.deviceID, IdentityPublicKey: base64.RawURLEncoding.EncodeToString(signer.publicKey)}
	capability := contracts.Capability{ID: "shell-main", DeviceID: signer.deviceID, Kind: contracts.CapabilityShell, SSHBinding: binding}
	if err := Verify(device, capability, "fabric-test"); err != nil {
		t.Fatalf("binding with padded host blob did not verify: %v", err)
	}
}

func TestVerify_TamperedBindingFails(t *testing.T) {
	_, device, capability, original := validFixture(t)
	tests := map[string]func(*contracts.SSHCapabilityBinding){
		"fabric": func(binding *contracts.SSHCapabilityBinding) { binding.FabricID = "fabric-other" },
		"device": func(binding *contracts.SSHCapabilityBinding) { binding.DeviceID = "device-other" },
		"capability": func(binding *contracts.SSHCapabilityBinding) {
			binding.CapabilityID = "shell-other"
		},
		"version": func(binding *contracts.SSHCapabilityBinding) { binding.BindingVersion++ },
		"key": func(binding *contracts.SSHCapabilityBinding) {
			binding.HostKeys[0] = validHostKey(binding.HostKeys[0].Algorithm, 0x99)
		},
		"signature": func(binding *contracts.SSHCapabilityBinding) {
			signature, err := base64.RawURLEncoding.DecodeString(binding.Signature)
			if err != nil {
				t.Fatal(err)
			}
			signature[0] ^= 0xff
			binding.Signature = base64.RawURLEncoding.EncodeToString(signature)
		},
		"schema": func(binding *contracts.SSHCapabilityBinding) {
			binding.SchemaVersion = "pfremote.ssh-capability-binding/v2"
		},
	}
	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			binding := cloneBinding(original)
			tamper(binding)
			candidate := capability
			candidate.SSHBinding = binding
			if err := Verify(device, candidate, "fabric-test"); err == nil {
				t.Fatal("tampered binding verified")
			}
		})
	}
}

func TestVerify_WrongDerivedDeviceIDFails(t *testing.T) {
	_, device, capability, _ := validFixture(t)
	other := newTestSigner(0x51)
	device.IdentityPublicKey = base64.RawURLEncoding.EncodeToString(other.publicKey)
	if err := Verify(device, capability, "fabric-test"); err == nil {
		t.Fatal("Device ID mismatch was accepted")
	}
}

func TestVerify_RequiresExactCapabilityBoundary(t *testing.T) {
	_, device, capability, _ := validFixture(t)

	notShell := capability
	notShell.Kind = contracts.CapabilityDesktop
	if err := Verify(device, notShell, "fabric-test"); err == nil {
		t.Fatal("Desktop Capability accepted an SSH binding")
	}

	wrongDevice := capability
	wrongDevice.DeviceID = "device-other"
	if err := Verify(device, wrongDevice, "fabric-test"); err == nil {
		t.Fatal("Capability belonging to another Device was accepted")
	}

	missing := capability
	missing.SSHBinding = nil
	if err := Verify(device, missing, "fabric-test"); err == nil {
		t.Fatal("missing binding was accepted")
	}
}

func TestSign_RejectsInvalidHostKeySets(t *testing.T) {
	signer := newTestSigner(0x37)
	algorithmMismatch := validHostKey("ssh-ed25519", 1)
	algorithmMismatch.Algorithm = "ssh-rsa"
	oversizedAlgorithm := "a" + strings.Repeat("b", 64)
	oversizedBlob := validHostKey("ssh-ed25519", 1)
	decoded, err := base64.RawStdEncoding.DecodeString(oversizedBlob.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	decoded = append(decoded, bytes.Repeat([]byte{0}, maxHostKeyBlob-len(decoded)+1)...)
	oversizedBlob.PublicKey = base64.RawStdEncoding.EncodeToString(decoded)

	key := validHostKey("ssh-ed25519", 1)
	paddedDuplicate := key
	decoded, err = base64.RawStdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	paddedDuplicate.PublicKey = base64.StdEncoding.EncodeToString(decoded)

	tests := map[string][]contracts.SSHHostKey{
		"empty":              nil,
		"duplicate":          {key, key},
		"duplicate encoding": {key, paddedDuplicate},
		"more than four": {
			validHostKey("ssh-ed25519", 1), validHostKey("ssh-ed25519", 2),
			validHostKey("ssh-ed25519", 3), validHostKey("ssh-ed25519", 4),
			validHostKey("ssh-ed25519", 5),
		},
		"invalid algorithm":    {{Algorithm: "ssh ed25519", PublicKey: key.PublicKey}},
		"algorithm too long":   {{Algorithm: oversizedAlgorithm, PublicKey: key.PublicKey}},
		"invalid base64":       {{Algorithm: "ssh-ed25519", PublicKey: "!!!!"}},
		"oversized blob":       {oversizedBlob},
		"algorithm mismatch":   {algorithmMismatch},
		"malformed blob":       {{Algorithm: "ssh-ed25519", PublicKey: base64.RawStdEncoding.EncodeToString([]byte{1, 2, 3})}},
		"base64 with newline":  {{Algorithm: "ssh-ed25519", PublicKey: key.PublicKey + "\n"}},
		"base64url characters": {{Algorithm: "ssh-ed25519", PublicKey: "____"}},
	}
	for name, keys := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Sign("fabric-test", "shell-main", 1, keys, signer); err == nil {
				t.Fatal("invalid host-key set was accepted")
			}
		})
	}
}

func TestSign_RejectsInvalidTupleVersionAndSigner(t *testing.T) {
	signer := newTestSigner(0x44)
	key := validHostKey("ssh-ed25519", 1)
	if _, err := Sign("fabric test", "shell-main", 1, []contracts.SSHHostKey{key}, signer); err == nil {
		t.Fatal("invalid Fabric ID was accepted")
	}
	if _, err := Sign("fabric-test", "shell/main", 1, []contracts.SSHHostKey{key}, signer); err == nil {
		t.Fatal("invalid Capability ID was accepted")
	}
	if _, err := Sign("fabric-test", "shell-main", 0, []contracts.SSHHostKey{key}, signer); err == nil {
		t.Fatal("zero binding version was accepted")
	}
	if _, err := Sign("fabric-test", "shell-main", maxBindingVersion+1, []contracts.SSHHostKey{key}, signer); err == nil {
		t.Fatal("binding version above SQLite's signed range was accepted")
	}
	if _, err := Sign("fabric-test", "shell-main", 1, []contracts.SSHHostKey{key}, nil); err == nil {
		t.Fatal("nil signer was accepted")
	}

	mismatched := newTestSigner(0x45)
	mismatched.deviceID = signer.deviceID
	if _, err := Sign("fabric-test", "shell-main", 1, []contracts.SSHHostKey{key}, mismatched); err == nil {
		t.Fatal("signer with mismatched Device ID was accepted")
	}

	badSignature := newTestSigner(0x46)
	badSignature.signature = []byte("short")
	if _, err := Sign("fabric-test", "shell-main", 1, []contracts.SSHHostKey{key}, badSignature); err == nil {
		t.Fatal("invalid signer signature was accepted")
	}

	failing := newTestSigner(0x47)
	failing.signErr = true
	_, err := Sign("fabric-test", "shell-main", 1, []contracts.SSHHostKey{key}, failing)
	if err == nil || strings.Contains(err.Error(), "key material") {
		t.Fatalf("signer error was not safely redacted: %v", err)
	}
}

func TestSign_CanonicalSortAndDefensiveCopies(t *testing.T) {
	signer := newTestSigner(0x55)
	first := validHostKey("ssh-rsa", 3)
	second := validHostKey("ssh-ed25519", 2)
	third := validHostKey("ssh-ed25519", 1)
	input := []contracts.SSHHostKey{first, second, third}
	inputBefore := append([]contracts.SSHHostKey(nil), input...)
	publicBefore := append([]byte(nil), signer.publicKey...)

	binding, err := Sign("fabric-test", "shell-main", 1, input, signer)
	if err != nil {
		t.Fatal(err)
	}
	if binding.HostKeys[0].Algorithm != "ssh-ed25519" || binding.HostKeys[0].PublicKey > binding.HostKeys[1].PublicKey || binding.HostKeys[2].Algorithm != "ssh-rsa" {
		t.Fatalf("host keys are not canonically sorted: %#v", binding.HostKeys)
	}
	if !equalHostKeys(input, inputBefore) {
		t.Fatal("Sign reordered the caller's key slice")
	}
	if !bytes.Equal(publicBefore, signer.publicKey) {
		t.Fatal("Sign mutated signer-owned public key bytes")
	}
	input[0] = validHostKey("ssh-ed25519", 9)
	if binding.HostKeys[2] != first {
		t.Fatal("returned binding retained the caller's mutable slice")
	}

	device := contracts.Device{ID: signer.deviceID, IdentityPublicKey: base64.RawURLEncoding.EncodeToString(signer.publicKey)}
	capability := contracts.Capability{ID: "shell-main", DeviceID: signer.deviceID, Kind: contracts.CapabilityShell, SSHBinding: binding}
	binding.HostKeys[0], binding.HostKeys[1] = binding.HostKeys[1], binding.HostKeys[0]
	beforeVerify := append([]contracts.SSHHostKey(nil), binding.HostKeys...)
	if err := Verify(device, capability, "fabric-test"); err != nil {
		t.Fatalf("canonical verification rejected reordered serialized keys: %v", err)
	}
	if !equalHostKeys(binding.HostKeys, beforeVerify) {
		t.Fatal("Verify reordered the caller's binding")
	}
}

func TestFingerprint_OpenSSHSHA256Format(t *testing.T) {
	key := validHostKey("ssh-ed25519", 0x66)
	blob, err := base64.RawStdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(blob)
	want := "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
	got, err := Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || strings.Contains(got, "=") {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
}

func TestKnownHosts_DeterministicCanonicalOutputWithoutMutation(t *testing.T) {
	_, _, _, binding := validFixture(t)
	binding.HostKeys[0], binding.HostKeys[1] = binding.HostKeys[1], binding.HostKeys[0]
	before := append([]contracts.SSHHostKey(nil), binding.HostKeys...)
	alias := "pfremote://fabric-test/devices/device-test/capabilities/shell-main"
	first, err := KnownHosts(alias, binding)
	if err != nil {
		t.Fatal(err)
	}
	second, err := KnownHosts(alias, binding)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("known_hosts output is not deterministic")
	}
	if !equalHostKeys(binding.HostKeys, before) {
		t.Fatal("KnownHosts reordered the caller's binding")
	}
	lines := strings.Split(strings.TrimSuffix(string(first), "\n"), "\n")
	if len(lines) != len(binding.HostKeys) {
		t.Fatalf("known_hosts line count = %d", len(lines))
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != alias {
			t.Fatalf("invalid known_hosts line: %q", line)
		}
		if _, err := base64.StdEncoding.DecodeString(fields[2]); err != nil {
			t.Fatalf("known_hosts key is not padded standard base64: %v", err)
		}
	}
	if lines[0] > lines[1] {
		t.Fatalf("known_hosts keys are not canonically ordered: %q", first)
	}
}

func TestKnownHosts_RejectsAliasInjectionAndInvalidBinding(t *testing.T) {
	_, _, _, binding := validFixture(t)
	aliases := []string{"", "alias with-space", "alias\nattacker", "alias\rattacker", "alias\tattacker", "alias,attacker", strings.Repeat("a", maxAliasLength+1)}
	for _, alias := range aliases {
		if _, err := KnownHosts(alias, binding); err == nil {
			t.Fatalf("unsafe alias %q was accepted", alias)
		}
	}
	if _, err := KnownHosts("safe-alias", nil); err == nil {
		t.Fatal("nil binding was accepted")
	}
	badSignature := cloneBinding(binding)
	badSignature.Signature = "invalid"
	if _, err := KnownHosts("safe-alias", badSignature); err == nil {
		t.Fatal("invalid signature encoding was accepted")
	}
}

func TestVerify_RejectsInvalidIdentityAndSignatureEncodings(t *testing.T) {
	_, device, capability, binding := validFixture(t)
	for _, encoded := range []string{"", "invalid", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, ed25519.PublicKeySize))} {
		candidate := device
		candidate.IdentityPublicKey = encoded
		if err := Verify(candidate, capability, "fabric-test"); err == nil {
			t.Fatalf("invalid Device public key encoding %q was accepted", encoded)
		}
	}
	for _, encoded := range []string{"", "invalid", binding.Signature + "\n", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, ed25519.SignatureSize))} {
		candidateBinding := cloneBinding(binding)
		candidateBinding.Signature = encoded
		candidate := capability
		candidate.SSHBinding = candidateBinding
		if err := Verify(device, candidate, "fabric-test"); err == nil {
			t.Fatalf("invalid signature encoding %q was accepted", encoded)
		}
	}
}

func equalHostKeys(left, right []contracts.SSHHostKey) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
