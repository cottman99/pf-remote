package enrollment

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

func (c *testClock) Advance(duration time.Duration) { c.now = c.now.Add(duration) }

type keyPair struct {
	deviceID string
	public   ed25519.PublicKey
	private  ed25519.PrivateKey
}

func (k keyPair) DeviceID() string             { return k.deviceID }
func (k keyPair) PublicKey() ed25519.PublicKey { return k.public }
func (k keyPair) Sign(value []byte) ([]byte, error) {
	return ed25519.Sign(k.private, value), nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("synthetic entropy failure") }

func TestEnrollment_FullJourney_ActivatesSyncsAndRevokesDevice(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)}
	manager := newManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{0x23}, 4096)))
	owner := makeKeyPair(t, 0x11)
	device := makeKeyPair(t, 0x22)
	initializeOwner(t, manager, owner)

	authorization := signedDeviceAuthorization(t, device, "Compute node", "device-request-0001")
	codes, err := manager.BeginDeviceAuthorization(authorization)
	if err != nil {
		t.Fatal(err)
	}
	if codes.ExpiresIn != int64(DefaultActivationTTL/time.Second) || codes.Interval != int64(DefaultPollingInterval/time.Second) {
		t.Fatalf("unexpected activation timing: %#v", codes)
	}
	approval := signedApproval(t, owner, codes.UserCode, "owner-request-0001")
	approved, err := manager.ApproveDevice(approval)
	if err != nil {
		t.Fatal(err)
	}
	if approved.DeviceID != device.deviceID || approved.Status != "approved" {
		t.Fatalf("approval = %#v", approved)
	}
	activated, err := manager.PollDevice(DevicePollRequest{
		SchemaVersion: SchemaVersion, DeviceCode: codes.DeviceCode, ClientVersion: "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if activated.Status != "active" || activated.DirectoryVersion != 2 || activated.GrantVersion != 1 {
		t.Fatalf("activation = %#v", activated)
	}

	syncRequest := signedVersionSync(t, device, "device-request-0002")
	synced, err := manager.SyncVersion(syncRequest)
	if err != nil {
		t.Fatal(err)
	}
	if synced.DeviceStatus != "active" || synced.DirectoryVersion != 2 || synced.SupportedClientMajor != 1 {
		t.Fatalf("version sync = %#v", synced)
	}
	if _, err := manager.SyncVersion(syncRequest); faultCode(err) != "REQUEST_REPLAYED" {
		t.Fatalf("replayed sync error = %v", err)
	}

	revocation := signedRevocation(t, owner, device.deviceID, "owner-request-0002")
	revoked, err := manager.RevokeDevice(revocation)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != "revoked" || revoked.DirectoryVersion != 3 || revoked.GrantVersion != 2 {
		t.Fatalf("revocation = %#v", revoked)
	}
	if _, err := manager.SyncVersion(signedVersionSync(t, device, "device-request-0003")); faultCode(err) != "DEVICE_REVOKED" {
		t.Fatalf("revoked sync error = %v", err)
	}
}

