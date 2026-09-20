package state

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestApplyShellCapabilityClaimsConfirmsExistingNamedTarget(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := localShellSigner{id: deviceID, public: public, private: private}
	binding, err := shellbinding.Sign("fabric-claim", "shell-main", 1, []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: syntheticStateHostKey}}, signer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 30, 2, 0, 0, 0, time.UTC)
	snapshot := Snapshot{SchemaVersion: SnapshotSchema, FabricID: "fabric-claim", DirectoryVersion: 1, GrantVersion: 1, CapturedAt: now,
		Devices:      []contracts.Device{{ID: deviceID, Alias: "workstation", DisplayName: "Workstation", State: "online"}},
		Capabilities: []contracts.Capability{{ID: "shell-main", DeviceID: deviceID, Alias: "automation", DisplayName: "Automation", Kind: contracts.CapabilityShell, State: "setup-required"}},
		Grants:       []contracts.Grant{{ID: "grant-shell", SubjectDeviceID: deviceID, CapabilityID: "shell-main", State: "active"}},
	}
	claim := contracts.ShellCapabilityClaim{DeviceID: deviceID, DevicePublicKey: base64.RawURLEncoding.EncodeToString(public), Binding: *binding}
	updated, changed, err := ApplyShellCapabilityClaims(snapshot, "fabric-claim", 2, []contracts.ShellCapabilityClaim{claim})
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 || updated.DirectoryVersion != 2 || updated.Capabilities[0].State != "available" || updated.Capabilities[0].SSHBinding == nil {
		t.Fatalf("updated=%#v changed=%d", updated, changed)
	}
	if !updated.CapturedAt.Equal(now) {
		t.Fatal("capability sync extended the cached authorization capture time")
	}
	if snapshot.Capabilities[0].SSHBinding != nil || snapshot.Devices[0].IdentityPublicKey != "" {
		t.Fatal("source snapshot was mutated")
	}
}

func TestApplyShellCapabilityClaimsRejectsWrongTarget(t *testing.T) {
	snapshot := SyntheticSnapshot("device-controller", time.Now())
	claim := contracts.ShellCapabilityClaim{DeviceID: "device-other", DevicePublicKey: "invalid", Binding: contracts.SSHCapabilityBinding{CapabilityID: "shell-main"}}
	if _, _, err := ApplyShellCapabilityClaims(snapshot, snapshot.FabricID, snapshot.DirectoryVersion+1, []contracts.ShellCapabilityClaim{claim}); err == nil {
		t.Fatal("wrong target claim accepted")
	}
}
