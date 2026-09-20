package mcpstdio

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type fakeClient struct {
	service contracts.Target
	exec    execArguments
}

func (f *fakeClient) List(context.Context) (contracts.CatalogResponse, error) {
	return contracts.CatalogResponse{SchemaVersion: contracts.CatalogSchema, Targets: []contracts.Target{f.service}}, nil
}
func (f *fakeClient) Inspect(_ context.Context, target string) (contracts.InspectResponse, error) {
	return contracts.InspectResponse{SchemaVersion: contracts.InspectSchema, Target: f.service}, nil
}
func (f *fakeClient) Context(context.Context, string, string, []string) (contracts.ContextResponse, error) {
	return contracts.ContextResponse{SchemaVersion: contracts.ContextSchema, Target: f.service, Envelope: "PF_REMOTE_TARGET/1"}, nil
}
func (f *fakeClient) Doctor(context.Context) (contracts.DoctorResponse, error) {
	return contracts.DoctorResponse{SchemaVersion: contracts.DoctorSchema, Overall: "ready"}, nil
}
func (f *fakeClient) Connect(context.Context, string) (contracts.ShellActionResponse, error) {
	return contracts.ShellActionResponse{SchemaVersion: contracts.ShellActionSchema, Status: "completed", Target: f.service}, nil
}
func (f *fakeClient) Exec(_ context.Context, target string, command []string) (contracts.ShellActionResponse, error) {
	f.exec = execArguments{Target: target, Command: append([]string(nil), command...)}
	return contracts.ShellActionResponse{SchemaVersion: contracts.ShellActionSchema, Status: "completed", Target: f.service}, nil
}
func (f *fakeClient) Open(context.Context, string) (contracts.DesktopActionResponse, error) {
	return contracts.DesktopActionResponse{SchemaVersion: contracts.DesktopActionSchema, Status: "completed", Target: f.service}, nil
}

func TestServerSupportsModernDiscoveryAndToolListing(t *testing.T) {
	client := &fakeClient{service: syntheticTarget()}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
	}, "\n")
	var output bytes.Buffer
	if err := (Server{Client: client}).Run(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	responses := decodeResponses(t, output.String())
	discovery := responses[0]["result"].(map[string]any)
	versions := discovery["supportedVersions"].([]any)
	if versions[0] != modernProtocol {
		t.Fatalf("discovery=%#v", discovery)
	}
	list := responses[1]["result"].(map[string]any)
	if len(list["tools"].([]any)) != 7 || list["cacheScope"] != "private" {
		t.Fatalf("tools/list=%#v", list)
	}
}

func TestServerInspectReturnsStructuredExactTarget(t *testing.T) {
	target := syntheticTarget()
	client := &fakeClient{service: target}
	input := `{"jsonrpc":"2.0","id":"inspect-1","method":"tools/call","params":{"name":"pfremote_inspect","arguments":{"target":"` + target.Canonical + `"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`
	var output bytes.Buffer
	if err := (Server{Client: client}).Run(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	result := decodeResponses(t, output.String())[0]["result"].(map[string]any)
	structured := result["structuredContent"].(map[string]any)
	resolved := structured["target"].(map[string]any)
	if resolved["canonical"] != target.Canonical || result["isError"] != false {
		t.Fatalf("result=%#v", result)
	}
}

func TestServerExecPreservesArgumentVector(t *testing.T) {
	target := syntheticTarget()
	client := &fakeClient{service: target}
	input := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pfremote_exec","arguments":{"target":"` + target.Canonical + `","command":["echo","done"]}}}`
	var output bytes.Buffer
	if err := (Server{Client: client}).Run(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if client.exec.Target != target.Canonical || len(client.exec.Command) != 2 || client.exec.Command[1] != "done" {
		t.Fatalf("exec=%#v", client.exec)
	}
}

func TestServerRejectsUnknownToolArguments(t *testing.T) {
	client := &fakeClient{service: syntheticTarget()}
	input := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"pfremote_inspect","arguments":{"target":"compute-node/shell","route":"forbidden"}}}`
	var output bytes.Buffer
	if err := (Server{Client: client}).Run(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	decoded := decodeResponses(t, output.String())[0]
	failure := decoded["error"].(map[string]any)
	if failure["code"] != float64(-32602) {
		t.Fatalf("response=%#v", decoded)
	}
}

func syntheticTarget() contracts.Target {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	fixture := catalog.Synthetic()
	fixture.Now = func() time.Time { return now }
	resolved, err := fixture.Resolve("compute-node/shell")
	if err != nil {
		panic(err)
	}
	return resolved
}

func decodeResponses(t *testing.T, raw string) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	var responses []map[string]any
	for decoder.More() {
		var value map[string]any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, value)
	}
	return responses
}
