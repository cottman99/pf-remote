package capabilitysync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestLoadClaimsCacheAcceptsVersionedDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claims.json")
	response := enrollment.ShellCapabilityListResponse{
		SchemaVersion:    enrollment.SchemaVersion,
		FabricID:         "fabric-cache",
		DirectoryVersion: 4,
		Capabilities: []contracts.ShellCapabilityClaim{{
			DeviceID:        "device-one",
			DevicePublicKey: "public-key",
			Binding:         contracts.SSHCapabilityBinding{CapabilityID: "shell-one"},
		}},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadClaimsCache(path)
	if err != nil || loaded.FabricID != response.FabricID || loaded.DirectoryVersion != response.DirectoryVersion || len(loaded.Capabilities) != 1 {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
}

func TestLoadClaimsCacheRejectsUnknownAndEmptyDocuments(t *testing.T) {
	for name, payload := range map[string]string{
		"unknown": `{"schema_version":"pfremote.enrollment/v1","fabric_id":"fabric-cache","directory_version":1,"capabilities":[],"extra":true}`,
		"empty":   `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "claims.json")
			if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadClaimsCache(path); err == nil {
				t.Fatal("invalid claims cache was accepted")
			}
		})
	}
}
