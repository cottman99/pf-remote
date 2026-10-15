//go:build windows

package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedRDPLoginIsSealedPerLaunchAndRemovedOnSigningFailure(t *testing.T) {
	secret := strings.Join([]string{"example", "access", "secret"}, "-")
	o := RDPOptions{Endpoint: "192.0.2.10:3389", SessionSelector: "workspace-2", Username: "example-user", Password: secret}
	path, cleanup, err := writeRDPProfile(o)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	b, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(b), secret) || !strings.Contains(string(b), "password 51:b:") || !strings.Contains(string(b), "username:s:workspace-2\\example-user") {
		t.Fatal("native profile lost its private per-launch account binding")
	}
	root := t.TempDir()
	t.Setenv("TEMP", root)
	t.Setenv("TMP", root)
	err = (OSCommand{SignRDP: func(_ context.Context, p string) error {
		if filepath.Dir(filepath.Dir(p)) != root {
			t.Fatal("test profile is outside the isolated directory")
		}
		return errors.New("synthetic signing failure")
	}}).Run(context.Background(), Launch{Name: "must-not-launch", RDP: &o})
	entries, readErr := os.ReadDir(root)
	if err == nil || readErr != nil || len(entries) != 0 {
		t.Fatal("failed launch retained an encrypted credential file")
	}
}
