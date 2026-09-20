package migration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type captureExternalCommand struct {
	executable string
	calls      int
}

func (c *captureExternalCommand) Run(_ context.Context, executable string) error {
	c.executable = executable
	c.calls++
	return nil
}

func TestExternalLegacyDesktopIsTargetBoundPrivateAndLaunchable(t *testing.T) {
	viewer := filepath.Join(t.TempDir(), "viewer.exe")
	if err := os.WriteFile(viewer, []byte("synthetic executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := externalLegacyCatalog(t, viewer, "")
	source, err := LoadLegacyCenterCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var canonical string
	for _, capability := range candidate.Snapshot.Capabilities {
		if capability.DisplayName == "Existing screen application" {
			if capability.State != "available" || capability.DesktopProfile == nil || capability.DesktopProfile.Protocol != "external" {
				t.Fatalf("external capability = %#v", capability)
			}
			canonical = "pfremote://" + candidate.Snapshot.FabricID + "/devices/" + capability.DeviceID + "/capabilities/" + capability.ID
		}
	}
	if canonical == "" || !candidate.External.HasTarget(canonical) || candidate.External.HasTarget(canonical+"-other") {
		t.Fatalf("external target binding missing: %q", canonical)
	}
	promoted := candidate.External.WithoutTargets([]string{canonical})
	if promoted.HasTarget(canonical) || !candidate.External.HasTarget(canonical) {
		t.Fatal("native Desktop promotion changed the source binding or kept its external override")
	}
	capture := &captureExternalCommand{}
	candidate.External.runner = capture
	candidate.External.goos = "windows"
	result, err := candidate.External.Run(context.Background(), canonical)
	if err != nil || result.Protocol != "external" || capture.calls != 1 || capture.executable != viewer {
		t.Fatalf("result=%#v err=%v capture=%#v", result, err, capture)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), viewer) || strings.Contains(string(encoded), "synthetic executable") {
		t.Fatalf("candidate leaked external launch material: %s", encoded)
	}
}

func TestUnsafeExternalActionStaysVisibleWithoutBlockingOtherDesktop(t *testing.T) {
	viewer := filepath.Join(t.TempDir(), "viewer.exe")
	if err := os.WriteFile(viewer, []byte("synthetic executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := LoadLegacyCenterCatalog(externalLegacyCatalog(t, viewer, "--unreviewed-argument"))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	states := make(map[string]string)
	for _, capability := range candidate.Snapshot.Capabilities {
		states[capability.DisplayName] = capability.State
	}
	if states["Existing screen application"] != "setup-required" || states["Existing virtual desktop"] != "available" {
		t.Fatalf("capability states = %#v", states)
	}
}

func externalLegacyCatalog(t *testing.T, viewer, arguments string) []byte {
	t.Helper()
	value := map[string]any{
		"deviceId": "owner-device", "defaultRoute": "gateway", "generatedAt": 1,
		"services": []map[string]any{
			{
				"id": "service-external", "deviceId": "device-a", "deviceName": "Existing computer",
				"name": "Existing screen application", "kind": "external", "username": nil,
				"aliyun": nil, "tailscale": nil, "webUrl": "https://example.invalid/opaque",
				"external": map[string]any{"executable": viewer, "arguments": arguments, "uri": nil}, "lan": nil,
			},
			{
				"id": "service-rdp", "deviceId": "device-a", "deviceName": "Existing computer",
				"name": "Existing virtual desktop", "kind": "rdp", "username": nil,
				"aliyun": nil, "tailscale": map[string]any{"host": "host.example.invalid", "port": 3389},
				"webUrl": nil, "external": nil, "lan": map[string]any{"hosts": []string{"192.0.2.20"}, "port": 3389},
			},
		},
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