func TestShellCapabilityPublication_ReachesOwnerAndDisappearsOnRevocation(t *testing.T) {
	manager := newManager(time.Now, bytes.NewReader(bytes.Repeat([]byte{0x44}, 4096)))
	owner := makeKeyPair(t, 0x31)
	device := makeKeyPair(t, 0x32)
	initializeOwner(t, manager, owner)
	codes, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, device, "Shell node", "device-shell-0001"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApproveDevice(signedApproval(t, owner, codes.UserCode, "owner-shell-0001")); err != nil {
		t.Fatal(err)
	}
	activated, err := manager.PollDevice(DevicePollRequest{SchemaVersion: SchemaVersion, DeviceCode: codes.DeviceCode, ClientVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	publish := signedShellPublication(t, device, "device-shell-0002", 1)
	published, err := manager.PublishShellCapability(publish)
	if err != nil || !published.Changed || published.DirectoryVersion != activated.DirectoryVersion+1 {
		t.Fatalf("published=%#v err=%v", published, err)
	}
	list := signedShellList(owner, "owner-shell-0002")
	listed, err := manager.ListShellCapabilities(list)
	if err != nil || len(listed.Capabilities) != 1 || listed.Capabilities[0].Binding.CapabilityID != "shell-main" || listed.Capabilities[0].DevicePublicKey != EncodePublicKey(device.public) {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}
	publish.RequestID = "device-shell-0003"
	publish.Signature = EncodeSignature(ed25519.Sign(device.private, ShellCapabilityPublishMessage(publish)))
	repeated, err := manager.PublishShellCapability(publish)
	if err != nil || repeated.Changed || repeated.DirectoryVersion != published.DirectoryVersion {
		t.Fatalf("repeated=%#v err=%v", repeated, err)
	}
	if _, err := manager.RevokeDevice(signedRevocation(t, owner, device.deviceID, "owner-shell-0003")); err != nil {
		t.Fatal(err)
	}
	list.RequestID = "owner-shell-0004"
	list.Signature = EncodeSignature(ed25519.Sign(owner.private, ShellCapabilityListMessage(list)))
	listed, err = manager.ListShellCapabilities(list)
	if err != nil || len(listed.Capabilities) != 0 {
		t.Fatalf("listed after revocation=%#v err=%v", listed, err)
	}
}

func TestPollDevice_TooFast_IncreasesRequiredInterval(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)}
	manager := newManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{0x34}, 4096)))
	owner := makeKeyPair(t, 0x31)
	device := makeKeyPair(t, 0x32)
	initializeOwner(t, manager, owner)
	codes, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, device, "Node", "device-request-1001"))
	if err != nil {
		t.Fatal(err)
	}
	poll := DevicePollRequest{SchemaVersion: SchemaVersion, DeviceCode: codes.DeviceCode, ClientVersion: "v1.0.0"}
	if _, err := manager.PollDevice(poll); faultCode(err) != "AUTHORIZATION_PENDING" {
		t.Fatalf("first poll error = %v", err)
	}
	if _, err := manager.PollDevice(poll); faultCode(err) != "SLOW_DOWN" {
		t.Fatalf("fast poll error = %v", err)
	}
	clock.Advance(10 * time.Second)
	if _, err := manager.ApproveDevice(signedApproval(t, owner, codes.UserCode, "owner-request-1001")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PollDevice(poll); err != nil {
		t.Fatal(err)
	}
}

func TestApproveDevice_ExpiredCode_FailsClosed(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)}
	manager := newManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{0x45}, 4096)))
	owner := makeKeyPair(t, 0x41)
	device := makeKeyPair(t, 0x42)
	initializeOwner(t, manager, owner)
	codes, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, device, "Node", "device-request-2001"))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(DefaultActivationTTL)
	if _, err := manager.ApproveDevice(signedApproval(t, owner, codes.UserCode, "owner-request-2001")); faultCode(err) != "ACTIVATION_EXPIRED" {
		t.Fatalf("expired approval error = %v", err)
	}
	if _, err := manager.PollDevice(DevicePollRequest{SchemaVersion: SchemaVersion, DeviceCode: codes.DeviceCode, ClientVersion: "1.0.0"}); faultCode(err) != "INVALID_DEVICE_CODE" {
		t.Fatalf("expired code remained usable: %v", err)
	}
}

func TestApproveDevice_InvalidCodes_AreRateLimited(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)}
	manager := newManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{0x56}, 4096)))
	owner := makeKeyPair(t, 0x51)
	initializeOwner(t, manager, owner)
	for index := 0; index < 10; index++ {
		request := signedApproval(t, owner, "BAD-CODE", fmt.Sprintf("owner-invalid-%04d", index))
		if _, err := manager.ApproveDevice(request); faultCode(err) != "INVALID_USER_CODE" {
			t.Fatalf("attempt %d error = %v", index, err)
		}
	}
	request := signedApproval(t, owner, "BAD-CODE", "owner-invalid-9999")
	if _, err := manager.ApproveDevice(request); faultCode(err) != "USER_CODE_RATE_LIMITED" {
		t.Fatalf("rate-limit error = %v", err)
	}
	clock.Advance(time.Minute)
	request = signedApproval(t, owner, "BAD-CODE", "owner-invalid-next1")
	if _, err := manager.ApproveDevice(request); faultCode(err) != "INVALID_USER_CODE" {
		t.Fatalf("rate limit did not reset: %v", err)
	}
}

