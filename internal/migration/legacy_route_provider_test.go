package migration

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
)

type fixedTailscalePeer struct {
	address string
	nodeID  string
}

type recoveringTailscalePeer struct {
	available bool
	nodeID    string
}

func (p *recoveringTailscalePeer) ResolvePeer(context.Context, string) (string, error) {
	if !p.available {
		return "", errors.New("offline")
	}
	return p.nodeID, nil
}

func (p *recoveringTailscalePeer) VerifyPeer(_ context.Context, _ string, id string) error {
	if !p.available || id != p.nodeID {
		return errors.New("identity unavailable")
	}
	return nil
}

func TestCachedOfflineRouteRecoversWithoutRebuildingCatalog(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		t.Run(map[bool]string{false: "base", true: "overlay"}[overlay], func(t *testing.T) {
			const target = "pfremote://fabric-example/devices/device-example/capabilities/shell-main"
			peer := &recoveringTailscalePeer{nodeID: "node-original"}
			bound := LegacyEndpointRoutes{byAdapter: map[string]map[string][]legacyBoundEndpoint{
				"tailscale": {target: {{endpoint: route.Endpoint{ID: "endpoint-example", Address: "192.0.2.20", Port: 22}}}},
			}, tailscaleVerifier: peer}
			if overlay {
				var err error
				bound, err = mergeLegacyEndpointRoutes(LegacyEndpointRoutes{}, bound)
				if err != nil {
					t.Fatal(err)
				}
			}
			provider := bound.Provider("tailscale")
			request := route.Request{CanonicalTarget: target}
			if _, err := provider.Acquire(context.Background(), request); err == nil {
				t.Fatal("offline route accepted")
			}
			peer.available = true
			a, err := provider.Acquire(context.Background(), request)
			if err != nil {
				t.Fatal("cached route did not recover:", err)
			}
			a.Close()
			peer.available = false
			if _, err := provider.Acquire(context.Background(), request); err == nil {
				t.Fatal("offline identity reused")
			}
		})
	}
}

func TestPinnedTailscaleNodeIsNeverReplacedByLateDiscovery(t *testing.T) {
	const target = "pfremote://fabric-example/devices/device-example/capabilities/shell-main"
	peer := &recoveringTailscalePeer{available: true, nodeID: "node-replacement"}
	bound := LegacyEndpointRoutes{byAdapter: map[string]map[string][]legacyBoundEndpoint{
		"tailscale": {target: {{endpoint: route.Endpoint{ID: "endpoint-example", Address: "192.0.2.20", Port: 22}, nodeID: "node-original"}}},
	}, tailscaleVerifier: peer}
	bound, err := mergeLegacyEndpointRoutes(LegacyEndpointRoutes{}, bound)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Provider("tailscale").Acquire(context.Background(), route.Request{CanonicalTarget: target}); err == nil {
		t.Fatal("pinned node was silently replaced")
	}
}

func (p fixedTailscalePeer) VerifyPeer(_ context.Context, address, nodeID string) error {
	if address != p.address || nodeID != p.nodeID {
		return errors.New("unexpected peer")
	}
	return nil
}

func TestBindLegacyEndpointRoutesKeepsEndpointsPrivateAndTargetBound(t *testing.T) {
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	canonical := "pfremote://fabric-private/devices/device-lab/capabilities/desktop-main"
	connection := source.Connections["service-desktop"]
	for i := range connection.Routes {
		if connection.Routes[i].Adapter == "tailscale" {
			connection.Routes[i].NodeID = "node-synthetic"
		}
	}
	source.Connections["service-desktop"] = connection
	bound, err := BindLegacyEndpointRoutes(source, map[string]string{"service-desktop": canonical})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := bound.Provider("tailscale").Acquire(context.Background(), route.Request{CanonicalTarget: canonical}); err == nil {
		t.Fatal("tailscale route acquired without immutable identity verifier")
	}
	bound = bound.WithTailscaleVerifier(fixedTailscalePeer{address: "host.example.invalid", nodeID: "node-synthetic"})
	tailscale, err := bound.Provider("tailscale").Acquire(context.Background(), route.Request{CanonicalTarget: canonical})
	if err != nil {
		t.Fatal(err)
	}
	if candidate := tailscale.Candidate(); candidate.Address != "host.example.invalid" || candidate.Port != 3389 || candidate.Adapter != "tailscale" {
		t.Fatalf("tailscale candidate = %#v", candidate)
	}
	lan, err := bound.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: canonical})
	if err != nil {
		t.Fatal(err)
	}
	if candidate := lan.Candidate(); candidate.Address != "192.0.2.20" || candidate.Port != 3389 || candidate.Adapter != "lan" {
		t.Fatalf("lan candidate = %#v", candidate)
	}
	if _, err := bound.Provider("lan").Acquire(context.Background(), route.Request{CanonicalTarget: "pfremote://fabric-private/devices/device-other/capabilities/desktop-main"}); err == nil {
		t.Fatal("unbound target acquired a private endpoint")
	}

	encoded, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"host.example.invalid", "192.0.2.20", "3389"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("serialized route binding contains %q: %s", forbidden, encoded)
		}
	}
}

func TestBindLegacyEndpointRoutesRejectsUnknownDuplicateOrInvalidBindings(t *testing.T) {
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	canonical := "pfremote://fabric-private/devices/device-lab/capabilities/desktop-main"
	tests := []map[string]string{
		{},
		{"missing-service": canonical},
		{"service-desktop": "not-a-target"},
		{"service-desktop": canonical, "service-shell": canonical},
	}
	for _, bindings := range tests {
		if _, err := BindLegacyEndpointRoutes(source, bindings); err == nil {
			t.Fatalf("unsafe bindings accepted: %#v", bindings)
		}
	}
}

func TestAvailableAdaptersDisablesConfiguredButUnreachableManualRoute(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	source, err := LoadLegacyCenterCatalog(syntheticLegacyCenterCatalogWithPorts())
	if err != nil {
		t.Fatal(err)
	}
	connection := source.Connections["service-desktop"]
	for index := range connection.Routes {
		if connection.Routes[index].Adapter == "lan" {
			connection.Routes[index].Address = address.IP.String()
			connection.Routes[index].Port = address.Port
		}
	}
	source.Connections["service-desktop"] = connection
	canonical := "pfremote://fabric-private/devices/device-lab/capabilities/desktop-main"
	bound, err := BindLegacyEndpointRoutes(source, map[string]string{"service-desktop": canonical})
	if err != nil {
		t.Fatal(err)
	}
	if available := bound.AvailableAdapters(context.Background(), canonical, 200*time.Millisecond); !available["lan"] {
		t.Fatal("reachable LAN route was not available")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if available := bound.AvailableAdapters(context.Background(), canonical, 100*time.Millisecond); available["lan"] {
		t.Fatal("stale LAN route remained available")
	}
}

func syntheticLegacyCenterCatalogWithPorts() []byte {
	return syntheticLegacyCenterCatalog(false)
}
