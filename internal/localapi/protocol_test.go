package localapi

import (
	"context"
	"errors"
	"testing"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type protocolShell struct{}

func (protocolShell) Run(context.Context, string, []string) (actions.ShellRunResult, error) {
	return actions.ShellRunResult{SessionID: "session-local-api-01", Output: "ready\n"}, nil
}

func TestRejectsUnknownVersion(t *testing.T) {
	response := NewHandler().Handle(Request{SchemaVersion: "pfremote.local-api/v2", Action: "list"})
	if response.Error == nil || response.Error.Code != "UNSUPPORTED_VERSION" {
		t.Fatalf("response = %#v", response)
	}
}

func TestProviderFailureFailsClosed(t *testing.T) {
	handler := Handler{Provider: func() (actions.Service, error) {
		return actions.Service{}, errors.New("synthetic state failure")
	}}
	response := handler.Handle(Request{SchemaVersion: SchemaVersion, Action: "list"})
	if response.Error == nil || response.Error.Code != "STATE_UNAVAILABLE" || response.Result != nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestListUsesSharedActionCore(t *testing.T) {
	response := (Handler{Service: actions.New()}).Handle(Request{SchemaVersion: SchemaVersion, Action: "list"})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestNewServerWithService_PreservesIdentityStatus(t *testing.T) {
	service := actions.New()
	service.DeviceID = "device-synthetic"
	server := NewServerWithService(service)
	response := server.Handler.Handle(Request{SchemaVersion: SchemaVersion, Action: "doctor"})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestExecReturnsVersionedAgentResult(t *testing.T) {
	service := actions.New()
	service.Shell = protocolShell{}
	response := (Handler{Service: service}).Handle(Request{SchemaVersion: SchemaVersion, Action: "exec", Target: "compute-node/shell", Command: []string{"echo", "ready"}})
	if response.Error != nil {
		t.Fatalf("response=%#v", response)
	}
	result, ok := response.Result.(contracts.ShellActionResponse)
	if !ok || result.SchemaVersion != contracts.ShellActionSchema || result.Output != "ready\n" {
		t.Fatalf("result=%#v", response.Result)
	}
}
