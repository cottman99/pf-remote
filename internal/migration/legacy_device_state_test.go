package migration

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyDeviceStateMarksOneComputerOfflineWithoutChangingOthers(t *testing.T) {
	source, err := LoadLegacyCenterCatalog([]byte(`{"deviceId":"owner-device","defaultRoute":"gateway","generatedAt":1,"services":[{"id":"service-a","deviceId":"device-a","deviceName":"Computer A","name":"Screen","kind":"rdp","username":null,"aliyun":null,"tailscale":{"host":"a.example.invalid","port":3389},"webUrl":null,"external":null,"lan":null},{"id":"service-b","deviceId":"device-b","deviceName":"Computer B","name":"Screen","kind":"rdp","username":null,"aliyun":null,"tailscale":{"host":"b.example.invalid","port":3389},"webUrl":null,"external":null,"lan":null}]}`))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	offlineID := candidate.Snapshot.Devices[1].ID
	otherID := candidate.Snapshot.Devices[2].ID
	otherCapabilityState := ""
	for _, capability := range candidate.Snapshot.Capabilities {
		if capability.DeviceID == otherID {
			otherCapabilityState = capability.State
		}
	}
	updated, err := ApplyLegacyDeviceStateOverrides(candidate, LegacyDeviceStateOverrides{SchemaVersion: LegacyDeviceStateSchema, Devices: []LegacyDeviceStateOverride{{LegacyDeviceID: offlineID, State: "offline"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range updated.Snapshot.Devices {
		if device.ID == offlineID && device.State != "offline" {
			t.Fatal("selected Device did not become offline")
		}
		if device.ID == otherID && device.State != "online" {
			t.Fatal("unselected Device state changed")
		}
	}
	for _, capability := range updated.Snapshot.Capabilities {
		if capability.DeviceID == offlineID && capability.State != "setup-required" {
			t.Fatal("offline Device action remained enabled")
		}
		if capability.DeviceID == otherID && capability.State != otherCapabilityState {
			t.Fatal("online Device action changed")
		}
	}
}

func TestLoadLegacyDeviceStateRejectsUnknownState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"pfremote.legacy-device-state/v1","devices":[{"legacy_device_id":"device-a","state":"online"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLegacyDeviceStateOverrides(path); err == nil {
		t.Fatal("expected unsafe online override to fail")
	}
}
