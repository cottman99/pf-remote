package migration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

// This opt-in test proves the read-only legacy Alibaba Cloud route against one
// already-online Linux VNC capability. It never prints private catalog or route
// material and never runs in normal repository checks.
func TestPrivateLegacyGatewayVisitorForOnlineLinuxVNC(t *testing.T) {
	if !strings.EqualFold(os.Getenv("PFREMOTE_PRIVATE_GATEWAY_INTEGRATION"), "true") {
		t.Skip("private Gateway integration is opt-in")
	}
	catalogPath := os.Getenv("PFREMOTE_LEGACY_CENTER_CATALOG")
	managedConfigPath := os.Getenv("PFREMOTE_LEGACY_MANAGED_FRP_CONFIG")
	if catalogPath == "" || managedConfigPath == "" {
		t.Fatal("private integration paths are not configured")
	}
	source, err := LoadLegacyCenterFile(catalogPath)
	if err != nil {
		t.Fatal("private catalog could not be loaded")
	}
	candidate, err := ProjectLegacyCenterCandidate(source, "device-private-integration", time.Now().UTC())
	if err != nil {
		t.Fatal("private candidate could not be projected")
	}
	var target string
	for _, capability := range candidate.Snapshot.Capabilities {
		if capability.Kind != contracts.CapabilityDesktop || capability.DesktopProfile == nil || capability.DesktopProfile.Protocol != "vnc" {
			continue
		}
		canonical := targetref.Reference{FabricID: candidate.Snapshot.FabricID, DeviceID: capability.DeviceID, CapabilityID: capability.ID}.String()
		if candidate.Gateway.HasTarget(canonical) {
			target = canonical
			break
		}
	}
	if target == "" {
		t.Fatal("no online Linux VNC Gateway route is available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	provider := route.ReachableProvider{Provider: candidate.Gateway.ManagedProvider(managedConfigPath)}
	acquired, err := provider.Acquire(ctx, route.Request{
		SubjectDeviceID: "device-private-integration", CanonicalTarget: target,
		AuthorizationExpiry: time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal("private Gateway visitor did not become reachable")
	}
	if acquired.Candidate().Adapter != "frp" {
		_ = acquired.Close()
		t.Fatal("private Gateway route used the wrong adapter")
	}
	if err := acquired.Close(); err != nil {
		t.Fatal("private Gateway visitor could not be released")
	}
}
