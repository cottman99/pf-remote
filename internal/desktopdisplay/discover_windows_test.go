package desktopdisplay

import (
	"os"
	"testing"
)

func TestNativeMonitorDiscoveryOnIsolatedDesktop(t *testing.T) {
	if os.Getenv("PFREMOTE_TEST_NATIVE_DISPLAYS") != "1" {
		t.Skip("requires signed-in Windows session")
	}
	layout, err := Discover()
	if err != nil || len(layout.Monitors) == 0 {
		t.Fatal("native read-only monitor discovery failed", err)
	}
	t.Logf("native monitor count=%d, valid layout=%t", len(layout.Monitors), len(layout.ID) == 64)
}
