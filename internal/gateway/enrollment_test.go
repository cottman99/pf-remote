package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type gatewayTestKey struct {
	deviceID string
	public   ed25519.PublicKey
	private  ed25519.PrivateKey
}

func (k gatewayTestKey) DeviceID() string             { return k.deviceID }
func (k gatewayTestKey) PublicKey() ed25519.PublicKey { return k.public }
func (k gatewayTestKey) Sign(value []byte) ([]byte, error) {
	return ed25519.Sign(k.private, value), nil
}

func TestEnrollmentHTTP_FullSignedJourney(t *testing.T) {
	handler := (Server{Version: "dev", Commit: "test"}).Handler()
	owner := makeGatewayTestKey(t, 0x11)
	device := makeGatewayTestKey(t, 0x22)

	ownerRequest := enrollment.OwnerInitializationRequest{
		SchemaVersion: enrollment.SchemaVersion, FabricID: "fabric-synthetic",
		OwnerDeviceID: owner.deviceID, OwnerPublicKey: enrollment.EncodePublicKey(owner.public), ClientVersion: "1.0.0",
	}
	ownerRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(owner.private, enrollment.OwnerInitializationMessage(ownerRequest)))
	var ownerResponse enrollment.OwnerInitializationResponse
	postGatewayJSON(t, handler, "/api/v1/owner/initialize", ownerRequest, http.StatusOK, &ownerResponse)

	authorizationRequest := enrollment.DeviceAuthorizationRequest{
		SchemaVersion: enrollment.SchemaVersion, DeviceID: device.deviceID, DeviceName: "Synthetic node",
		DevicePublicKey: enrollment.EncodePublicKey(device.public), ClientVersion: "1.0.0", RequestID: "device-http-request-0001",
	}
	authorizationRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(device.private, enrollment.DeviceAuthorizationMessage(authorizationRequest)))
	var authorization enrollment.DeviceAuthorizationResponse
	postGatewayJSON(t, handler, "/api/v1/device-authorizations", authorizationRequest, http.StatusOK, &authorization)

	approvalRequest := enrollment.OwnerApprovalRequest{
		SchemaVersion: enrollment.SchemaVersion, OwnerDeviceID: owner.deviceID,
		UserCode: authorization.UserCode, RequestID: "owner-http-request-0001", ClientVersion: "1.0.0",
	}
	approvalRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(owner.private, enrollment.OwnerApprovalMessage(approvalRequest)))
	var approval enrollment.OwnerApprovalResponse
	postGatewayJSON(t, handler, "/api/v1/device-authorizations/approve", approvalRequest, http.StatusOK, &approval)

	pollRequest := enrollment.DevicePollRequest{
		SchemaVersion: enrollment.SchemaVersion, DeviceCode: authorization.DeviceCode, ClientVersion: "1.0.0",
	}
	var activated enrollment.DeviceActivationResponse
	postGatewayJSON(t, handler, "/api/v1/device-authorizations/poll", pollRequest, http.StatusOK, &activated)
	if activated.DeviceID != device.deviceID || activated.Status != "active" {
		t.Fatalf("activation = %#v", activated)
	}

	syncRequest := enrollment.VersionSyncRequest{
		SchemaVersion: enrollment.SchemaVersion, DeviceID: device.deviceID,
		RequestID: "device-http-request-0002", ClientVersion: "1.0.0",
	}
	syncRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(device.private, enrollment.VersionSyncMessage(syncRequest)))
	var syncResponse enrollment.VersionSyncResponse
	postGatewayJSON(t, handler, "/api/v1/versions/sync", syncRequest, http.StatusOK, &syncResponse)

	binding, err := shellbinding.Sign("fabric-synthetic", "shell-main", 1, []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: "AAAAC3NzaC1lZDI1NTE5AAAAIMvF3FK8rr2A2r9iVfj0x8l26LThB5GXxJ7XyQPRZP5Z"}}, device)
	if err != nil {
		t.Fatal(err)
	}
	publishRequest := enrollment.ShellCapabilityPublishRequest{SchemaVersion: enrollment.SchemaVersion, DeviceID: device.deviceID, Binding: *binding, RequestID: "device-http-request-0003", ClientVersion: "1.0.0"}
	publishRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(device.private, enrollment.ShellCapabilityPublishMessage(publishRequest)))
	server := httptest.NewServer(handler)
	defer server.Close()
	client := enrollment.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	publishResponse, err := client.PublishShellCapability(context.Background(), publishRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !publishResponse.Changed || publishResponse.CapabilityID != "shell-main" {
		t.Fatalf("publish = %#v", publishResponse)
	}
	listRequest := enrollment.ShellCapabilityListRequest{SchemaVersion: enrollment.SchemaVersion, OwnerDeviceID: owner.deviceID, RequestID: "owner-http-request-shell-0001", ClientVersion: "1.0.0"}
	listRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(owner.private, enrollment.ShellCapabilityListMessage(listRequest)))
	listResponse, err := client.ListShellCapabilities(context.Background(), listRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(listResponse.Capabilities) != 1 || listResponse.Capabilities[0].DeviceID != device.deviceID {
		t.Fatalf("Shell capability list = %#v", listResponse)
	}

	revokeRequest := enrollment.OwnerRevocationRequest{
		SchemaVersion: enrollment.SchemaVersion, OwnerDeviceID: owner.deviceID, DeviceID: device.deviceID,
		RequestID: "owner-http-request-0002", ClientVersion: "1.0.0",
	}
	revokeRequest.Signature = enrollment.EncodeSignature(ed25519.Sign(owner.private, enrollment.OwnerRevocationMessage(revokeRequest)))
	var revoked enrollment.OwnerRevocationResponse
	postGatewayJSON(t, handler, "/api/v1/devices/revoke", revokeRequest, http.StatusOK, &revoked)
	if revoked.Status != "revoked" || revoked.GrantVersion <= activated.GrantVersion {
		t.Fatalf("revocation = %#v", revoked)
	}
}

