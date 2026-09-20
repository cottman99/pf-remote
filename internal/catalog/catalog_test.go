package catalog

import (
	"testing"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestResolveCanonicalCurrentAndHistoricalAlias(t *testing.T) {
	c := Synthetic()
	canonical := "pfremote://fabric-demo/devices/device-compute/capabilities/shell-main"
	for _, input := range []string{canonical, "compute-node/shell", "lab-node/shell"} {
		target, err := c.Resolve(input)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", input, err)
		}
		if target.Canonical != canonical {
			t.Fatalf("Resolve(%q) = %q", input, target.Canonical)
		}
	}
}

func TestCachedAuthorization_ExpiresAtSevenDayBoundary(t *testing.T) {
	capturedAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	now := capturedAt.Add(DefaultAuthorizationTTL - time.Second)
	fixture := Synthetic()
	fixture.CapturedAt = capturedAt
	fixture.Now = func() time.Time { return now }
	fixture.SubjectDeviceID = "device-reader"
	addSubjectDevice(&fixture, "active")
	fixture.Grants = []contracts.Grant{{
		ID: "grant-shell", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active",
	}}

	targets := fixture.List()
	if len(targets) != 1 || targets[0].Authorization.Status != "active" || targets[0].Authorization.RemainingSeconds != 1 {
		t.Fatalf("authorization before expiry = %#v", targets)
	}
	now = capturedAt.Add(DefaultAuthorizationTTL)
	if targets := fixture.List(); len(targets) != 0 {
		t.Fatalf("targets at expiry boundary = %#v", targets)
	}
	authorization := fixture.Authorization()
	if authorization.Status != "expired" || authorization.RemainingSeconds != 0 || !authorization.ValidUntil.Equal(now) {
		t.Fatalf("catalog authorization at expiry = %#v", authorization)
	}
	if _, err := fixture.Resolve("compute-node/shell"); err == nil {
		t.Fatal("expired target alias resolved")
	}
	if _, err := fixture.Resolve("pfremote://fabric-demo/devices/device-compute/capabilities/shell-main"); err == nil {
		t.Fatal("expired canonical target resolved")
	}
}

func TestGrantExpiry_CannotExtendSnapshotAndShortestBoundaryWins(t *testing.T) {
	capturedAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	now := capturedAt.Add(time.Hour)
	shortExpiry := capturedAt.Add(2 * time.Hour)
	longExpiry := capturedAt.Add(30 * 24 * time.Hour)
	fixture := Synthetic()
	fixture.CapturedAt = capturedAt
	fixture.Now = func() time.Time { return now }
	fixture.SubjectDeviceID = "device-reader"
	addSubjectDevice(&fixture, "active")
	fixture.Grants = []contracts.Grant{
		{ID: "grant-short", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active", ValidUntil: &shortExpiry},
		{ID: "grant-long", SubjectDeviceID: "device-reader", CapabilityID: "desktop-main", State: "active", ValidUntil: &longExpiry},
	}
	targets := fixture.List()
	if len(targets) != 2 {
		t.Fatalf("targets = %#v", targets)
	}
	for _, target := range targets {
		switch target.Capability.ID {
		case "shell-main":
			if !target.Authorization.ValidUntil.Equal(shortExpiry) {
				t.Fatalf("short Grant validity = %s", target.Authorization.ValidUntil)
			}
		case "desktop-main":
			want := capturedAt.Add(DefaultAuthorizationTTL)
			if !target.Authorization.ValidUntil.Equal(want) {
				t.Fatalf("long Grant validity = %s, want snapshot boundary %s", target.Authorization.ValidUntil, want)
			}
		}
	}
}

func TestMultipleActiveGrants_UseLatestStillValidAuthority(t *testing.T) {
	capturedAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	now := capturedAt.Add(3 * time.Hour)
	expired := capturedAt.Add(2 * time.Hour)
	valid := capturedAt.Add(4 * time.Hour)
	fixture := Synthetic()
	fixture.CapturedAt = capturedAt
	fixture.Now = func() time.Time { return now }
	fixture.SubjectDeviceID = "device-reader"
	addSubjectDevice(&fixture, "active")
	fixture.Grants = []contracts.Grant{
		{ID: "grant-expired", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active", ValidUntil: &expired},
		{ID: "grant-valid", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active", ValidUntil: &valid},
	}
	targets := fixture.List()
	if len(targets) != 1 || !targets[0].Authorization.ValidUntil.Equal(valid) {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestList_WithSubject_ReturnsOnlyGrantedCapabilities(t *testing.T) {
	fixture := Synthetic()
	fixture.SubjectDeviceID = "device-reader"
	addSubjectDevice(&fixture, "active")
	fixture.Grants = []contracts.Grant{
		{ID: "grant-shell", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active"},
		{ID: "grant-revoked", SubjectDeviceID: "device-reader", CapabilityID: "desktop-main", State: "revoked"},
	}
	targets := fixture.List()
	if len(targets) != 1 || targets[0].Capability.ID != "shell-main" || !targets[0].Granted {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestList_UnauthorizedSubject_CannotEnumerateTargets(t *testing.T) {
	fixture := Synthetic()
	fixture.SubjectDeviceID = "device-unauthorized"
	if targets := fixture.List(); len(targets) != 0 {
		t.Fatalf("unauthorized targets = %#v", targets)
	}
}

func TestRevokedSubjectCannotUseOtherwiseActiveGrant(t *testing.T) {
	fixture := Synthetic()
	fixture.SubjectDeviceID = "device-reader"
	addSubjectDevice(&fixture, "revoked")
	fixture.Grants = []contracts.Grant{{
		ID: "grant-shell", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active",
	}}
	if targets := fixture.List(); len(targets) != 0 {
		t.Fatalf("revoked subject targets = %#v", targets)
	}
}

func TestRevokedTargetDeviceCannotBeListedOrResolved(t *testing.T) {
	fixture := Synthetic()
	fixture.SubjectDeviceID = "device-reader"
	addSubjectDevice(&fixture, "online")
	fixture.Grants = []contracts.Grant{{
		ID: "grant-shell", SubjectDeviceID: "device-reader", CapabilityID: "shell-main", State: "active",
	}}
	for index := range fixture.Devices {
		if fixture.Devices[index].ID == "device-compute" {
			fixture.Devices[index].State = "revoked"
		}
	}
	if targets := fixture.List(); len(targets) != 0 {
		t.Fatalf("revoked target Device remained visible: %#v", targets)
	}
	for _, reference := range []string{
		"compute-node/shell",
		"pfremote://fabric-demo/devices/device-compute/capabilities/shell-main",
	} {
		if _, err := fixture.Resolve(reference); err == nil {
			t.Fatalf("revoked target resolved from %q", reference)
		}
	}
}

func TestListIsStable(t *testing.T) {
	targets := Synthetic().List()
	if len(targets) != 4 {
		t.Fatalf("len = %d, want 4", len(targets))
	}
	for i := 1; i < len(targets); i++ {
		if targets[i-1].Alias > targets[i].Alias {
			t.Fatalf("targets are not sorted: %q > %q", targets[i-1].Alias, targets[i].Alias)
		}
	}
}

func addSubjectDevice(catalog *Catalog, state string) {
	catalog.Devices = append(catalog.Devices, contracts.Device{
		ID: "device-reader", Alias: "reader", DisplayName: "Reader", State: state,
	})
}
