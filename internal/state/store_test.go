package state

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestStore_CommitAndReloadSnapshot_RoundTripsNormalizedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))
	if err := store.CommitSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, recovered, err := reopened.LoadLatestValidSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if recovered || loaded.FabricID != snapshot.FabricID || loaded.DirectoryVersion != 1 || loaded.GrantVersion != 1 {
		t.Fatalf("loaded snapshot = %#v, recovered = %t", loaded, recovered)
	}
	if len(loaded.Devices) != len(snapshot.Devices) || len(loaded.Capabilities) != 4 || len(loaded.Grants) != 4 {
		t.Fatalf("loaded counts = devices:%d capabilities:%d grants:%d", len(loaded.Devices), len(loaded.Capabilities), len(loaded.Grants))
	}
	var desktopProfile *contracts.DesktopProfile
	for _, capability := range loaded.Capabilities {
		if capability.ID == "desktop-main" {
			desktopProfile = capability.DesktopProfile
		}
	}
	if desktopProfile == nil || desktopProfile.Protocol != "rdp" || desktopProfile.Authentication != "windows-sso" {
		t.Fatalf("loaded Desktop profile = %#v", desktopProfile)
	}
}

func TestStore_RecentSessionsSurviveRestartAndStayBounded(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	for index := 1; index <= 3; index++ {
		entry := contracts.RecentSession{
			SessionID:       fmt.Sprintf("session-%d", index),
			CanonicalTarget: "pfremote://fabric-example/devices/device-a/capabilities/shell-a",
			Action:          "exec",
			Status:          "completed",
			StartedAt:       baseTime.Add(time.Duration(index) * time.Minute),
		}
		if err := store.RecordRecentSession(ctx, entry, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	entries, err := reopened.LoadRecentSessions(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].SessionID != "session-3" || entries[1].SessionID != "session-2" {
		t.Fatalf("recent entries = %#v", entries)
	}
}

func TestStore_MigratesVersion4DatabaseForRecentActivity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state-v4.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE recent_activity`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM schema_migrations WHERE version = 5`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	entry := contracts.RecentSession{
		SessionID: "session-after-migration", CanonicalTarget: "pfremote://fabric-example/devices/device-a/capabilities/shell-a",
		Action: "exec", Status: "completed", StartedAt: time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC),
	}
	if err := migrated.RecordRecentSession(context.Background(), entry, 20); err != nil {
		t.Fatal(err)
	}
	entries, err := migrated.LoadRecentSessions(context.Background(), 20)
	if err != nil || len(entries) != 1 || entries[0].SessionID != entry.SessionID {
		t.Fatalf("migrated recent entries=%#v err=%v", entries, err)
	}
}

