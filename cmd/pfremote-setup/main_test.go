package main

import (
	"path/filepath"
	"testing"
)

func TestInstallationRootUsesPerUserProgramsDirectory(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)

	got, err := installationRoot("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(local, "Programs", "PFRemote")
	if got != want {
		t.Fatalf("installationRoot() = %q, want %q", got, want)
	}
}

func TestInstallationRootRejectsCustomRootOutsideVerifier(t *testing.T) {
	t.Setenv("PFREMOTE_SETUP_TESTING", "")
	if _, err := installationRoot(t.TempDir()); err == nil {
		t.Fatal("installationRoot accepted a custom root outside the isolated verifier")
	}
}

func TestInstallationRootAllowsCustomRootForVerifier(t *testing.T) {
	t.Setenv("PFREMOTE_SETUP_TESTING", "true")
	want, err := filepath.Abs(filepath.Join(t.TempDir(), "candidate"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := installationRoot(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("installationRoot() = %q, want %q", got, want)
	}
}
