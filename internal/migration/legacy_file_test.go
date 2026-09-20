package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLegacyCenterFileReadsExactRegularFileWithoutWriting(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "catalog.json")
	if err := os.WriteFile(path, syntheticLegacyCenterCatalogWithPorts(), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLegacyCenterFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(loaded.Inventory.Devices) != 2 || len(loaded.Connections) != 3 {
		t.Fatalf("legacy file changed or loaded incompletely")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected files beside legacy source: %#v, %v", entries, err)
	}
}

func TestLoadLegacyCenterFileRejectsLinkAndDirectory(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "catalog.json")
	if err := os.WriteFile(path, syntheticLegacyCenterCatalogWithPorts(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLegacyCenterFile(directory); err == nil {
		t.Fatal("directory accepted as legacy catalog")
	}
	link := filepath.Join(directory, "catalog-link.json")
	if err := os.Symlink(path, link); err == nil {
		if _, err := LoadLegacyCenterFile(link); err == nil {
			t.Fatal("link accepted as legacy catalog")
		}
	}
}
