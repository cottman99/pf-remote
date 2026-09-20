package localapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
)

func TestProtectedEndpointRoundTrip(t *testing.T) {
	setTestEndpoint(t)
	listener, endpoint, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- NewServerWithService(actions.New()).Serve(ctx, listener) }()

	connection, err := dialTestEndpoint(endpoint, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{SchemaVersion: SchemaVersion, Action: "list"}
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v", response)
	}

	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}
