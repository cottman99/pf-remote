package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type testClient struct{ service actions.Service }

type testDesktop struct{}

func (testDesktop) Run(context.Context, string) (actions.DesktopRunResult, error) {
	return actions.DesktopRunResult{SessionID: "desktop-cli-01", Protocol: "rdp", RenderingEnvironment: "virtual"}, nil
}

func (testDesktop) RunVia(_ context.Context, _ string, adapter string) (actions.DesktopRunResult, error) {
	return actions.DesktopRunResult{SessionID: "desktop-cli-01", Protocol: "rdp", RenderingEnvironment: "virtual", RouteAdapter: adapter}, nil
}

func newTestClient() testClient { return testClient{service: actions.New()} }

func (c testClient) List(context.Context) (contracts.CatalogResponse, error) {
	return c.service.List(), nil
}

func (c testClient) Inspect(_ context.Context, target string) (contracts.InspectResponse, error) {
	return c.service.Inspect(target)
}

func (c testClient) Context(_ context.Context, target, task string, constraints []string) (contracts.ContextResponse, error) {
	return c.service.Context(target, task, constraints)
}

func (c testClient) Doctor(context.Context) (contracts.DoctorResponse, error) {
	return c.service.Doctor(), nil
}

func (c testClient) Connect(ctx context.Context, target string) (contracts.ShellActionResponse, error) {
	return c.service.Connect(ctx, target)
}

func (c testClient) Exec(ctx context.Context, target string, command []string) (contracts.ShellActionResponse, error) {
	return c.service.Exec(ctx, target, command)
}

func (c testClient) Open(ctx context.Context, target string) (contracts.DesktopActionResponse, error) {
	return c.service.Open(ctx, target)
}

func (c testClient) OpenVia(ctx context.Context, target, adapter string) (contracts.DesktopActionResponse, error) {
	return c.service.OpenVia(ctx, target, adapter)
}

func (c testClient) SaveDesktopCredentialAndOpen(ctx context.Context, target, credential string) (contracts.DesktopActionResponse, error) {
	return c.service.SaveDesktopCredentialAndOpen(ctx, target, credential)
}

func (c testClient) SaveDesktopCredentialAndOpenVia(ctx context.Context, target, credential, adapter string) (contracts.DesktopActionResponse, error) {
	return c.service.SaveDesktopCredentialAndOpenVia(ctx, target, credential, adapter)
}

func TestListJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"list", "--json"}, bytes.NewReader(nil), &stdout, &stderr, newTestClient()); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	var response map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response["schema_version"] != "pfremote.catalog/v1" {
		t.Fatalf("schema_version = %v", response["schema_version"])
	}
}

func TestOpenJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	client := newTestClient()
	client.service.Desktop = testDesktop{}
	if code := run([]string{"open", "compute-node/desktop", "--json"}, bytes.NewReader(nil), &stdout, &stderr, client); code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr.String())
	}
	var response map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response["schema_version"] != contracts.DesktopActionSchema || response["protocol"] != "rdp" {
		t.Fatalf("response = %#v", response)
	}
}
