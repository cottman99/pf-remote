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