func TestInitializeOwner_InvalidSignature_IsRejected(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x61)
	request := ownerInitialization(owner)
	request.Signature = EncodeSignature(bytes.Repeat([]byte{0x7f}, ed25519.SignatureSize))
	if _, err := manager.InitializeOwner(request); faultCode(err) != "INVALID_SIGNATURE" {
		t.Fatalf("invalid signature error = %v", err)
	}
}

func TestInitializeOwner_SecondOwner_IsRejected(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x71)
	initializeOwner(t, manager, owner)
	if _, err := manager.InitializeOwner(ownerInitialization(owner)); faultCode(err) != "OWNER_ALREADY_INITIALIZED" {
		t.Fatalf("second initialization error = %v", err)
	}
}

func TestBeginDeviceAuthorization_ReplayedProof_IsRejected(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x21)
	device := makeKeyPair(t, 0x22)
	initializeOwner(t, manager, owner)
	request := signedDeviceAuthorization(t, device, "Node", "device-request-3001")
	if _, err := manager.BeginDeviceAuthorization(request); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BeginDeviceAuthorization(request); faultCode(err) != "REQUEST_REPLAYED" {
		t.Fatalf("replay error = %v", err)
	}
}

func TestBeginDeviceAuthorization_BoundsPendingActivations(t *testing.T) {
	manager := NewManager()
	manager.pendingLimit = 1
	owner := makeKeyPair(t, 0x23)
	first := makeKeyPair(t, 0x24)
	second := makeKeyPair(t, 0x25)
	initializeOwner(t, manager, owner)
	if _, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, first, "First", "device-capacity-0001")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, first, "First", "device-capacity-0002")); faultCode(err) != "ACTIVATION_ALREADY_PENDING" {
		t.Fatalf("duplicate pending activation error = %v", err)
	}
	if _, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, second, "Second", "device-capacity-0003")); faultCode(err) != "ACTIVATION_CAPACITY_REACHED" {
		t.Fatalf("capacity error = %v", err)
	}
}

func TestBeginDeviceAuthorization_PrunesExpiredActivationAndReplayBucket(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)}
	manager := newManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{0x37}, 4096)))
	manager.pendingLimit = 1
	owner := makeKeyPair(t, 0x26)
	first := makeKeyPair(t, 0x27)
	second := makeKeyPair(t, 0x28)
	initializeOwner(t, manager, owner)
	if _, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, first, "First", "device-pruning-0001")); err != nil {
		t.Fatal(err)
	}
	clock.Advance(deviceReplayRetention + time.Second)
	if _, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, second, "Second", "device-pruning-0002")); err != nil {
		t.Fatal(err)
	}
	if len(manager.activations) != 1 {
		t.Fatalf("pending activations = %d", len(manager.activations))
	}
	if _, exists := manager.deviceRequestIDs[first.deviceID]; exists {
		t.Fatal("expired unregistered Device replay bucket was retained")
	}
}

func TestBeginDeviceAuthorization_EntropyFailure_IsSafe(t *testing.T) {
	manager := newManager(time.Now, failingReader{})
	owner := makeKeyPair(t, 0x31)
	device := makeKeyPair(t, 0x32)
	initializeOwner(t, manager, owner)
	request := signedDeviceAuthorization(t, device, "Node", "device-request-4001")
	if _, err := manager.BeginDeviceAuthorization(request); faultCode(err) != "ENTROPY_UNAVAILABLE" {
		t.Fatalf("entropy error = %v", err)
	}
}

