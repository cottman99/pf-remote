package enrollment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCapabilityClientRequiresTLSExceptLoopback(t *testing.T) {
	client := Client{BaseURL: "http://192.0.2.10"}
	if _, err := client.ListShellCapabilities(context.Background(), ShellCapabilityListRequest{}); err == nil {
		t.Fatal("remote plaintext Gateway accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","fabric_id":"fabric-test","directory_version":1,"capabilities":[]}`))
	}))
	defer server.Close()
	client = Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.ListShellCapabilities(context.Background(), ShellCapabilityListRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestClientHealthRequiresAnExactHealthyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/healthz" {
			t.Fatalf("health request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	if err := (Client{BaseURL: server.URL}).Health(context.Background()); err != nil {
		t.Fatal(err)
	}

	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"degraded"}`))
	}))
	defer invalid.Close()
	if err := (Client{BaseURL: invalid.URL}).Health(context.Background()); err == nil {
		t.Fatal("invalid Gateway health response accepted")
	}
}

func TestClientEnrollmentEndpoints(t *testing.T) {
	wanted := map[string]bool{
		"/api/v1/owner/initialize":              false,
		"/api/v1/device-authorizations":         false,
		"/api/v1/device-authorizations/approve": false,
		"/api/v1/device-authorizations/poll":    false,
		"/api/v1/versions/sync":                 false,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, exists := wanted[r.URL.Path]; !exists {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		wanted[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/owner/initialize":
			_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","fabric_id":"fabric-test","owner_device_id":"device-owner","directory_version":1}`))
		case "/api/v1/device-authorizations":
			_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","device_code":"device-code","user_code":"USER-CODE","verification_uri":"https://example.invalid","verification_uri_complete":"https://example.invalid/complete","expires_in":600,"interval":5}`))
		case "/api/v1/device-authorizations/approve":
			_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","device_id":"device-node","device_name":"Node","status":"active"}`))
		case "/api/v1/device-authorizations/poll":
			_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","fabric_id":"fabric-test","device_id":"device-node","status":"active","directory_version":2,"grant_version":2}`))
		case "/api/v1/versions/sync":
			_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","fabric_id":"fabric-test","device_id":"device-node","device_status":"active","directory_version":2,"grant_version":2,"supported_client_major":1}`))
		}
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL}
	ctx := context.Background()
	if _, err := client.InitializeOwner(ctx, OwnerInitializationRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RequestDeviceAuthorization(ctx, DeviceAuthorizationRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApproveDevice(ctx, OwnerApprovalRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PollDevice(ctx, DevicePollRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SyncVersions(ctx, VersionSyncRequest{}); err != nil {
		t.Fatal(err)
	}
	for path, called := range wanted {
		if !called {
			t.Fatalf("endpoint not called: %s", path)
		}
	}
}
