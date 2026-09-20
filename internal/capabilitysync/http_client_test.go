package capabilitysync

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cottman99/pf-remote/internal/enrollment"
)

func TestGatewayHTTPClientTrustsOnlyConfiguredPrivateCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"pfremote.enrollment/v1","fabric_id":"fabric-test","directory_version":1,"capabilities":[]}`))
	}))
	defer server.Close()
	certificatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	client := enrollment.Client{BaseURL: server.URL, HTTPClient: GatewayHTTPClient(certificatePEM)}
	if _, err := client.ListShellCapabilities(context.Background(), enrollment.ShellCapabilityListRequest{}); err != nil {
		t.Fatal(err)
	}
	if GatewayHTTPClient("not a certificate") != nil {
		t.Fatal("invalid private certificate authority was accepted")
	}
}
