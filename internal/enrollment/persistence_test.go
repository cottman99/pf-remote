package enrollment

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type memoryStateStore struct {
	payload   []byte
	loadErr   error
	saveErr   error
	saveCount int
	deviceID  string
	status    string
	directory uint64
	grants    uint64
}

func (s *memoryStateStore) LoadControlState(context.Context, string) ([]byte, error) {
	return append([]byte(nil), s.payload...), s.loadErr
}

func (s *memoryStateStore) SaveControlState(_ context.Context, _ string, payload []byte) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.payload = append([]byte(nil), payload...)
	s.saveCount++
	return nil
}

func (s *memoryStateStore) SaveControlStateWithDeviceStatus(ctx context.Context, schema string, payload []byte, deviceID, status string, directory, grants uint64) error {
	if err := s.SaveControlState(ctx, schema, payload); err != nil {
		return err
	}
	s.deviceID, s.status = deviceID, status
	s.directory, s.grants = directory, grants
	return nil
}

func TestPersistentManager_RestartPreservesApprovalReplayAndRevocation(t *testing.T) {
	store := &memoryStateStore{}
	manager, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	owner := makeKeyPair(t, 0x61)
	device := makeKeyPair(t, 0x62)
	initializeOwner(t, manager, owner)
	codes, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, device, "Persistent node", "device-persist-0001"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApproveDevice(signedApproval(t, owner, codes.UserCode, "owner-persist-0001")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(store.payload), codes.DeviceCode) || strings.Contains(string(store.payload), codes.UserCode) {
		t.Fatal("persistent state contains a raw activation code")
	}

	restarted, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	activated, err := restarted.PollDevice(DevicePollRequest{SchemaVersion: SchemaVersion, DeviceCode: codes.DeviceCode, ClientVersion: "1.0.0"})
	if err != nil || activated.Status != "active" {
		t.Fatalf("activation after restart = %#v, %v", activated, err)
	}
	if _, err := restarted.PublishShellCapability(signedShellPublication(t, device, "device-persist-shell-0001", 1)); err != nil {
		t.Fatal(err)
	}
	sync := signedVersionSync(t, device, "device-persist-0002")
	if _, err := restarted.SyncVersion(sync); err != nil {
		t.Fatal(err)
	}

	restartedAgain, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := restartedAgain.ListShellCapabilities(signedShellList(owner, "owner-persist-shell-0001"))
	if err != nil || len(listed.Capabilities) != 1 || listed.Capabilities[0].Binding.CapabilityID != "shell-main" {
		t.Fatalf("persistent Shell bindings=%#v err=%v", listed, err)
	}
	if _, err := restartedAgain.SyncVersion(sync); faultCode(err) != "REQUEST_REPLAYED" {
		t.Fatalf("replayed request after restart = %v", err)
	}
	if _, err := restartedAgain.RevokeDevice(signedRevocation(t, owner, device.deviceID, "owner-persist-0002")); err != nil {
		t.Fatal(err)
	}
	if store.deviceID != device.deviceID || store.status != "revoked" || store.directory == 0 || store.grants == 0 {
		t.Fatalf("revocation status update = device:%q status:%q directory:%d grants:%d", store.deviceID, store.status, store.directory, store.grants)
	}

	finalRestart, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalRestart.SyncVersion(signedVersionSync(t, device, "device-persist-0003")); faultCode(err) != "DEVICE_REVOKED" {
		t.Fatalf("revoked Device after restart = %v", err)
	}
	if store.saveCount < 6 {
		t.Fatalf("save count = %d, want lifecycle state saved at each mutation", store.saveCount)
	}
}

func TestPersistentManager_CorruptStateFailsClosed(t *testing.T) {
	store := &memoryStateStore{payload: []byte(`{"schema_version":"pfremote.enrollment-state/v1","unexpected":true}`)}
	if _, err := NewPersistentManager(store); err == nil {
		t.Fatal("corrupt control state was accepted")
	}
}

