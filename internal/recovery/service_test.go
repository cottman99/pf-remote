package recovery

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestExportRestoreToNewDatabase_LossAndRecovery(t *testing.T) {
	ctx := context.Background()
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	source, err := state.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	control, snapshot := recoveryFixture(t, "fabric-recovery", 1, 1)
	if err := source.SaveControlStateWithDeviceStatus(ctx, enrollment.PersistentStateSchema, control,
		snapshot.Devices[0].ID, "active", snapshot.DirectoryVersion, snapshot.GrantVersion); err != nil {
		t.Fatal(err)
	}
	if err := source.CommitSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, summary, err := export(ctx, source, "correct horse battery staple", 100_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if summary.DeviceCount != 2 || summary.OwnerDeviceID != snapshot.Devices[0].ID {
		t.Fatalf("export summary = %#v", summary)
	}

	destination := filepath.Join(t.TempDir(), "restored.db")
	restoredSummary, err := RestoreToNewDatabase(ctx, destination, encoded, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if restoredSummary != summary {
		t.Fatalf("restore summary = %#v, want %#v", restoredSummary, summary)
	}
	restored, err := state.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	loaded, _, err := restored.LoadLatestValidSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerFound := false
	for _, device := range loaded.Devices {
		if device.ID == summary.OwnerDeviceID {
			ownerFound = true
		}
	}
	if loaded.FabricID != snapshot.FabricID || !ownerFound {
		t.Fatalf("restored snapshot = %#v", loaded)
	}
	revokedFound := false
	for _, device := range loaded.Devices {
		if device.State == "revoked" {
			revokedFound = true
		}
	}
	if !revokedFound {
		t.Fatal("restored snapshot lost the revoked Device state")
	}
	manager, err := enrollment.NewPersistentManager(restored)
	if err != nil {
		t.Fatalf("restored enrollment state = %v", err)
	}
	_ = manager
}

func TestRestoreRejectsWrongPasswordThenAtomicallyReplacesUninitializedGatewayAndRejectsReplay(t *testing.T) {
	ctx := context.Background()
	source, err := state.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	control, snapshot := recoveryFixture(t, "fabric-recovery", 1, 1)
	if err := source.SaveControlStateWithDeviceStatus(ctx, enrollment.PersistentStateSchema, control,
		snapshot.Devices[0].ID, "active", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := source.CommitSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, _, err := export(ctx, source, "correct horse battery staple", 100_000)
	if err != nil {
		t.Fatal(err)
	}
	source.Close()

	wrongDestination := filepath.Join(t.TempDir(), "wrong.db")
	if _, err := RestoreToNewDatabase(ctx, wrongDestination, encoded, "wrong password"); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("wrong password error = %v", err)
	}
	wrongStore, err := state.Open(wrongDestination)
	if err != nil {
		// Opening now is the proof that the failed restore did not publish a damaged file.
		t.Fatalf("wrong-password destination was not absent/usable: %v", err)
	}
	wrongStore.Close()

	existingPath := filepath.Join(t.TempDir(), "existing.db")
	existing, err := state.Open(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := existing.CommitSnapshot(ctx, state.SyntheticSnapshot("device-owner-existing", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := existing.RecordRecentSession(ctx, contracts.RecentSession{
		SessionID: "session-before-recovery", CanonicalTarget: "pfremote://fabric-example/devices/device-a/capabilities/shell-a",
		Action: "exec", Status: "completed", StartedAt: time.Now().UTC(),
	}, 20); err != nil {
		t.Fatal(err)
	}
	existing.Close()
	if _, err := RestoreToNewDatabase(ctx, existingPath, encoded, "correct horse battery staple"); err != nil {
		t.Fatalf("replace uninitialized Gateway = %v", err)
	}
	replaced, err := state.Open(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := replaced.LoadLatestValidSnapshot(ctx)
	if err != nil || loaded.FabricID != "fabric-recovery" {
		t.Fatalf("Gateway state was not replaced: fabric=%q err=%v", loaded.FabricID, err)
	}
	recent, err := replaced.LoadRecentSessions(ctx, 20)
	if err != nil || len(recent) != 0 {
		t.Fatalf("recovery retained local recent activity: entries=%#v err=%v", recent, err)
	}
	replaced.Close()
	if _, err := RestoreToNewDatabase(ctx, existingPath, encoded, "correct horse battery staple"); !errors.Is(err, ErrRollback) {
		t.Fatalf("replayed recovery error = %v", err)
	}
	unchanged, err := state.Open(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unchanged.Close()
	loaded, _, err = unchanged.LoadLatestValidSnapshot(ctx)
	if err != nil || loaded.FabricID != "fabric-recovery" {
		t.Fatalf("replay changed Gateway state: fabric=%q err=%v", loaded.FabricID, err)
	}
}

func TestRestoreRejectsWrongFabricAndOlderVersionsWithoutChangingLiveState(t *testing.T) {
	ctx := context.Background()
	source, err := state.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	control, snapshot := recoveryFixture(t, "fabric-recovery", 1, 1)
	if err := source.SaveControlStateWithDeviceStatus(ctx, enrollment.PersistentStateSchema, control, snapshot.Devices[0].ID, "active", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := source.CommitSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, _, err := export(ctx, source, "correct horse battery staple", 100_000)
	if err != nil {
		t.Fatal(err)
	}
	source.Close()

	wrongFabricPath := filepath.Join(t.TempDir(), "wrong-fabric.db")
	wrongFabric, err := state.Open(wrongFabricPath)
	if err != nil {
		t.Fatal(err)
	}
	wrongControl, wrongSnapshot := recoveryFixture(t, "fabric-other", 2, 2)
	if err := wrongFabric.SaveControlStateWithDeviceStatus(ctx, enrollment.PersistentStateSchema, wrongControl, wrongSnapshot.Devices[0].ID, "active", 2, 2); err != nil {
		t.Fatal(err)
	}
	if err := wrongFabric.CommitSnapshot(ctx, wrongSnapshot); err != nil {
		t.Fatal(err)
	}
	wrongFabric.Close()
	if _, err := RestoreToNewDatabase(ctx, wrongFabricPath, encoded, "correct horse battery staple"); !errors.Is(err, ErrWrongFabric) {
		t.Fatalf("wrong Fabric error = %v", err)
	}

	newerPath := filepath.Join(t.TempDir(), "newer.db")
	newer, err := state.Open(newerPath)
	if err != nil {
		t.Fatal(err)
	}
	newerControl, newerSnapshot := recoveryFixture(t, "fabric-recovery", 2, 2)
	if err := newer.SaveControlStateWithDeviceStatus(ctx, enrollment.PersistentStateSchema, newerControl, newerSnapshot.Devices[0].ID, "active", 2, 2); err != nil {
		t.Fatal(err)
	}
	if err := newer.CommitSnapshot(ctx, newerSnapshot); err != nil {
		t.Fatal(err)
	}
	newer.Close()
	if _, err := RestoreToNewDatabase(ctx, newerPath, encoded, "correct horse battery staple"); !errors.Is(err, ErrRollback) {
		t.Fatalf("downgrade error = %v", err)
	}
	unchanged, err := state.Open(newerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unchanged.Close()
	loaded, _, err := unchanged.LoadLatestValidSnapshot(ctx)
	if err != nil || loaded.DirectoryVersion != 2 || loaded.GrantVersion != 2 {
		t.Fatalf("downgrade changed live versions: %#v err=%v", loaded, err)
	}
}

func recoveryFixture(t *testing.T, fabricID string, directoryVersion, grantVersion uint64) ([]byte, state.Snapshot) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	deviceID, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey := base64.RawURLEncoding.EncodeToString(publicKey)
	revokedSeed := make([]byte, ed25519.SeedSize)
	for index := range revokedSeed {
		revokedSeed[index] = byte(index + 41)
	}
	revokedPublicKey := ed25519.NewKeyFromSeed(revokedSeed).Public().(ed25519.PublicKey)
	revokedDeviceID, err := identity.DeviceIDFromPublicKey(revokedPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	revokedEncodedKey := base64.RawURLEncoding.EncodeToString(revokedPublicKey)
	control, err := json.Marshal(map[string]any{
		"schema_version": enrollment.PersistentStateSchema,
		"fabric_id":      fabricID, "owner_device_id": deviceID, "owner_public_key": encodedKey,
		"directory_version": directoryVersion, "grant_version": grantVersion,
		"devices": []map[string]string{
			{"device_id": deviceID, "device_name": "Owner laptop", "public_key": encodedKey, "status": "active"},
			{"device_id": revokedDeviceID, "device_name": "Old laptop", "public_key": revokedEncodedKey, "status": "revoked"},
		},
		"failed_code_attempts": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := state.Snapshot{
		SchemaVersion: state.SnapshotSchema, FabricID: fabricID, DirectoryVersion: directoryVersion, GrantVersion: grantVersion,
		CapturedAt: time.Date(2026, 8, 29, 5, 0, 0, 0, time.UTC),
		Devices: []contracts.Device{
			{ID: deviceID, Alias: "owner-laptop", DisplayName: "Owner laptop", State: "online", IdentityPublicKey: encodedKey},
			{ID: revokedDeviceID, Alias: "old-laptop", DisplayName: "Old laptop", State: "revoked", IdentityPublicKey: revokedEncodedKey},
		},
	}
	return control, snapshot
}