func TestVersionValidation_UnsupportedMajor_IsRejected(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x41)
	request := ownerInitialization(owner)
	request.ClientVersion = "2.0.0"
	request.Signature = EncodeSignature(ed25519.Sign(owner.private, OwnerInitializationMessage(request)))
	if _, err := manager.InitializeOwner(request); faultCode(err) != "UNSUPPORTED_CLIENT_VERSION" {
		t.Fatalf("version error = %v", err)
	}
}

func TestOwnerAction_InvalidSignature_DoesNotRevealDeviceState(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x51)
	initializeOwner(t, manager, owner)
	request := OwnerRevocationRequest{
		SchemaVersion: SchemaVersion, OwnerDeviceID: owner.deviceID,
		DeviceID: "device-does-not-exist", RequestID: "owner-request-5001",
		ClientVersion: "1.0.0", Signature: EncodeSignature(bytes.Repeat([]byte{0x77}, ed25519.SignatureSize)),
	}
	if _, err := manager.RevokeDevice(request); faultCode(err) != "INVALID_SIGNATURE" {
		t.Fatalf("unauthenticated state lookup error = %v", err)
	}
}

func TestManager_UninitializedActions_FailClosed(t *testing.T) {
	manager := NewManager()
	if _, err := manager.BeginDeviceAuthorization(DeviceAuthorizationRequest{}); faultCode(err) != "OWNER_NOT_INITIALIZED" {
		t.Fatalf("authorization error = %v", err)
	}
	if _, err := manager.PollDevice(DevicePollRequest{}); faultCode(err) != "OWNER_NOT_INITIALIZED" {
		t.Fatalf("poll error = %v", err)
	}
	if _, err := manager.RevokeDevice(OwnerRevocationRequest{}); faultCode(err) != "OWNER_NOT_INITIALIZED" {
		t.Fatalf("revocation error = %v", err)
	}
	if _, err := manager.SyncVersion(VersionSyncRequest{}); faultCode(err) != "OWNER_NOT_INITIALIZED" {
		t.Fatalf("sync error = %v", err)
	}
}

func TestBeginDeviceAuthorization_InvalidDeviceBinding_IsRejected(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x61)
	device := makeKeyPair(t, 0x62)
	initializeOwner(t, manager, owner)
	request := signedDeviceAuthorization(t, device, "Node", "device-request-6001")
	request.DeviceID = "device-invalid"
	request.Signature = EncodeSignature(ed25519.Sign(device.private, DeviceAuthorizationMessage(request)))
	if _, err := manager.BeginDeviceAuthorization(request); faultCode(err) != "DEVICE_IDENTITY_MISMATCH" {
		t.Fatalf("binding error = %v", err)
	}
}

func TestRevokeDevice_OwnerDeviceRequiresRecovery(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x71)
	initializeOwner(t, manager, owner)
	request := signedRevocation(t, owner, owner.deviceID, "owner-request-7001")
	if _, err := manager.RevokeDevice(request); faultCode(err) != "OWNER_DEVICE_REQUIRES_RECOVERY" {
		t.Fatalf("owner revocation error = %v", err)
	}
}

func TestPollDevice_InvalidCode_IsRejected(t *testing.T) {
	manager := NewManager()
	owner := makeKeyPair(t, 0x31)
	initializeOwner(t, manager, owner)
	if _, err := manager.PollDevice(DevicePollRequest{
		SchemaVersion: SchemaVersion, DeviceCode: "invalid-device-code", ClientVersion: "1.0.0",
	}); faultCode(err) != "INVALID_DEVICE_CODE" {
		t.Fatalf("poll error = %v", err)
	}
}

func TestEncodingHelpers_ProduceURLSafeValues(t *testing.T) {
	key := makeKeyPair(t, 0x41)
	if EncodePublicKey(key.public) == "" {
		t.Fatal("public key encoding is empty")
	}
	signature := ed25519.Sign(key.private, []byte("message"))
	encoded := EncodeSignature(signature)
	if encoded == "" || bytes.Contains([]byte(encoded), []byte("=")) {
		t.Fatalf("signature encoding = %q", encoded)
	}
}

func makeKeyPair(t *testing.T, seedByte byte) keyPair {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seedByte}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return keyPair{deviceID: deviceID, public: public, private: private}
}

