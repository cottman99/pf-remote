package migration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExportLegacyCenterCatalog_RedactsInfrastructureAndIsDeterministic(t *testing.T) {
	first := syntheticLegacyCenterCatalog(false)
	second := syntheticLegacyCenterCatalog(true)
	inventoryA, err := ExportLegacyCenterCatalog(first)
	if err != nil {
		t.Fatal(err)
	}
	inventoryB, err := ExportLegacyCenterCatalog(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inventoryA, inventoryB) {
		t.Fatalf("inventories differ:\n%#v\n%#v", inventoryA, inventoryB)
	}
	if len(inventoryA.Devices) != 2 || len(inventoryA.Devices[0].Capabilities) != 2 {
		t.Fatalf("inventory = %#v", inventoryA)
	}
	if inventoryA.Devices[0].Capabilities[0].Kind != "desktop" || inventoryA.Devices[0].Capabilities[0].PathCount != 3 {
		t.Fatalf("desktop = %#v", inventoryA.Devices[0].Capabilities[0])
	}
	encoded, err := json.Marshal(inventoryA)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"192.0.2.10", "example.invalid", "fixture-secret", "operator-name", "gateway-a"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("redacted inventory contains %q: %s", forbidden, encoded)
		}
	}
}

func TestLoadLegacyCenterCatalog_RetainsPrivateRoutesOnlyInMemory(t *testing.T) {
	compatibility, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalog(false))
	if err != nil {
		t.Fatal(err)
	}
	connection := compatibility.Connections["service-desktop"]
	if connection.Username != "operator-name" || len(connection.Routes) != 3 {
		t.Fatalf("connection = %#v", connection)
	}
	if connection.Routes[0].Address != "192.0.2.10" || connection.Routes[0].ProxySecret != "fixture-secret" {
		t.Fatalf("gateway route = %#v", connection.Routes[0])
	}
	encoded, err := json.Marshal(compatibility)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"192.0.2.10", "fixture-secret", "operator-name", "example.invalid"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("compatibility serialization contains %q: %s", forbidden, encoded)
		}
	}
}

func TestExportLegacyCenterCatalog_RejectsUnsafeOrAmbiguousInput(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"deviceId":"owner-device","defaultRoute":"gateway","generatedAt":1,"unknown":"value","services":[]}`),
		[]byte(`{"deviceId":"owner-device","defaultRoute":"gateway","generatedAt":1,"services":[{"id":"service-a","deviceId":"device-a","deviceName":"Computer","name":"Web","kind":"web","username":null,"aliyun":{},"tailscale":null,"webUrl":null,"external":null,"lan":null}]}`),
		[]byte(`{"deviceId":"owner-device","defaultRoute":"gateway","generatedAt":1,"services":[{"id":"service-a","deviceId":"device-a","deviceName":"Computer","name":"Shell","kind":"ssh","username":null,"aliyun":null,"tailscale":null,"webUrl":null,"external":null,"lan":null}]}`),
	}
	for _, input := range tests {
		if _, err := ExportLegacyCenterCatalog(input); err == nil {
			t.Fatalf("unsafe catalog was accepted: %s", input)
		}
	}
}

func syntheticLegacyCenterCatalog(reverse bool) []byte {
	services := []map[string]any{
		{"id": "service-desktop", "deviceId": "device-a", "deviceName": "Lab computer", "name": "Current screen", "kind": "rdp", "username": "operator-name", "aliyun": map[string]any{"serverAddress": "192.0.2.10", "proxySecret": "fixture-secret"}, "tailscale": map[string]any{"host": "host.example.invalid", "port": 3389}, "webUrl": nil, "external": nil, "lan": map[string]any{"hosts": []string{"192.0.2.20"}, "port": 3389}},
		{"id": "service-shell", "deviceId": "device-a", "deviceName": "Lab computer", "name": "Automation", "kind": "ssh", "username": nil, "aliyun": map[string]any{"proxyName": "gateway-a"}, "tailscale": nil, "webUrl": nil, "external": nil, "lan": nil},
		{"id": "service-vnc", "deviceId": "device-b", "deviceName": "Office computer", "name": "Independent desktop", "kind": "vnc", "username": nil, "aliyun": nil, "tailscale": map[string]any{"host": "other.example.invalid", "port": 5900}, "webUrl": nil, "external": nil, "lan": nil},
	}
	if reverse {
		services[0], services[2] = services[2], services[0]
	}
	value := map[string]any{"deviceId": "owner-device", "defaultRoute": "gateway", "generatedAt": 1, "services": services}
	encoded, _ := json.Marshal(value)
	return encoded
}