func TestPrepareRecoveryState_DropsEphemeralEnrollmentData(t *testing.T) {
	store := &memoryStateStore{}
	manager, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	owner := makeKeyPair(t, 0x63)
	device := makeKeyPair(t, 0x64)
	initializeOwner(t, manager, owner)
	if _, err := manager.BeginDeviceAuthorization(signedDeviceAuthorization(t, device, "Temporary node", "device-recovery-0001")); err != nil {
		t.Fatal(err)
	}
	manager.recordOwnerRequestID("owner-recovery-0001")
	if err := manager.persistLocked(); err != nil {
		t.Fatal(err)
	}
	prepared, summary, err := PrepareRecoveryState(store.payload)
	if err != nil {
		t.Fatal(err)
	}
	if summary.OwnerDeviceID != owner.deviceID || summary.FabricID == "" || summary.DeviceStatuses[owner.deviceID] != "active" {
		t.Fatalf("summary = %#v", summary)
	}
	var state persistentState
	if err := json.Unmarshal(prepared, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Activations) != 0 || len(state.OwnerRequestIDs) != 0 || len(state.DeviceRequestIDs) != 0 || state.FailedCodeAttempts != 0 {
		t.Fatalf("ephemeral state was retained: %#v", state)
	}
	if restored := newManager(time.Now, rand.Reader); restored.restore(prepared) != nil {
		t.Fatal("prepared recovery state did not restore")
	}
}

func TestPersistentManager_SaveFailureMakesManagerUnhealthy(t *testing.T) {
	store := &memoryStateStore{saveErr: errors.New("synthetic disk failure")}
	manager, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	owner := makeKeyPair(t, 0x71)
	request := ownerInitialization(owner)
	if _, err := manager.InitializeOwner(request); faultCode(err) != "STATE_PERSISTENCE_FAILED" {
		t.Fatalf("initial save failure = %v", err)
	}
	store.saveErr = nil
	if _, err := manager.InitializeOwner(request); faultCode(err) != "STATE_PERSISTENCE_FAILED" {
		t.Fatalf("unhealthy manager retry = %v", err)
	}
}

func TestNewPersistentManager_StoreLoadFailureIsRedacted(t *testing.T) {
	store := &memoryStateStore{loadErr: errors.New(`open C:\private\state.db: denied`)}
	_, err := NewPersistentManager(store)
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("load error = %v", err)
	}
}

func TestReplayWindows_AreBoundedAndDiscardOldestIDs(t *testing.T) {
	manager := NewManager()
	for index := 0; index <= maxOwnerReplayIDs; index++ {
		manager.recordOwnerRequestID(fmt.Sprintf("owner-request-%05d", index))
	}
	if len(manager.ownerRequestIDs) != maxOwnerReplayIDs || len(manager.ownerRequestOrder) != maxOwnerReplayIDs {
		t.Fatalf("Owner replay window sizes = %d/%d", len(manager.ownerRequestIDs), len(manager.ownerRequestOrder))
	}
	if _, exists := manager.ownerRequestIDs["owner-request-00000"]; exists {
		t.Fatal("oldest Owner request ID was not discarded")
	}
	for index := 0; index <= maxDeviceReplayIDs; index++ {
		manager.recordDeviceRequestID("device-synthetic", fmt.Sprintf("device-request-%05d", index))
	}
	if len(manager.deviceRequestIDs["device-synthetic"]) != maxDeviceReplayIDs || len(manager.deviceRequestOrder["device-synthetic"]) != maxDeviceReplayIDs {
		t.Fatalf("Device replay window sizes = %d/%d", len(manager.deviceRequestIDs["device-synthetic"]), len(manager.deviceRequestOrder["device-synthetic"]))
	}
	if _, exists := manager.deviceRequestIDs["device-synthetic"]["device-request-00000"]; exists {
		t.Fatal("oldest Device request ID was not discarded")
	}
}