func TestStore_MigratesVersion3DatabaseForDesktopProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state-v3.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE capability_desktop_profiles`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM schema_migrations WHERE version = 4`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	snapshot := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))
	if err := migrated.CommitSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, recovered, err := migrated.LoadLatestValidSnapshot(context.Background())
	if err != nil || recovered {
		t.Fatalf("load err=%v recovered=%t", err, recovered)
	}
	var profile *contracts.DesktopProfile
	for _, capability := range loaded.Capabilities {
		if capability.ID == "desktop-main" {
			profile = capability.DesktopProfile
		}
	}
	if profile == nil || profile.RenderingEnvironment != "virtual" {
		t.Fatalf("migrated Desktop profile = %#v", profile)
	}
	var migrationCount int
	if err := migrated.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 4`).Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatalf("migration marker count=%d err=%v", migrationCount, err)
	}
}

func TestStore_MigratesVersion2DatabaseAndPreservesExistingSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state-v2.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	base := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))
	if err := store.CommitSnapshot(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE ssh_binding_versions`,
		`DROP TABLE capability_ssh_bindings`,
		`DROP TABLE device_identity_keys`,
		`DELETE FROM schema_migrations WHERE version = 3`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, recovered, err := migrated.LoadLatestValidSnapshot(context.Background())
	if err != nil || recovered || loaded.DirectoryVersion != base.DirectoryVersion {
		t.Fatalf("v2 load err=%v recovered=%t version=%d", err, recovered, loaded.DirectoryVersion)
	}
	signed := signedShellSnapshot(t, base.CapturedAt.Add(time.Minute), 1)
	if err := migrated.CommitSnapshot(context.Background(), signed); err != nil {
		t.Fatal(err)
	}
	var migrationCount int
	if err := migrated.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 3`).Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatalf("migration marker count=%d err=%v", migrationCount, err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, recovered, err = reopened.LoadLatestValidSnapshot(context.Background())
	if err != nil || recovered || loaded.DirectoryVersion != signed.DirectoryVersion {
		t.Fatalf("reopened load err=%v recovered=%t version=%d", err, recovered, loaded.DirectoryVersion)
	}
}

type snapshotSigner struct {
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func (s snapshotSigner) DeviceID() string             { return s.id }
func (s snapshotSigner) PublicKey() ed25519.PublicKey { return s.public }
func (s snapshotSigner) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(s.private, message), nil
}

func signedShellSnapshot(t *testing.T, capturedAt time.Time, version uint64) Snapshot {
	return signedShellSnapshotWithHostByte(t, capturedAt, version, 0x41)
}

func signedShellSnapshotWithHostByte(t *testing.T, capturedAt time.Time, version uint64, hostByte byte) Snapshot {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x68}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := snapshotSigner{id: deviceID, public: public, private: private}
	algorithm := "ssh-ed25519"
	var blob bytes.Buffer
	_ = binary.Write(&blob, binary.BigEndian, uint32(len(algorithm)))
	blob.WriteString(algorithm)
	_ = binary.Write(&blob, binary.BigEndian, uint32(ed25519.PublicKeySize))
	blob.Write(bytes.Repeat([]byte{hostByte}, ed25519.PublicKeySize))
	binding, err := shellbinding.Sign("fabric-demo", "shell-main", version, []contracts.SSHHostKey{{
		Algorithm: algorithm, PublicKey: base64.RawStdEncoding.EncodeToString(blob.Bytes()),
	}}, signer)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := SyntheticSnapshot("device-owner-synthetic", capturedAt)
	snapshot.DirectoryVersion = version
	snapshot.GrantVersion = version
	for index := range snapshot.Devices {
		if snapshot.Devices[index].ID == "device-compute" {
			snapshot.Devices[index].ID = deviceID
			snapshot.Devices[index].IdentityPublicKey = base64.RawURLEncoding.EncodeToString(public)
		}
	}
	for index := range snapshot.Capabilities {
		if snapshot.Capabilities[index].DeviceID == "device-compute" {
			snapshot.Capabilities[index].DeviceID = deviceID
		}
		if snapshot.Capabilities[index].ID == "shell-main" {
			snapshot.Capabilities[index].SSHBinding = binding
		}
	}
	return snapshot
}

func TestStore_SignedShellBindingRoundTripsAtomically(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	snapshot := signedShellSnapshot(t, time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC), 1)
	if err := store.CommitSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, recovered, err := store.LoadLatestValidSnapshot(context.Background())
	if err != nil || recovered {
		t.Fatalf("load err=%v recovered=%t", err, recovered)
	}
	var shell contracts.Capability
	for _, capability := range loaded.Capabilities {
		if capability.ID == "shell-main" {
			shell = capability
		}
	}
	if shell.SSHBinding == nil || shell.SSHBinding.BindingVersion != 1 {
		t.Fatalf("binding did not round trip: %#v", shell.SSHBinding)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("round-tripped binding is invalid: %v", err)
	}
}

func TestStore_CorruptNewestSignedBindingDoesNotRollBackToOlderKeyVersion(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	if err := store.CommitSnapshot(context.Background(), signedShellSnapshot(t, base, 1)); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitSnapshot(context.Background(), signedShellSnapshot(t, base.Add(time.Minute), 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		UPDATE capability_ssh_bindings SET binding_json = replace(binding_json, '"binding_version":2', '"binding_version":3')
		WHERE snapshot_id = (SELECT snapshot_id FROM active_snapshot WHERE singleton = 1)
		AND capability_id = 'shell-main'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadLatestValidSnapshot(context.Background()); err == nil {
		t.Fatal("corrupt newest binding rolled back to an older accepted key version")
	}
}

func TestStore_RejectsSignedBindingVersionRollbackWithoutChangingActiveSnapshot(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	if err := store.CommitSnapshot(context.Background(), signedShellSnapshot(t, base, 2)); err != nil {
		t.Fatal(err)
	}
	path := store.path
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CommitSnapshot(context.Background(), signedShellSnapshot(t, base.Add(time.Minute), 1)); err == nil {
		t.Fatal("SSH binding version rollback was committed")
	}
	loaded, recovered, err := store.LoadLatestValidSnapshot(context.Background())
	if err != nil || recovered || loaded.DirectoryVersion != 2 {
		t.Fatalf("load err=%v recovered=%t version=%d", err, recovered, loaded.DirectoryVersion)
	}
}

func TestStore_RejectsSameBindingVersionWithDifferentHostKeys(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	if err := store.CommitSnapshot(context.Background(), signedShellSnapshotWithHostByte(t, base, 1, 0x41)); err != nil {
		t.Fatal(err)
	}
	changed := signedShellSnapshotWithHostByte(t, base.Add(time.Minute), 1, 0x42)
	if err := store.CommitSnapshot(context.Background(), changed); err == nil {
		t.Fatal("same SSH binding version authorized different host keys")
	}
}

func TestStore_BindingHighWaterSurvivesSnapshotPruningAndRestart(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	if err := store.CommitSnapshot(context.Background(), signedShellSnapshot(t, base, 7)); err != nil {
		t.Fatal(err)
	}
	for version := uint64(8); version <= 14; version++ {
		snapshot := SyntheticSnapshot("device-owner-synthetic", base.Add(time.Duration(version)*time.Minute))
		snapshot.DirectoryVersion = version
		snapshot.GrantVersion = version
		if err := store.CommitSnapshot(context.Background(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	var retained int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM snapshots WHERE state = 'committed'`).Scan(&retained); err != nil || retained != 5 {
		t.Fatalf("retained=%d err=%v", retained, err)
	}
	path := store.path
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.CommitSnapshot(context.Background(), signedShellSnapshot(t, base.Add(20*time.Minute), 6)); err == nil {
		t.Fatal("pruning and restart erased the SSH binding high-water mark")
	}
}

