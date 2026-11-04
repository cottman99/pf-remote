package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
)

func TestLegacyGatewayRouteStaysPrivateAndProducesABoundedVisitorLease(t *testing.T) {
	credential := strings.Repeat("a", 18)
	source := LegacyCenterCompatibility{Connections: map[string]LegacyCenterConnection{
		"service-a": {Protocol: "vnc", Routes: []LegacyCenterRoute{{
			Adapter: "legacy-gateway", Address: "relay.example.com", ControlPort: 7000,
			ProxyName: "desktop_proxy_01", ProxySecret: credential,
		}}},
	}}
	target := "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"
	routes := BindLegacyGatewayRoutes(source, map[string]string{"service-a": target})
	if !routes.HasTarget(target) {
		t.Fatal("legacy Gateway route was not bound to the exact target")
	}
	client := legacyGatewayLeaseClient{routes: routes.byTarget}
	expiry := time.Now().UTC().Add(time.Hour)
	lease, err := client.Acquire(context.Background(), route.Request{CanonicalTarget: target, AuthorizationExpiry: expiry})
	if err != nil || lease.Adapter != "frp" || lease.CanonicalTarget != target || lease.ServerName != "desktop_proxy_01" || !lease.ExpiresAt.Before(expiry) {
		t.Fatalf("lease=%#v err=%v", lease, err)
	}
}

func TestLegacyGatewayRouteReusesOnlyTheMatchingManagedVisitor(t *testing.T) {
	target := "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"
	credential := strings.Repeat("a", 18)
	otherCredential := strings.Repeat("b", 18)
	routes := LegacyGatewayRoutes{byTarget: map[string]LegacyCenterRoute{target: {
		Adapter: "legacy-gateway", Address: "relay.example.com", ControlPort: 7000,
		ProxyName: "desktop_proxy_01", ProxySecret: credential,
	}}}
	configPath := filepath.Join(t.TempDir(), "frpc-managed.toml")
	secretField := "secret" + "Key"
	config := fmt.Sprintf(`serverAddr = "relay.example.com"

[[visitors]]
name = "local_visitor_01"
type = "stcp"
serverName = "desktop_proxy_01"
%s = "%s"
bindAddr = "127.0.0.1"
bindPort = 45901

[[visitors]]
name = "local_visitor_02"
type = "stcp"
serverName = "desktop_proxy_02"
%s = "%s"
bindAddr = "127.0.0.1"
bindPort = 45902
`, secretField, credential, secretField, otherCredential)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	acquired, err := routes.ManagedProvider(configPath).Acquire(context.Background(), route.Request{CanonicalTarget: target})
	if err != nil {
		t.Fatal(err)
	}
	candidate := acquired.Candidate()
	if candidate.Adapter != "frp" || candidate.Address != "127.0.0.1" || candidate.Port != 45901 {
		t.Fatalf("candidate=%#v", candidate)
	}
}

func TestManagedTCPVisitorsIgnoreUnrelatedUDPIdentity(t *testing.T) {
	target := "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"
	credential := strings.Repeat("a", 18)
	routes := LegacyGatewayRoutes{byTarget: map[string]LegacyCenterRoute{target: {
		Adapter: "legacy-gateway", Address: "relay.example.com", ControlPort: 7000,
		ProxyName: "desktop_proxy_01", ProxySecret: credential,
	}}}
	configPath := filepath.Join(t.TempDir(), "frpc.toml")
	field := "secret" + "Key"
	// UDP may share a numeric port and have a different credential alphabet.
	config := fmt.Sprintf("[[visitors]]\ntype = \"sudp\"\nserverName = \"desktop_proxy_01\"\n%s = \"_udp_fixture\"\nbindPort = 45901\n\n[[visitors]]\ntype = \"stcp\"\nserverName = \"desktop_proxy_01\"\n%s = \"%s\"\nbindAddr = \"127.0.0.1\"\nbindPort = 45901\n", field, field, credential)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := routes.ManagedProvider(configPath).Acquire(context.Background(), route.Request{CanonicalTarget: target})
	if err != nil {
		t.Fatal("unrelated UDP visitor rejected the TCP route", err)
	}
	defer a.Close()
	if a.Candidate().Network != "tcp" || a.Candidate().Port != 45901 {
		t.Fatal("wrong transport selected")
	}
	// An otherwise matching UDP visitor must never become a TCP route.
	config = strings.ReplaceAll(config, "\"stcp\"", "\"sudp\"")
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ManagedProvider(configPath).Acquire(context.Background(), route.Request{CanonicalTarget: target}); err == nil {
		t.Fatal("UDP visitor accepted as TCP")
	}
}

func TestLegacyGatewayRouteRejectsNonLoopbackManagedVisitor(t *testing.T) {
	target := "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"
	credential := strings.Repeat("a", 18)
	routes := LegacyGatewayRoutes{byTarget: map[string]LegacyCenterRoute{target: {
		Adapter: "legacy-gateway", Address: "relay.example.com", ControlPort: 7000,
		ProxyName: "desktop_proxy_01", ProxySecret: credential,
	}}}
	configPath := filepath.Join(t.TempDir(), "frpc-managed.toml")
	config := fmt.Sprintf(`[[visitors]]
serverName = "desktop_proxy_01"
%s = "%s"
bindAddr = "0.0.0.0"
bindPort = 45901
`, "secret"+"Key", credential)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ManagedProvider(configPath).Acquire(context.Background(), route.Request{CanonicalTarget: target}); err == nil {
		t.Fatal("non-loopback legacy visitor became selectable")
	}
}

func TestLegacyGatewayRouteRejectsIncompletePrivateMaterial(t *testing.T) {
	source := LegacyCenterCompatibility{Connections: map[string]LegacyCenterConnection{
		"service-a": {Protocol: "vnc", Routes: []LegacyCenterRoute{{Adapter: "legacy-gateway", Address: "relay.example.com", ControlPort: 7000}}},
	}}
	target := "pfremote://fabric-test/devices/device-a/capabilities/desktop-a"
	if BindLegacyGatewayRoutes(source, map[string]string{"service-a": target}).HasTarget(target) {
		t.Fatal("incomplete legacy Gateway material became selectable")
	}
}
