package migration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/route"
)

type resolvingTailscalePeer struct {
	address string
	nodeID  string
}

func (p resolvingTailscalePeer) ResolvePeer(_ context.Context, address string) (string, error) {
	if address == p.address {
		return p.nodeID, nil
	}
	return "", errors.New("peer unavailable")
}

func (p resolvingTailscalePeer) VerifyPeer(_ context.Context, address, nodeID string) error {
	if address == p.address && nodeID == p.nodeID {
		return nil
	}
	return errors.New("peer mismatch")
}

func TestProjectLegacyCenterCandidateProducesUsableSafeCatalog(t *testing.T) {
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	shellConnection := source.Connections["service-shell"]
	shellConnection.Username = "operator-name"
	source.Connections["service-shell"] = shellConnection
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.Snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	view := catalog.New(
		candidate.Snapshot.FabricID, candidate.Snapshot.Devices, candidate.Snapshot.Capabilities,
		candidate.Snapshot.Grants, "device-controller", now, func() time.Time { return now },
	)
	targets := view.List()
	if len(targets) != 3 {
		t.Fatalf("targets = %#v", targets)
	}
	var desktopState, shellState, vncState, shellCanonical string
	for _, target := range targets {
		switch target.Capability.DisplayName {
		case "Current screen":
			desktopState = target.Capability.State
			if target.Capability.DesktopProfile == nil || target.Capability.DesktopProfile.Protocol != "rdp" {
				t.Fatalf("desktop target = %#v", target)
			}
		case "Automation":
			shellState = target.Capability.State
			shellCanonical = target.Canonical
		case "Independent desktop":
			vncState = target.Capability.State
		}
	}
	if desktopState != "available" || shellState != "setup-required" || vncState != "setup-required" {
		t.Fatalf("states desktop=%q shell=%q vnc=%q", desktopState, shellState, vncState)
	}
	if shellCanonical == "" || !candidate.Shell.HasTarget(shellCanonical) {
		t.Fatalf("private Shell target binding missing: %q", shellCanonical)
	}

	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"host.example.invalid", "192.0.2.10", "192.0.2.20", "fixture-secret", "operator-name"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("candidate serialization contains %q: %s", forbidden, encoded)
		}
	}
}

func TestProjectLegacyCenterCandidateRejectsMissingConnectionMaterial(t *testing.T) {
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	delete(source.Connections, "service-shell")
	if _, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now()); err == nil {
		t.Fatal("candidate accepted missing private connection material")
	}
}

func TestLegacyRDPProfileSupportsLANAndGatewayAlongsideTailscale(t *testing.T) {
	connection := LegacyCenterConnection{Protocol: "rdp", Routes: []LegacyCenterRoute{
		{Adapter: "tailscale", Address: "192.0.2.20", Port: 3389, NodeID: "node-synthetic"},
		{Adapter: "lan", Address: "192.0.2.30", Port: 3389},
		{Adapter: "legacy-gateway", Address: "gateway.example.invalid", ControlPort: 7000, Port: 14110, ProxyName: "desktop_proxy_01", ProxySecret: strings.Repeat("s", 16)},
	}}
	profile, ready := legacyDesktopProfile(connection)
	if !ready || profile.Authentication != "windows-sso" {
		t.Fatalf("profile=%#v ready=%v", profile, ready)
	}
}

func TestInvalidLegacyFallbackDoesNotHideCatalog(t *testing.T) {
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	connection := source.Connections["service-desktop"]
	for index := range connection.Routes {
		connection.Routes[index].Address = ""
		connection.Routes[index].Port = 0
	}
	source.Connections["service-desktop"] = connection
	candidate, err := ProjectLegacyCenterCandidate(source, "device-controller", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range candidate.Snapshot.Capabilities {
		if capability.DisplayName == "Current screen" && capability.State != "setup-required" {
			t.Fatalf("invalid routed desktop state = %q", capability.State)
		}
	}
}

func TestResolvedTailscaleIdentityMakesVNCReadyAndReverified(t *testing.T) {
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	peer := resolvingTailscalePeer{address: "other.example.invalid", nodeID: "node-synthetic"}
	if got := ResolveLegacyTailscaleIdentities(context.Background(), &source, peer); got != 1 {
		t.Fatalf("resolved routes = %d", got)
	}
	candidate, err := ProjectLegacyCenterCandidateWithTailscale(source, "device-controller", time.Now(), peer)
	if err != nil {
		t.Fatal(err)
	}
	var vncCanonical string
	for _, capability := range candidate.Snapshot.Capabilities {
		if capability.DisplayName == "Independent desktop" {
			if capability.State != "available" || capability.DesktopProfile == nil || capability.DesktopProfile.Authentication != "legacy-vnc-password" || capability.DesktopProfile.RenderingEnvironment != "virtual" {
				t.Fatalf("vnc capability = %#v", capability)
			}
			vncCanonical = "pfremote://" + candidate.Snapshot.FabricID + "/devices/" + capability.DeviceID + "/capabilities/" + capability.ID
		}
	}
	if vncCanonical == "" {
		t.Fatal("VNC target missing")
	}
	if _, err := candidate.Routes.Provider("tailscale").Acquire(context.Background(), route.Request{CanonicalTarget: vncCanonical}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "node-synthetic") || strings.Contains(string(encoded), "other.example.invalid") {
		t.Fatalf("candidate leaked private Tailscale identity: %s", encoded)
	}
}
