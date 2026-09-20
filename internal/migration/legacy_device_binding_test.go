package migration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

func TestApplyLegacyDeviceBindingsRekeysCatalogAndPrivateExecutors(t *testing.T) {
	viewer := filepath.Join(t.TempDir(), "viewer.exe")
	if err := os.WriteFile(viewer, []byte("synthetic executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := LoadLegacyCenterCatalog(externalLegacyCatalog(t, viewer, ""))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	legacyID := candidate.Snapshot.Devices[1].ID
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	bindings := LegacyDeviceBindings{SchemaVersion: LegacyDeviceBindingsSchema, Devices: []LegacyDeviceBinding{{
		LegacyDeviceID: legacyID, DeviceID: deviceID, IdentityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}}}
	rekeyed, err := ApplyLegacyDeviceBindings(candidate, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if rekeyed.Snapshot.Devices[1].ID != deviceID || rekeyed.Snapshot.Devices[1].IdentityPublicKey == "" {
		t.Fatalf("bound Device = %#v", rekeyed.Snapshot.Devices[1])
	}
	for _, capability := range rekeyed.Snapshot.Capabilities {
		if capability.DeviceID != deviceID {
			t.Fatalf("capability Device = %q", capability.DeviceID)
		}
		canonical := targetref.Reference{FabricID: rekeyed.Snapshot.FabricID, DeviceID: deviceID, CapabilityID: capability.ID}.String()
		if capability.DesktopProfile != nil && capability.DesktopProfile.Protocol == "external" && !rekeyed.External.HasTarget(canonical) {
			t.Fatalf("external target was not rekeyed: %s", canonical)
		}
		if capability.DesktopProfile != nil && capability.DesktopProfile.Protocol == "rdp" {
			if _, err := rekeyed.Routes.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: canonical}); err != nil {
				t.Fatalf("route target was not rekeyed: %v", err)
			}
		}
	}
	encoded, err := json.Marshal(rekeyed)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || string(encoded) == "null" {
		t.Fatal("rekeyed candidate was not serializable")
	}
}

func TestLoadLegacyDeviceBindingsRejectsIdentityMismatch(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bindings.json")
	payload := LegacyDeviceBindings{SchemaVersion: LegacyDeviceBindingsSchema, Devices: []LegacyDeviceBinding{{
		LegacyDeviceID: "device-legacy", DeviceID: "device-wrong", IdentityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLegacyDeviceBindings(path); err == nil {
		t.Fatal("identity mismatch was accepted")
	}
}

func TestApplyLegacyDeviceBindingsCollapsesControllerWhenItIsTheBoundDevice(t *testing.T) {
	viewer := filepath.Join(t.TempDir(), "viewer.exe")
	if err := os.WriteFile(viewer, []byte("synthetic executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	source, err := LoadLegacyCenterCatalog(externalLegacyCatalog(t, viewer, ""))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProjectLegacyCenterCandidate(source, deviceID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	legacyID := candidate.Snapshot.Devices[1].ID
	bindings := LegacyDeviceBindings{SchemaVersion: LegacyDeviceBindingsSchema, Devices: []LegacyDeviceBinding{{
		LegacyDeviceID: legacyID, DeviceID: deviceID, IdentityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}}}

	rekeyed, err := ApplyLegacyDeviceBindings(candidate, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(rekeyed.Snapshot.Devices) != 1 || rekeyed.Snapshot.Devices[0].ID != deviceID || rekeyed.Snapshot.Devices[0].Alias == "owner-controller" {
		t.Fatalf("collapsed Devices = %#v", rekeyed.Snapshot.Devices)
	}
	for _, capability := range rekeyed.Snapshot.Capabilities {
		if capability.DeviceID != deviceID {
			t.Fatalf("capability Device = %q", capability.DeviceID)
		}
	}
	for _, grant := range rekeyed.Snapshot.Grants {
		if grant.SubjectDeviceID != deviceID {
			t.Fatalf("grant subject = %q", grant.SubjectDeviceID)
		}
	}
}

func TestApplyLegacyDeviceBindingsRejectsCollisionWithNonController(t *testing.T) {
	viewer := filepath.Join(t.TempDir(), "viewer.exe")
	if err := os.WriteFile(viewer, []byte("synthetic executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	source, err := LoadLegacyCenterCatalog(externalLegacyCatalog(t, viewer, ""))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProjectLegacyCenterCandidate(source, deviceID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Snapshot.Devices[0].Alias = "unrelated-device"
	legacyID := candidate.Snapshot.Devices[1].ID
	bindings := LegacyDeviceBindings{SchemaVersion: LegacyDeviceBindingsSchema, Devices: []LegacyDeviceBinding{{
		LegacyDeviceID: legacyID, DeviceID: deviceID, IdentityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}}}
	if _, err := ApplyLegacyDeviceBindings(candidate, bindings); err == nil {
		t.Fatal("non-controller identity collision was accepted")
	}
}
