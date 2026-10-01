package migration

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSharedWorkspaceKeepsOriginalIdentityAndRoutes(t *testing.T) {
	input := syntheticLegacyCenterCatalogWithPorts()
	before, err := LoadLegacyCenterCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	settings := []byte(`{"schema_version":"pfremote.shared-desktops/v1","workspaces":[{"service_id":"service-desktop","display":2,"display_name":"Workspace 2 with sound"}]}`)
	merged, err := mergeSharedDesktopCatalog(input, settings)
	if err != nil {
		t.Fatal(err)
	}
	after, err := LoadLegacyCenterCatalog(merged)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Connections["service-desktop"], after.Connections["service-desktop"]) {
		t.Fatal("original service changed")
	}
	shared := after.Connections["service-desktop-shared-2"]
	if shared.SessionSelector != "pfremote-shared-2" || !reflect.DeepEqual(shared.Routes, before.Connections["service-desktop"].Routes) {
		t.Fatal("workspace inherited another route")
	}
	if _, err := mergeSharedDesktopCatalog(input, []byte(`{"schema_version":"pfremote.shared-desktops/v1","workspaces":[{"service_id":"missing","display":2,"display_name":"Missing"}]}`)); err == nil {
		t.Fatal("missing base accepted")
	}
}

func TestSidecarPreservesFabricAndExistingCapabilitiesAcrossRollback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog.json")
	if err := os.WriteFile(path, syntheticLegacyCenterCatalogWithPorts(), 0o600); err != nil {
		t.Fatal(err)
	}
	project := func() LegacyCandidate {
		source, err := LoadLegacyCenterFile(path)
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return candidate
	}
	before := project()
	sidecar := filepath.Join(root, "shared-desktops-v1.json")
	if err := os.WriteFile(sidecar, []byte(`{"schema_version":"pfremote.shared-desktops/v1","workspaces":[{"service_id":"service-desktop","display":2,"display_name":"Workspace 2 with sound"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	after := project()
	if before.Snapshot.FabricID != after.Snapshot.FabricID {
		t.Fatal("adding a workspace changed Fabric identity")
	}
	for _, original := range before.Snapshot.Capabilities {
		found := false
		for _, current := range after.Snapshot.Capabilities {
			if original.ID == current.ID {
				found = reflect.DeepEqual(original, current)
				break
			}
		}
		if !found {
			t.Fatal("existing capability identity or profile changed")
		}
	}
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	rollback := project()
	if before.Snapshot.FabricID != rollback.Snapshot.FabricID || !reflect.DeepEqual(before.Snapshot.Capabilities, rollback.Snapshot.Capabilities) {
		t.Fatal("rollback changed existing capabilities")
	}
}
