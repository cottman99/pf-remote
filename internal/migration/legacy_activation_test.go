package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableLegacyCenterWritesOnlySafeIdempotentMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "legacy-compatibility-v1.json")
	if err := EnableLegacyCenter(path); err != nil {
		t.Fatal(err)
	}
	if err := EnableLegacyCenter(path); err != nil {
		t.Fatal(err)
	}
	active, err := LegacyCenterEnabled(path)
	if err != nil || !active {
		t.Fatalf("active=%v err=%v", active, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ProgramData", "catalog.json", "address", "credential", "password"} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("activation marker contains %q: %s", forbidden, content)
		}
	}
}

func TestDisableAndReenableLegacyCenterAreReversible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "legacy-compatibility-v1.json")
	if err := EnableLegacyCenter(path); err != nil {
		t.Fatal(err)
	}
	if err := DisableLegacyCenter(path); err != nil {
		t.Fatal(err)
	}
	if active, err := LegacyCenterEnabled(path); err != nil || active {
		t.Fatalf("disabled active=%v err=%v", active, err)
	}
	if err := DisableLegacyCenter(path); err != nil {
		t.Fatal(err)
	}
	if err := EnableLegacyCenter(path); err != nil {
		t.Fatal(err)
	}
	if active, err := LegacyCenterEnabled(path); err != nil || !active {
		t.Fatalf("reenabled active=%v err=%v", active, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("activation replacement residue = %#v", entries)
	}
}

func TestEnableLegacyCenterRejectsExistingInvalidState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-compatibility-v1.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"wrong","source":"windows-center"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnableLegacyCenter(path); err == nil {
		t.Fatal("invalid existing activation was overwritten")
	}
	if active, err := LegacyCenterEnabled(path); err == nil || active {
		t.Fatalf("invalid activation active=%v err=%v", active, err)
	}
}

func TestLegacyActivationWithoutEnabledFieldRemainsActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-compatibility-v1.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"pfremote.legacy-compatibility-activation/v1","source":"windows-center"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if active, err := LegacyCenterEnabled(path); err != nil || !active {
		t.Fatalf("legacy active=%v err=%v", active, err)
	}
}
