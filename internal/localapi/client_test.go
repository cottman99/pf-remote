package localapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
)

type clientShell struct{}

func (clientShell) Run(context.Context, string, []string) (actions.ShellRunResult, error) {
	return actions.ShellRunResult{SessionID: "session-client-01", Output: "finished\n"}, nil
}

type clientDesktop struct{}

func (clientDesktop) Run(context.Context, string) (actions.DesktopRunResult, error) {
	return actions.DesktopRunResult{SessionID: "desktop-client-01", Protocol: "rdp", RenderingEnvironment: "virtual"}, nil
}

func TestClientList_UsesVersionedLocalProtocol(t *testing.T) {
	client := pipeClient(Handler{Service: actions.New()})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := client.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if response.SchemaVersion == "" || len(response.Targets) != 4 {
		t.Fatalf("response = %#v", response)
	}
}

func TestClientList_PreservesStructuredRemoteError(t *testing.T) {
	client := pipeClient(Handler{Provider: func() (actions.Service, error) {
		return actions.Service{}, errors.New("synthetic state failure")
	}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.List(ctx)
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Failure.Code != "STATE_UNAVAILABLE" {
		t.Fatalf("error = %#v", err)
	}
}

func TestClientExecUsesProtectedVersionedAction(t *testing.T) {
	service := actions.New()
	service.Shell = clientShell{}
	client := pipeClient(Handler{Service: service})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := client.Exec(ctx, "compute-node/shell", []string{"echo", "finished"})
	if err != nil || response.SessionID != "session-client-01" || response.Output != "finished\n" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestClientOpenUsesProtectedVersionedAction(t *testing.T) {
	service := actions.New()
	service.Desktop = clientDesktop{}
	client := pipeClient(Handler{Service: service})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := client.Open(ctx, "compute-node/desktop")
	if err != nil || response.SessionID != "desktop-client-01" || response.Protocol != "rdp" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func pipeClient(handler Handler) Client {
	return Client{dial: func(context.Context) (net.Conn, error) {
		client, server := net.Pipe()
		go (&Server{Handler: handler}).serveConnection(context.Background(), server)
		return client, nil
	}}
}