func TestEnrollmentHTTP_RemotePlaintext_IsRejected(t *testing.T) {
	handler := (Server{}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/owner/initialize", bytes.NewBufferString("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.25:49200"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response contracts.Error
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "TLS_REQUIRED" || response.CorrelationID == "" {
		t.Fatalf("error = %#v", response)
	}
}

func TestEnrollmentHTTP_UnknownJSONField_IsRejected(t *testing.T) {
	handler := (Server{}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/owner/initialize", bytes.NewBufferString(`{"unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "127.0.0.1:49200"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response contracts.Error
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "INVALID_REQUEST" {
		t.Fatalf("error = %#v", response)
	}
}

func TestEnrollmentHTTP_UnsupportedVersion_UsesStandardError(t *testing.T) {
	handler := (Server{}).Handler()
	owner := makeGatewayTestKey(t, 0x31)
	request := enrollment.OwnerInitializationRequest{
		SchemaVersion: "pfremote.enrollment/v2", FabricID: "fabric-synthetic",
		OwnerDeviceID: owner.deviceID, OwnerPublicKey: enrollment.EncodePublicKey(owner.public), ClientVersion: "2.0.0",
	}
	request.Signature = enrollment.EncodeSignature(ed25519.Sign(owner.private, enrollment.OwnerInitializationMessage(request)))
	var response contracts.Error
	postGatewayJSON(t, handler, "/api/v1/owner/initialize", request, http.StatusUpgradeRequired, &response)
	if response.SchemaVersion != contracts.ErrorSchema || response.Code != "UNSUPPORTED_SCHEMA" || response.CorrelationID == "" {
		t.Fatalf("error = %#v", response)
	}
}

func postGatewayJSON(t *testing.T, handler http.Handler, path string, requestBody any, expectedStatus int, responseBody any) {
	t.Helper()
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "127.0.0.1:49200"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != expectedStatus {
		t.Fatalf("%s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%s missing no-store header", path)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), responseBody); err != nil {
		t.Fatal(err)
	}
}

func makeGatewayTestKey(t *testing.T, seedByte byte) gatewayTestKey {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seedByte}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return gatewayTestKey{deviceID: deviceID, public: public, private: private}
}
