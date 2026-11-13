package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestPeerManagementUsesProtectedHandlerWithoutLegacyProvider(t *testing.T) {
	called := false
	h := Handler{Peers: func(_ context.Context, b json.RawMessage) (any, error) {
		called = true
		return map[string]string{"status": "ready"}, nil
	}}
	r := h.Handle(Request{SchemaVersion: SchemaVersion, Action: "peer-management", PeerRequest: json.RawMessage(`{"operation":"status"}`)})
	if r.Error != nil || !called {
		t.Fatal("peer handler unavailable without legacy catalog")
	}
	h.Peers = func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("private internal failure")
	}
	r = h.Handle(Request{SchemaVersion: SchemaVersion, Action: "peer-management"})
	if r.Error == nil || r.Error.Code != "PEER_REQUEST_REJECTED" || r.Error.Summary == "private internal failure" {
		t.Fatal("missing safe rejection")
	}
}