func initializeOwner(t *testing.T, manager *Manager, owner keyPair) OwnerInitializationResponse {
	t.Helper()
	response, err := manager.InitializeOwner(ownerInitialization(owner))
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func ownerInitialization(owner keyPair) OwnerInitializationRequest {
	request := OwnerInitializationRequest{
		SchemaVersion: SchemaVersion, FabricID: "fabric-synthetic",
		OwnerDeviceID: owner.deviceID, OwnerPublicKey: EncodePublicKey(owner.public), ClientVersion: "1.0.0",
	}
	request.Signature = EncodeSignature(ed25519.Sign(owner.private, OwnerInitializationMessage(request)))
	return request
}

func signedDeviceAuthorization(t *testing.T, device keyPair, name, requestID string) DeviceAuthorizationRequest {
	t.Helper()
	request := DeviceAuthorizationRequest{
		SchemaVersion: SchemaVersion, DeviceID: device.deviceID, DeviceName: name,
		DevicePublicKey: EncodePublicKey(device.public), ClientVersion: "1.0.0", RequestID: requestID,
	}
	request.Signature = EncodeSignature(ed25519.Sign(device.private, DeviceAuthorizationMessage(request)))
	return request
}

func signedApproval(t *testing.T, owner keyPair, userCode, requestID string) OwnerApprovalRequest {
	t.Helper()
	request := OwnerApprovalRequest{
		SchemaVersion: SchemaVersion, OwnerDeviceID: owner.deviceID, UserCode: userCode,
		RequestID: requestID, ClientVersion: "1.0.0",
	}
	request.Signature = EncodeSignature(ed25519.Sign(owner.private, OwnerApprovalMessage(request)))
	return request
}

func signedRevocation(t *testing.T, owner keyPair, deviceID, requestID string) OwnerRevocationRequest {
	t.Helper()
	request := OwnerRevocationRequest{
		SchemaVersion: SchemaVersion, OwnerDeviceID: owner.deviceID, DeviceID: deviceID,
		RequestID: requestID, ClientVersion: "1.0.0",
	}
	request.Signature = EncodeSignature(ed25519.Sign(owner.private, OwnerRevocationMessage(request)))
	return request
}

func signedVersionSync(t *testing.T, device keyPair, requestID string) VersionSyncRequest {
	t.Helper()
	request := VersionSyncRequest{
		SchemaVersion: SchemaVersion, DeviceID: device.deviceID,
		RequestID: requestID, ClientVersion: "1.0.0",
	}
	request.Signature = EncodeSignature(ed25519.Sign(device.private, VersionSyncMessage(request)))
	return request
}

func signedShellPublication(t *testing.T, device keyPair, requestID string, version uint64) ShellCapabilityPublishRequest {
	t.Helper()
	binding, err := shellbinding.Sign("fabric-synthetic", "shell-main", version, []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: "AAAAC3NzaC1lZDI1NTE5AAAAIMvF3FK8rr2A2r9iVfj0x8l26LThB5GXxJ7XyQPRZP5Z"}}, device)
	if err != nil {
		t.Fatal(err)
	}
	request := ShellCapabilityPublishRequest{SchemaVersion: SchemaVersion, DeviceID: device.deviceID, Binding: *binding, RequestID: requestID, ClientVersion: "1.0.0"}
	request.Signature = EncodeSignature(ed25519.Sign(device.private, ShellCapabilityPublishMessage(request)))
	return request
}

func signedShellList(owner keyPair, requestID string) ShellCapabilityListRequest {
	request := ShellCapabilityListRequest{SchemaVersion: SchemaVersion, OwnerDeviceID: owner.deviceID, RequestID: requestID, ClientVersion: "1.0.0"}
	request.Signature = EncodeSignature(ed25519.Sign(owner.private, ShellCapabilityListMessage(request)))
	return request
}

func faultCode(err error) string {
	var enrollmentFault *Fault
	if errors.As(err, &enrollmentFault) {
		return enrollmentFault.Code
	}
	return ""
}
