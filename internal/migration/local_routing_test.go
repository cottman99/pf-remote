package migration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cottman99/pf-remote/internal/route"
)

func TestLocalRoutingPreservesLegacyDefaultAndRejectsInvalidState(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.db")
	value, err := LoadLocalRouting(state)
	if err != nil || value.RequiredAdapter != "" {
		t.Fatal("absent configuration changed legacy behavior")
	}
	path := filepath.Join(dir, "local-routing-v1.json")
	source := filepath.Join(dir, "source.toml")
	if err := os.WriteFile(source, []byte("# private"), 0600); err != nil {
		t.Fatal(err)
	}
	valid := LocalRouting{SchemaVersion: "pfremote.local-routing/v1", FRPConfigPath: source, RequiredAdapter: "frp"}
	b, _ := json.Marshal(valid)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLocalRouting(state); err != nil || got != valid {
		t.Fatal("valid routing was rejected", err)
	}
	for _, bad := range []string{
		`{}`, `{"schema_version":"pfremote.local-routing/v1","required_adapter":"frp"}`,
		`{"schema_version":"pfremote.local-routing/v1","required_adapter":"unknown"}`,
		`{"schema_version":"pfremote.local-routing/v1","catalog_path":"relative.json"}`,
		`{"schema_version":"pfremote.local-routing/v1","unrecognized":true}`,
		string(b) + `{}`,
	} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLocalRouting(state); err == nil {
			t.Fatal("invalid settings silently accepted", bad)
		}
	}
}

type countingRoutingProvider struct{ calls *int }

func (p countingRoutingProvider) Acquire(context.Context, route.Request) (route.Acquisition, error) {
	*p.calls++
	return nil, os.ErrNotExist
}

func TestRequiredGatewayCannotFallBackToDirectOrUnknownRoute(t *testing.T) {
	direct, gateway := 0, 0
	providers := []route.NamedProvider{
		{Adapter: "lan", Provider: countingRoutingProvider{&direct}},
		{Adapter: "frp", Provider: countingRoutingProvider{&gateway}},
	}
	settings := LocalRouting{RequiredAdapter: "frp"}
	if _, err := settings.Select(providers, nil).Acquire(context.Background(), route.Request{}); err == nil {
		t.Fatal("unavailable Gateway succeeded")
	}
	if direct != 0 || gateway != 1 {
		t.Fatal("required Gateway fell back to another path")
	}
	if _, err := settings.Select(providers[:1], nil).Acquire(context.Background(), route.Request{}); err == nil || direct != 0 {
		t.Fatal("missing Gateway fell back")
	}
	legacy := (LocalRouting{}).Select(providers, nil)
	if len(legacy.Providers) != 2 || !legacy.EvaluateAll {
		t.Fatal("default Smart routing changed")
	}
}

func TestLegacyShellUsesSameGatewayPolicy(t *testing.T) {
	s := LegacyShellConnections{users: map[string]string{"example": "owner"}}
	runner := s.RoutedRunner("controller", nil, LegacyEndpointRoutes{}, "", countingRoutingProvider{new(int)}, LocalRouting{RequiredAdapter: "frp"}).(legacyShellRunner)
	if runner.gateway == nil || runner.settings.RequiredAdapter != "frp" || runner.users["example"] != "owner" {
		t.Fatal("Shell lost private routing or account")
	}
}
