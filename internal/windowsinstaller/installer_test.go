package windowsinstaller

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallUpgradeRollbackAndUninstall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "PF Remote")
	firstArchive, firstManifest, firstArchiveHash, firstManifestHash := releaseFixture(t, "0.1.0", "first")
	first, err := Install(root, firstArchive, firstManifest, firstArchiveHash, firstManifestHash)
	if err != nil || first.Current != "0.1.0" || first.Previous != "" {
		t.Fatalf("first install = %#v, %v", first, err)
	}
	reinstalled, err := Install(root, firstArchive, firstManifest, firstArchiveHash, firstManifestHash)
	if err != nil || reinstalled.Current != "0.1.0" || reinstalled.Previous != "" {
		t.Fatalf("same-version reinstall = %#v, %v", reinstalled, err)
	}
	secondArchive, secondManifest, secondArchiveHash, secondManifestHash := releaseFixture(t, "0.2.0", "second")
	second, err := Install(root, secondArchive, secondManifest, secondArchiveHash, secondManifestHash)
	if err != nil || second.Current != "0.2.0" || second.Previous != "0.1.0" {
		t.Fatalf("upgrade = %#v, %v", second, err)
	}
	rolledBack, err := Rollback(root)
	if err != nil || rolledBack.Current != "0.1.0" || rolledBack.Previous != "0.2.0" {
		t.Fatalf("rollback = %#v, %v", rolledBack, err)
	}
	input, err := os.ReadFile(rolledBack.AppPath)
	if err != nil || string(input) != "first" {
		t.Fatalf("rolled back app = %q, %v", input, err)
	}
	if err := Uninstall(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("marked installation remained after uninstall")
	}
}

func TestInstallRejectsTamperingAndUninstallRejectsUnmarkedRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "PF Remote")
	archive, manifest, archiveHash, manifestHash := releaseFixture(t, "0.1.0", "content")
	if err := os.WriteFile(archive, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(root, archive, manifest, archiveHash, manifestHash); err == nil {
		t.Fatal("tampered payload installed")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(root); err == nil {
		t.Fatal("unmarked directory was removed")
	}
}

func TestInstallKeepsOnlyCurrentAndRollbackVersions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "PF Remote")
	for index, version := range []string{"0.1.0", "0.2.0", "0.3.0"} {
		archive, manifest, archiveHash, manifestHash := releaseFixture(t, version, version)
		if _, err := Install(root, archive, manifest, archiveHash, manifestHash); err != nil {
			t.Fatalf("install %d: %v", index, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "0.1.0")); !os.IsNotExist(err) {
		t.Fatalf("obsolete version was retained: %v", err)
	}
	for _, version := range []string{"0.2.0", "0.3.0"} {
		if _, err := os.Stat(filepath.Join(root, "versions", version)); err != nil {
			t.Fatalf("retained version %s is missing: %v", version, err)
		}
	}
}

func TestInstallDefersCleanupOfAVersionStillUsedByAnAgentHost(t *testing.T) {
	root := filepath.Join(t.TempDir(), "PF Remote")
	for _, version := range []string{"0.1.0", "0.2.0"} {
		archive, manifest, archiveHash, manifestHash := releaseFixture(t, version, version)
		if _, err := Install(root, archive, manifest, archiveHash, manifestHash); err != nil {
			t.Fatal(err)
		}
	}
	originalRemove := removeVersion
	t.Cleanup(func() { removeVersion = originalRemove })
	removeVersion = func(path string) error {
		if filepath.Base(path) == "0.1.0" {
			return errors.New("version is in use")
		}
		return originalRemove(path)
	}
	thirdArchive, thirdManifest, thirdArchiveHash, thirdManifestHash := releaseFixture(t, "0.3.0", "third")
	third, err := Install(root, thirdArchive, thirdManifest, thirdArchiveHash, thirdManifestHash)
	if err != nil || third.Current != "0.3.0" || third.Previous != "0.2.0" {
		t.Fatalf("upgrade with in-use obsolete version = %#v, %v", third, err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "0.1.0")); err != nil {
		t.Fatalf("in-use version was not deferred: %v", err)
	}

	removeVersion = originalRemove
	fourthArchive, fourthManifest, fourthArchiveHash, fourthManifestHash := releaseFixture(t, "0.4.0", "fourth")
	if _, err := Install(root, fourthArchive, fourthManifest, fourthArchiveHash, fourthManifestHash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "0.1.0")); !os.IsNotExist(err) {
		t.Fatalf("deferred version was not cleaned on a later upgrade: %v", err)
	}
}

func releaseFixture(t *testing.T, version, content string) (string, string, string, string) {
	t.Helper()
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "payload.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(archiveFile)
	entry, err := archive.Create("PFRemoteCenter.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if archive.Close() != nil || archiveFile.Close() != nil {
		t.Fatal("close fixture archive")
	}
	hash := sha256.Sum256([]byte(content))
	manifest := Manifest{
		SchemaVersion: ManifestSchema, Product: "PF Remote", Version: version, Architecture: "x64",
		Files: []ManifestFile{{Path: "PFRemoteCenter.exe", Size: int64(len(content)), SHA256: hex.EncodeToString(hash[:])}},
	}
	manifestBytes, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(directory, "release-manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	archiveHash, _ := HashFile(archivePath)
	manifestHash, _ := HashFile(manifestPath)
	return archivePath, manifestPath, archiveHash, manifestHash
}
