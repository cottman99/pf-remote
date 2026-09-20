package migration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseAndPlan_IsDeterministicAndCollapsesConnectionPaths(t *testing.T) {
	first := syntheticInventory(false)
	second := syntheticInventory(true)
	planA, err := ParseAndPlan(first)
	if err != nil {
		t.Fatal(err)
	}
	planB, err := ParseAndPlan(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(planA, planB) {
		t.Fatalf("plans differ:\n%#v\n%#v", planA, planB)
	}
	if len(planA.Devices) != 2 || planA.Devices[0].Alias != "lab-computer" || planA.Devices[1].Alias != "lab-computer-2" {
		t.Fatalf("Device aliases = %#v", planA.Devices)
	}
	if len(planA.Warnings) != 1 || !strings.Contains(planA.Warnings[0], "disambiguated") {
		t.Fatalf("warnings = %#v", planA.Warnings)
	}
	capability := planA.Devices[0].Capabilities[0]
	if capability.PathCount != 3 || capability.Alias != "desktop" {
		t.Fatalf("planned Capability = %#v", capability)
	}
	encoded, err := json.Marshal(planA)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"address", "hostname", "port", "password", "token", "route"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("plan contains forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func TestParseAndPlan_RejectsUnknownSecretAndRouteFields(t *testing.T) {
	for _, field := range []string{`"password":"secret",`, `"address":"192.0.2.1",`, `"routes":["gateway-a"],`} {
		input := []byte(`{"schema_version":"pfremote.legacy-inventory/v1","devices":[{` + field + `"legacy_ref":"legacy-device-a","name":"computer","display_name":"Computer","capabilities":[{"legacy_ref":"legacy-capability-a","name":"shell","display_name":"Shell","kind":"shell","path_count":1}]}]}`)
		if _, err := ParseAndPlan(input); err == nil {
			t.Fatalf("unknown field was accepted: %s", field)
		}
	}
}

func TestParseAndPlan_RejectsDanglingGrantAndOversizedInput(t *testing.T) {
	input := []byte(`{"schema_version":"pfremote.legacy-inventory/v1","devices":[{"legacy_ref":"legacy-device-a","name":"computer","display_name":"Computer","capabilities":[{"legacy_ref":"legacy-capability-a","name":"shell","display_name":"Shell","kind":"shell","path_count":1}]}],"grants":[{"legacy_ref":"legacy-grant-a","subject_device_ref":"missing-device","capability_ref":"legacy-capability-a","state":"active"}]}`)
	if _, err := ParseAndPlan(input); err == nil {
		t.Fatal("dangling Grant was accepted")
	}
	if _, err := ParseAndPlan(make([]byte, MaxInputBytes+1)); err == nil {
		t.Fatal("oversized inventory was accepted")
	}
}

func syntheticInventory(reverse bool) []byte {
	devices := []map[string]any{
		{"legacy_ref": "legacy-device-b", "name": "Lab Computer", "display_name": "Lab computer B", "capabilities": []map[string]any{{"legacy_ref": "legacy-capability-b", "name": "Shell", "display_name": "Automation", "kind": "shell", "path_count": 1}}},
		{"legacy_ref": "legacy-device-a", "name": "Lab Computer", "display_name": "Lab computer A", "capabilities": []map[string]any{{"legacy_ref": "legacy-capability-a", "name": "Desktop", "display_name": "Current screen", "kind": "desktop", "path_count": 3}}},
	}
	if reverse {
		devices[0], devices[1] = devices[1], devices[0]
	}
	inventory := map[string]any{
		"schema_version": InventorySchema,
		"devices":        devices,
		"grants":         []map[string]any{{"legacy_ref": "legacy-grant-a", "subject_device_ref": "legacy-device-b", "capability_ref": "legacy-capability-a", "state": "active"}},
	}
	encoded, _ := json.Marshal(inventory)
	return encoded
}
