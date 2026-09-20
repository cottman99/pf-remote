package state

import (
	"testing"
	"time"
)

func TestSnapshotRejectsMismatchedDesktopAuthentication(t *testing.T) {
	for _, authentication := range []string{"x509-route-grant", "legacy-private-executor"} {
		snapshot := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
		for index := range snapshot.Capabilities {
			if snapshot.Capabilities[index].DesktopProfile != nil {
				snapshot.Capabilities[index].DesktopProfile.Authentication = authentication
			}
		}
		if err := snapshot.Validate(); err == nil {
			t.Fatalf("expected RDP with %q authentication to fail", authentication)
		}
	}
}

func TestSnapshotKeepsLegacyDesktopProfileReadableButNotLaunchable(t *testing.T) {
	snapshot := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	for index := range snapshot.Capabilities {
		if snapshot.Capabilities[index].DesktopProfile != nil {
			snapshot.Capabilities[index].DesktopProfile.Authentication = ""
		}
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotAcceptsTailscaleBoundRDPProfile(t *testing.T) {
	snapshot := SyntheticSnapshot("device-owner", time.Now().UTC())
	for index := range snapshot.Capabilities {
		if snapshot.Capabilities[index].DesktopProfile != nil && snapshot.Capabilities[index].DesktopProfile.Protocol == "rdp" {
			snapshot.Capabilities[index].DesktopProfile.Authentication = "tailscale-device"
		}
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
}