func TestStore_FailedRefresh_KeepsPreviousActiveSnapshot(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	first := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))
	if err := store.CommitSnapshot(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER fail_synthetic_grant BEFORE INSERT ON grants
		WHEN NEW.grant_id = 'grant-trigger-failure'
		BEGIN SELECT RAISE(ABORT, 'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	failed := SyntheticSnapshot("device-owner-synthetic", first.CapturedAt.Add(time.Minute))
	failed.DirectoryVersion = 2
	failed.GrantVersion = 2
	failed.Grants[0].ID = "grant-trigger-failure"
	if err := store.CommitSnapshot(context.Background(), failed); err == nil {
		t.Fatal("synthetic transaction failure unexpectedly committed")
	}
	loaded, recovered, err := store.LoadLatestValidSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if recovered || loaded.DirectoryVersion != first.DirectoryVersion || loaded.GrantVersion != first.GrantVersion {
		t.Fatalf("last valid snapshot changed: %#v, recovered = %t", loaded, recovered)
	}
}

func TestStore_CorruptActiveRows_FallsBackToPreviousCommittedSnapshot(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	first := SyntheticSnapshot("device-owner-synthetic", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))
	if err := store.CommitSnapshot(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := SyntheticSnapshot("device-owner-synthetic", first.CapturedAt.Add(time.Minute))
	second.DirectoryVersion = 2
	second.GrantVersion = 2
	if err := store.CommitSnapshot(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		UPDATE capabilities SET features_json = '{'
		WHERE snapshot_id = (SELECT snapshot_id FROM active_snapshot WHERE singleton = 1)
		AND capability_id = 'desktop-main'`); err != nil {
		t.Fatal(err)
	}
	loaded, recovered, err := store.LoadLatestValidSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !recovered || loaded.DirectoryVersion != 1 || loaded.GrantVersion != 1 {
		t.Fatalf("fallback snapshot = %#v, recovered = %t", loaded, recovered)
	}
	var activeDirectoryVersion uint64
	if err := store.db.QueryRow(`
		SELECT s.directory_version FROM active_snapshot a
		JOIN snapshots s ON s.snapshot_id = a.snapshot_id WHERE a.singleton = 1`).Scan(&activeDirectoryVersion); err != nil {
		t.Fatal(err)
	}
	if activeDirectoryVersion != first.DirectoryVersion {
		t.Fatalf("recovered active directory version = %d, want %d", activeDirectoryVersion, first.DirectoryVersion)
	}
}

func TestStore_RetainsFiveCommittedSnapshots(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	for version := 1; version <= 7; version++ {
		snapshot := SyntheticSnapshot("device-owner-synthetic", base.Add(time.Duration(version)*time.Minute))
		snapshot.DirectoryVersion = uint64(version)
		snapshot.GrantVersion = uint64(version)
		if err := store.CommitSnapshot(context.Background(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM snapshots WHERE state = 'committed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("retained snapshots = %d", count)
	}
}

func TestStore_InvalidSnapshot_DoesNotCreateDatabaseState(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	snapshot := SyntheticSnapshot("device-owner-synthetic", time.Now())
	snapshot.Capabilities = append(snapshot.Capabilities, snapshot.Capabilities[0])
	if err := store.CommitSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("invalid snapshot committed")
	}
	if _, _, err := store.LoadLatestValidSnapshot(context.Background()); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("load error = %v", err)
	}
}

func TestStore_Open_ConfiguresDurablePragmas(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	var journalMode string
	var synchronous int
	var foreignKeys int
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" || synchronous != 2 || foreignKeys != 1 {
		t.Fatalf("pragmas = journal:%q synchronous:%d foreign_keys:%d", journalMode, synchronous, foreignKeys)
	}
	if err := store.IntegrityCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPath_ReturnsVersionedDatabaseFilename(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "pfremote-v1.db" {
		t.Fatalf("default path = %q", path)
	}
}

func TestSnapshotValidate_BrokenReferences_AreRejected(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{name: "unknown capability device", mutate: func(snapshot *Snapshot) { snapshot.Capabilities[0].DeviceID = "device-missing" }},
		{name: "unknown grant subject", mutate: func(snapshot *Snapshot) { snapshot.Grants[0].SubjectDeviceID = "device-missing" }},
		{name: "unknown grant capability", mutate: func(snapshot *Snapshot) { snapshot.Grants[0].CapabilityID = "capability-missing" }},
		{name: "unsupported grant state", mutate: func(snapshot *Snapshot) { snapshot.Grants[0].State = "unknown" }},
	}
	for index, test := range tests {
		t.Run(fmt.Sprintf("%02d_%s", index, test.name), func(t *testing.T) {
			snapshot := SyntheticSnapshot("device-owner-synthetic", time.Now())
			test.mutate(&snapshot)
			if err := snapshot.Validate(); err == nil {
				t.Fatal("invalid snapshot passed validation")
			}
		})
	}
}

func TestStore_ControlStateRoundTripsAndReplacesAtomically(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.SaveControlState(ctx, "test.control/v1", []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	payload, err := store.LoadControlState(ctx, "test.control/v1")
	if err != nil || string(payload) != `{"version":1}` {
		t.Fatalf("first payload = %q, %v", payload, err)
	}
	if err := store.SaveControlState(ctx, "test.control/v1", []byte(`{"version":2}`)); err != nil {
		t.Fatal(err)
	}
	payload, err = store.LoadControlState(ctx, "test.control/v1")
	if err != nil || string(payload) != `{"version":2}` {
		t.Fatalf("replacement payload = %q, %v", payload, err)
	}
	if _, err := store.LoadControlState(ctx, "test.control/v2"); err == nil {
		t.Fatal("unsupported control-state schema was treated as empty state")
	}
}

func TestStore_DeviceRevocationAtomicallyOverlaysSnapshotsAndCanBeCleared(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	capturedAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	if err := store.CommitSnapshot(ctx, SyntheticSnapshot("device-owner-synthetic", capturedAt)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveControlStateWithDeviceStatus(
		ctx, "test.control/v1", []byte(`{"status":"revoked"}`),
		"device-compute", "revoked", 5, 7,
	); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := store.LoadLatestValidSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state := snapshotDeviceState(loaded, "device-compute"); state != "revoked" {
		t.Fatalf("revoked Device state = %q", state)
	}
	if !loaded.CapturedAt.Equal(capturedAt) || loaded.DirectoryVersion != 5 || loaded.GrantVersion != 7 {
		t.Fatalf("revocation changed snapshot boundary or lost versions: %#v", loaded)
	}
	if err := store.SaveControlStateWithDeviceStatus(
		ctx, "test.control/v1", []byte(`{"status":"active"}`),
		"device-compute", "active", 6, 7,
	); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = store.LoadLatestValidSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state := snapshotDeviceState(loaded, "device-compute"); state != "online" {
		t.Fatalf("reactivated Device state = %q", state)
	}
	if loaded.DirectoryVersion != 6 || loaded.GrantVersion != 7 {
		t.Fatalf("reactivated versions = directory:%d grant:%d", loaded.DirectoryVersion, loaded.GrantVersion)
	}
}

func TestStore_InvalidDeviceStatusDoesNotReplaceControlState(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.SaveControlState(ctx, "test.control/v1", []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveControlStateWithDeviceStatus(
		ctx, "test.control/v1", []byte(`{"version":2}`),
		"device-compute", "unknown", 2, 2,
	); err == nil {
		t.Fatal("invalid Device status committed")
	}
	payload, err := store.LoadControlState(ctx, "test.control/v1")
	if err != nil || string(payload) != `{"version":1}` {
		t.Fatalf("control state after rejected update = %q, %v", payload, err)
	}
}

func TestStore_AuthorizationClockPersistsHighWaterMarkAcrossRollbackAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first := time.Date(2026, 8, 18, 12, 0, 0, 123, time.UTC)
	second := first.Add(48 * time.Hour)
	if observed, err := store.ObserveAuthorizationTime(ctx, first); err != nil || !observed.Equal(first) {
		t.Fatalf("first observation = %s, %v", observed, err)
	}
	if observed, err := store.ObserveAuthorizationTime(ctx, second); err != nil || !observed.Equal(second) {
		t.Fatalf("second observation = %s, %v", observed, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if observed, err := reopened.ObserveAuthorizationTime(ctx, first); err != nil || !observed.Equal(second) {
		t.Fatalf("rolled-back observation = %s, %v; want %s", observed, err, second)
	}
}

func TestStore_AuthorizationClockRejectsZeroTime(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	if _, err := store.ObserveAuthorizationTime(context.Background(), time.Time{}); err == nil {
		t.Fatal("zero authorization time was accepted")
	}
}

func TestStore_CommittingSnapshotSeedsAuthorizationClock(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	capturedAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	if err := store.CommitSnapshot(context.Background(), SyntheticSnapshot("device-owner-synthetic", capturedAt)); err != nil {
		t.Fatal(err)
	}
	rolledBack := capturedAt.Add(-24 * time.Hour)
	observed, err := store.ObserveAuthorizationTime(context.Background(), rolledBack)
	if err != nil || !observed.Equal(capturedAt) {
		t.Fatalf("observation after pre-use rollback = %s, %v; want %s", observed, err, capturedAt)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func snapshotDeviceState(snapshot Snapshot, deviceID string) string {
	for _, device := range snapshot.Devices {
		if device.ID == deviceID {
			return device.State
		}
	}
	return ""
}
