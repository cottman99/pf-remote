//go:build windows

package mcpstdio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/localapi"
)

type alignedShellRunner struct {
	mu      sync.Mutex
	target  string
	command []string
}

func (r *alignedShellRunner) Run(_ context.Context, target string, command []string) (actions.ShellRunResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.target = target
	r.command = append([]string(nil), command...)
	return actions.ShellRunResult{SessionID: "session-aligned-mcp", ExitCode: 0, Output: "aligned-control-completed\n"}, nil
}

func (r *alignedShellRunner) observed() (string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.target, append([]string(nil), r.command...)
}

func TestHandoffContextReachesExactActionRunnerThroughLocalAPIAndMCP(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\pfremote-mcp-alignment-%d`, os.Getpid())
	t.Setenv("PFREMOTE_LOCAL_ENDPOINT", endpoint)
	listener, _, err := localapi.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runner := &alignedShellRunner{}
	service := actions.New()
	service.Shell = runner
	serveDone := make(chan error, 1)
	go func() { serveDone <- localapi.NewServerWithService(service).Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		if err := <-serveDone; err != nil {
			t.Errorf("local API serve: %v", err)
		}
	})

	client := localapi.NewClient()
	handoff, err := client.Context(context.Background(), "compute-node/shell", "run the selected task", nil)
	if err != nil {
		t.Fatal(err)
	}
	canonical := handoff.Target.Canonical
	if !strings.Contains(handoff.Envelope, "target: "+canonical) {
		t.Fatalf("handoff envelope = %q", handoff.Envelope)
	}

	request := `{"jsonrpc":"2.0","id":"aligned-exec","method":"tools/call","params":{"name":"pfremote_exec","arguments":{"target":"` + canonical + `","command":["fixture-task","--exact-target"]}}}`
	var output bytes.Buffer
	if err := (Server{Client: client}).Run(context.Background(), strings.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	response := decodeResponses(t, output.String())[0]
	result, ok := response["result"].(map[string]any)
	if !ok || result["isError"] != false {
		t.Fatalf("MCP response = %#v", response)
	}
	structured := result["structuredContent"].(map[string]any)
	target := structured["target"].(map[string]any)
	observedTarget, observedCommand := runner.observed()
	if target["canonical"] != canonical || observedTarget != canonical || len(observedCommand) != 2 || observedCommand[1] != "--exact-target" {
		t.Fatalf("canonical=%q response=%#v runner_target=%q command=%v", canonical, target, observedTarget, observedCommand)
	}
}
