package migration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

func TestProjectLegacyCenterOverlayPreservesBaseAndMergesPrivateBindings(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	baseSource, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	baseShell := baseSource.Connections["service-shell"]
	baseShell.Username = "base-operator"
	baseSource.Connections["service-shell"] = baseShell
	base, err := ProjectLegacyCenterCandidate(baseSource, "device-controller", now)
	if err != nil {
		t.Fatal(err)
	}
	baseSnapshot := base.Snapshot
	baseTarget := canonicalForCapabilityName(t, base, "Current screen")
	before, err := base.Routes.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: baseTarget})
	if err != nil {
		t.Fatal(err)
	}
	beforeCandidate := before.Candidate()

	overlaySource, err := LoadLegacyCenterCatalog(legacyOverlayCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	merged, err := ProjectLegacyCenterOverlay(base, overlaySource, "device-controller", now.Add(time.Minute), nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Snapshot.FabricID != baseSnapshot.FabricID || !reflect.DeepEqual(merged.Snapshot.Devices[:len(baseSnapshot.Devices)], baseSnapshot.Devices) {
		t.Fatal("overlay changed the base Fabric or existing Devices")
	}
	for index := range baseSnapshot.Capabilities {
		if !reflect.DeepEqual(merged.Snapshot.Capabilities[index], baseSnapshot.Capabilities[index]) {
			t.Fatal("overlay changed an existing Capability identity")
		}
	}
	after, err := merged.Routes.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: baseTarget})
	if err != nil || !reflect.DeepEqual(after.Candidate(), beforeCandidate) {
		t.Fatal("overlay changed an existing private route")
	}

	shellTarget := canonicalForCapabilityName(t, merged, "Overlay automation")
	vncTarget := canonicalForCapabilityName(t, merged, "Overlay desktop")
	externalTarget := canonicalForCapabilityName(t, merged, "Overlay screen application")
	if !merged.Shell.HasTarget(shellTarget) || !merged.Gateway.HasTarget(vncTarget) || !merged.External.HasTarget(externalTarget) {
		t.Fatal("overlay private Shell, Gateway, or External binding is missing")
	}
	acquired, err := merged.Routes.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: shellTarget})
	if err != nil || acquired.Candidate().Adapter != "lan" {
		t.Fatal("overlay private endpoint route is unavailable")
	}
	if got, want := (targetref.Reference{FabricID: baseSnapshot.FabricID, DeviceID: mappingKey("device", "overlay-device"), CapabilityID: mappingKey("capability", "overlay-device\x00overlay-shell")}).String(), shellTarget; got != want {
		t.Fatalf("overlay mapping identity is not stable: got %q want %q", got, want)
	}
}

func TestProjectLegacyCenterOverlayRejectsIdentityConflictWithoutChangingBase(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	base, err := ProjectLegacyCenterCandidate(source, "device-controller", now)
	if err != nil {
		t.Fatal(err)
	}
	before := base.Snapshot
	if _, err := ProjectLegacyCenterOverlay(base, source, "device-controller", now.Add(time.Minute), nil); err == nil {
		t.Fatal("overlay accepted a conflicting stable mapping identity")
	}
	if !reflect.DeepEqual(base.Snapshot, before) {
		t.Fatal("failed overlay mutated the base candidate")
	}
}

func TestProjectLegacyCenterOverlayExtendsExistingDeviceWithNewCapability(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	baseSource, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	base, err := ProjectLegacyCenterCandidate(baseSource, "device-controller", now)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"deviceId": "overlay-owner", "defaultRoute": "lan", "generatedAt": 2,
		"services": []map[string]any{{
			"id": "service-rdp-extension", "deviceId": "device-a", "deviceName": "Lab computer", "name": "Windows desktop", "kind": "rdp", "username": "operator",
			"aliyun": nil, "tailscale": nil, "webUrl": nil, "external": nil, "lan": map[string]any{"hosts": []string{"192.0.2.42"}, "port": 3389},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := LoadLegacyCenterCatalog(payload)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := ProjectLegacyCenterOverlay(base, overlay, "device-controller", now.Add(time.Minute), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Snapshot.Devices) != len(base.Snapshot.Devices) || len(merged.Snapshot.Capabilities) != len(base.Snapshot.Capabilities)+1 {
		t.Fatalf("merged snapshot = %d Devices, %d Capabilities", len(merged.Snapshot.Devices), len(merged.Snapshot.Capabilities))
	}
	target := canonicalForCapabilityName(t, merged, "Windows desktop")
	acquired, err := merged.Routes.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: target})
	if err != nil || acquired.Candidate().Port != 3389 {
		t.Fatalf("extended route is unavailable: candidate=%#v err=%v", acquired, err)
	}
}

func canonicalForCapabilityName(t *testing.T, candidate LegacyCandidate, displayName string) string {
	t.Helper()
	for _, capability := range candidate.Snapshot.Capabilities {
		if capability.DisplayName == displayName {
			return targetref.Reference{FabricID: candidate.Snapshot.FabricID, DeviceID: capability.DeviceID, CapabilityID: capability.ID}.String()
		}
	}
	t.Fatalf("Capability %q is missing", displayName)
	return ""
}

func legacyOverlayCatalog(t *testing.T) []byte {
	t.Helper()
	viewer := filepath.Join(t.TempDir(), "viewer.exe")
	if err := os.WriteFile(viewer, []byte("synthetic viewer"), 0o600); err != nil {
		t.Fatal(err)
	}
	services := []map[string]any{
		{
			"id": "overlay-shell", "deviceId": "overlay-device", "deviceName": "Overlay computer", "name": "Overlay automation", "kind": "ssh", "username": "overlay-operator",
			"aliyun": nil, "tailscale": nil, "webUrl": nil, "external": nil, "lan": map[string]any{"hosts": []string{"192.0.2.31"}, "port": 22},
		},
		{
			"id": "overlay-vnc", "deviceId": "overlay-device", "deviceName": "Overlay computer", "name": "Overlay desktop", "kind": "vnc", "username": nil,
			"aliyun":    map[string]any{"serverAddress": "relay.example.invalid", "serverPort": 7000, "proxyName": "overlay_proxy_01", "proxySecret": "overlay_secret_01", "gatewayPort": 5901},
			"tailscale": nil, "webUrl": nil, "external": nil, "lan": nil,
		},
		{
			"id": "overlay-external", "deviceId": "overlay-device", "deviceName": "Overlay computer", "name": "Overlay screen application", "kind": "external", "username": nil,
			"aliyun": nil, "tailscale": nil, "webUrl": "https://example.invalid/opaque", "external": map[string]any{"executable": viewer, "arguments": "", "uri": nil}, "lan": nil,
		},
	}
	encoded, err := json.Marshal(map[string]any{"deviceId": "overlay-owner", "defaultRoute": "gateway", "generatedAt": 1, "services": services})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
