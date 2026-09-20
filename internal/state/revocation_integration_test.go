package state_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestGatewayRevocationImmediatelyRemovesDaemonTarget(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ownerPublic, ownerPrivate := integrationKey(t)
	targetPublic, targetPrivate := integrationKey(t)
	ownerID := integrationDeviceID(t, ownerPublic)
	targetID := integrationDeviceID(t, targetPublic)
	capturedAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	snapshot := state.Snapshot{
		SchemaVersion: state.SnapshotSchema, FabricID: "fabric-integration",
		DirectoryVersion: 1, GrantVersion: 1, CapturedAt: capturedAt,
		Devices: []contracts.Device{
			{ID: ownerID, Alias: "owner", DisplayName: "Owner", State: "online"},
			{ID: targetID, Alias: "target", DisplayName: "Target", State: "online"},
		},
		Capabilities: []contracts.Capability{{
			ID: "shell-main", DeviceID: targetID, Alias: "shell", DisplayName: "Shell",
			Kind: contracts.CapabilityShell, State: "available",
		}},
		Grants: []contracts.Grant{{
			ID: "grant-owner-shell", SubjectDeviceID: ownerID, CapabilityID: "shell-main", State: "active",
		}},
	}
	if err := store.CommitSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	manager, err := enrollment.NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	initialize := enrollment.OwnerInitializationRequest{
		SchemaVersion: enrollment.SchemaVersion, FabricID: snapshot.FabricID,
		OwnerDeviceID: ownerID, OwnerPublicKey: enrollment.EncodePublicKey(ownerPublic), ClientVersion: "1.0.0",
	}
	initialize.Signature = enrollment.EncodeSignature(ed25519.Sign(ownerPrivate, enrollment.OwnerInitializationMessage(initialize)))
	if _, err := manager.InitializeOwner(initialize); err != nil {
		t.Fatal(err)
	}
	authorize := enrollment.DeviceAuthorizationRequest{
		SchemaVersion: enrollment.SchemaVersion, DeviceID: targetID, DeviceName: "Target",
		DevicePublicKey: enrollment.EncodePublicKey(targetPublic), RequestID: "integration-device-0001", ClientVersion: "1.0.0",
	}
	authorize.Signature = enrollment.EncodeSignature(ed25519.Sign(targetPrivate, enrollment.DeviceAuthorizationMessage(authorize)))
	codes, err := manager.BeginDeviceAuthorization(authorize)
	if err != nil {
		t.Fatal(err)
	}
	approve := enrollment.OwnerApprovalRequest{
		SchemaVersion: enrollment.SchemaVersion, OwnerDeviceID: ownerID, UserCode: codes.UserCode,
		RequestID: "integration-owner-0001", ClientVersion: "1.0.0",
	}
	approve.Signature = enrollment.EncodeSignature(ed25519.Sign(ownerPrivate, enrollment.OwnerApprovalMessage(approve)))
	if _, err := manager.ApproveDevice(approve); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PollDevice(enrollment.DevicePollRequest{
		SchemaVersion: enrollment.SchemaVersion, DeviceCode: codes.DeviceCode, ClientVersion: "1.0.0",
	}); err != nil {
		t.Fatal(err)
	}
	provider := func() (actions.Service, error) {
		current, _, err := store.LoadLatestValidSnapshot(ctx)
		if err != nil {
			return actions.Service{}, err
		}
		service := actions.New()
		service.DeviceID = ownerID
		service.Catalog = catalog.New(
			current.FabricID, current.Devices, current.Capabilities, current.Grants,
			ownerID, current.CapturedAt, func() time.Time { return capturedAt.Add(time.Hour) },
		)
		return service, nil
	}
	handler := localapi.Handler{Provider: provider}
	listed := handler.Handle(localapi.Request{SchemaVersion: localapi.SchemaVersion, Action: "list"})
	if listed.Error != nil || len(listed.Result.(contracts.CatalogResponse).Targets) != 1 {
		t.Fatalf("target before revocation = %#v", listed)
	}
	revoke := enrollment.OwnerRevocationRequest{
		SchemaVersion: enrollment.SchemaVersion, OwnerDeviceID: ownerID, DeviceID: targetID,
		RequestID: "integration-owner-0002", ClientVersion: "1.0.0",
	}
	revoke.Signature = enrollment.EncodeSignature(ed25519.Sign(ownerPrivate, enrollment.OwnerRevocationMessage(revoke)))
	if _, err := manager.RevokeDevice(revoke); err != nil {
		t.Fatal(err)
	}
	listed = handler.Handle(localapi.Request{SchemaVersion: localapi.SchemaVersion, Action: "list"})
	if listed.Error != nil || len(listed.Result.(contracts.CatalogResponse).Targets) != 0 {
		t.Fatalf("target after revocation = %#v", listed)
	}
	inspected := handler.Handle(localapi.Request{
		SchemaVersion: localapi.SchemaVersion, Action: "inspect",
		Target: "pfremote://fabric-integration/devices/" + targetID + "/capabilities/shell-main",
	})
	if inspected.Error == nil || inspected.Error.Code != "TARGET_NOT_FOUND" {
		t.Fatalf("revoked target inspection = %#v", inspected)
	}
	loaded, _, err := store.LoadLatestValidSnapshot(ctx)
	if err != nil || !loaded.CapturedAt.Equal(capturedAt) {
		t.Fatalf("revocation moved cached authorization boundary: %s, %v", loaded.CapturedAt, err)
	}
}

func integrationKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return publicKey, privateKey
}

func integrationDeviceID(t *testing.T, publicKey ed25519.PublicKey) string {
	t.Helper()
	deviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return deviceID
}
