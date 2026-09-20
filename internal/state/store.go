package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
	_ "modernc.org/sqlite"
)

var ErrNoSnapshot = errors.New("no committed state snapshot")

type Store struct {
	path string
	db   *sql.DB
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", safeError(err))
	}
	return filepath.Join(configDir, "PF Remote", "state", "pfremote-v1.db"), nil
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("state database path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve state database path: %w", safeError(err))
	}
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", safeError(err))
	}
	if err := protectStateDirectory(dir); err != nil {
		return nil, err
	}

	urlPath := filepath.ToSlash(absPath)
	if filepath.VolumeName(absPath) != "" {
		// A leading slash keeps the drive letter in the URL path rather than
		// letting SQLite interpret it as an authority (file:///C:/...).
		urlPath = "/" + urlPath
	}
	fileURL := (&url.URL{Scheme: "file", Path: urlPath}).String()
	dsn := fileURL + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.New("open state database")
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	store := &Store{path: absPath, db: database}
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize state database: %w", safeError(err))
	}
	if err := protectStateFile(absPath); err != nil {
		database.Close()
		return nil, err
	}
	if err := store.migrate(context.Background()); err != nil {
		database.Close()
		return nil, err
	}
	if err := store.IntegrityCheck(context.Background()); err != nil {
		database.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ReplaceFromDatabase atomically swaps every Gateway-owned control table from
// a previously validated staging database. Any failure rolls the live database
// back as one transaction.
func (s *Store) ReplaceFromDatabase(ctx context.Context, stagingPath string) error {
	absPath, err := filepath.Abs(stagingPath)
	if err != nil || absPath == s.path {
		return errors.New("recovery staging database is invalid")
	}
	if _, err := s.db.ExecContext(ctx, `ATTACH DATABASE ? AS recovered`, absPath); err != nil {
		return fmt.Errorf("attach recovery staging database: %w", safeError(err))
	}
	defer s.db.ExecContext(context.Background(), `DETACH DATABASE recovered`)
	var snapshots, controls int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM recovered.snapshots WHERE state = 'committed'`).Scan(&snapshots); err != nil || snapshots != 1 {
		return errors.New("recovery staging database has an invalid snapshot set")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM recovered.control_state`).Scan(&controls); err != nil || controls != 1 {
		return errors.New("recovery staging database has invalid control state")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin recovery replacement: %w", safeError(err))
	}
	defer tx.Rollback()
	statements := []string{
		`DELETE FROM active_snapshot`, `DELETE FROM grants`, `DELETE FROM capability_desktop_profiles`,
		`DELETE FROM capability_ssh_bindings`, `DELETE FROM device_identity_keys`, `DELETE FROM capabilities`,
		`DELETE FROM devices`, `DELETE FROM snapshots`, `DELETE FROM ssh_binding_versions`,
		`DELETE FROM device_revocations`, `DELETE FROM control_versions`, `DELETE FROM authorization_clock`, `DELETE FROM control_state`,
		`DELETE FROM recent_activity`,
		`INSERT INTO snapshots SELECT * FROM recovered.snapshots`,
		`INSERT INTO devices SELECT * FROM recovered.devices`,
		`INSERT INTO capabilities SELECT * FROM recovered.capabilities`,
		`INSERT INTO device_identity_keys SELECT * FROM recovered.device_identity_keys`,
		`INSERT INTO capability_ssh_bindings SELECT * FROM recovered.capability_ssh_bindings`,
		`INSERT INTO capability_desktop_profiles SELECT * FROM recovered.capability_desktop_profiles`,
		`INSERT INTO grants SELECT * FROM recovered.grants`,
		`INSERT INTO ssh_binding_versions SELECT * FROM recovered.ssh_binding_versions`,
		`INSERT INTO active_snapshot SELECT * FROM recovered.active_snapshot`,
		`INSERT INTO control_state SELECT * FROM recovered.control_state`,
		`INSERT INTO authorization_clock SELECT * FROM recovered.authorization_clock`,
		`INSERT INTO control_versions SELECT * FROM recovered.control_versions`,
		`INSERT INTO device_revocations SELECT * FROM recovered.device_revocations`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("replace Gateway state: %w", safeError(err))
		}
	}
	var quickCheck string
	if err := tx.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&quickCheck); err != nil || quickCheck != "ok" {
		return errors.New("recovered Gateway state failed its integrity check")
	}
	var foreignKeyViolations int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations); err != nil || foreignKeyViolations != 0 {
		return errors.New("recovered Gateway state failed its relationship check")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovered Gateway state: %w", safeError(err))
	}
	return nil
}

func (s *Store) LoadControlState(ctx context.Context, schema string) ([]byte, error) {
	var storedSchema string
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT schema_version, payload FROM control_state WHERE singleton = 1`).Scan(&storedSchema, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load control state: %w", safeError(err))
	}
	if storedSchema != schema {
		return nil, errors.New("control state uses an unsupported schema")
	}
	return append([]byte(nil), payload...), nil
}

func (s *Store) SaveControlState(ctx context.Context, schema string, payload []byte) error {
	if err := validateControlState(schema, payload); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin control-state transaction: %w", safeError(err))
	}
	defer tx.Rollback()
	if err := writeControlState(ctx, tx, schema, payload); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit control state: %w", safeError(err))
	}
	return nil
}

// SaveControlStateWithDeviceStatus atomically persists enrollment state and the
// revocation overlay consumed by catalog snapshots. An active transition clears
// an older revocation; a revoked transition installs or advances it.
func (s *Store) SaveControlStateWithDeviceStatus(ctx context.Context, schema string, payload []byte, deviceID, status string, directoryVersion, grantVersion uint64) error {
	if err := validateControlState(schema, payload); err != nil {
		return err
	}
	if !validIdentifier(deviceID) || status != "active" && status != "revoked" || directoryVersion == 0 || grantVersion == 0 {
		return errors.New("Device status update is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin Device-status transaction: %w", safeError(err))
	}
	defer tx.Rollback()
	if err := writeControlState(ctx, tx, schema, payload); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_versions(singleton, directory_version, grant_version, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			directory_version = MAX(directory_version, excluded.directory_version),
			grant_version = MAX(grant_version, excluded.grant_version),
			updated_at = excluded.updated_at`, directoryVersion, grantVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("write control versions: %w", safeError(err))
	}
	if status == "revoked" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO device_revocations(device_id, directory_version, grant_version, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(device_id) DO UPDATE SET
				directory_version = MAX(directory_version, excluded.directory_version),
				grant_version = MAX(grant_version, excluded.grant_version),
				updated_at = excluded.updated_at`, deviceID, directoryVersion, grantVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("write Device revocation: %w", safeError(err))
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM device_revocations WHERE device_id = ?`, deviceID); err != nil {
		return fmt.Errorf("clear Device revocation: %w", safeError(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Device status: %w", safeError(err))
	}
	return nil
}

func validateControlState(schema string, payload []byte) error {
	if schema == "" || len(payload) == 0 {
		return errors.New("control state schema and payload are required")
	}
	return nil
}

func writeControlState(ctx context.Context, tx *sql.Tx, schema string, payload []byte) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_state(singleton, schema_version, payload, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			schema_version = excluded.schema_version,
			payload = excluded.payload,
			updated_at = excluded.updated_at`, schema, payload, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("write control state: %w", safeError(err))
	}
	return nil
}

// ObserveAuthorizationTime returns a persistent high-water mark so a local
// wall-clock rollback cannot extend cached authorization after time was seen.
func (s *Store) ObserveAuthorizationTime(ctx context.Context, observed time.Time) (time.Time, error) {
	if observed.IsZero() {
		return time.Time{}, errors.New("observed authorization time is required")
	}
	observed = observed.UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return time.Time{}, fmt.Errorf("begin authorization-clock transaction: %w", safeError(err))
	}
	defer tx.Rollback()
	var storedNanos int64
	err = tx.QueryRowContext(ctx, `SELECT last_observed_unix_nano FROM authorization_clock WHERE singleton = 1`).Scan(&storedNanos)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, fmt.Errorf("read authorization clock: %w", safeError(err))
	}
	effective := observed
	if err == nil {
		stored := time.Unix(0, storedNanos).UTC()
		if stored.After(effective) {
			effective = stored
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO authorization_clock(singleton, last_observed_unix_nano) VALUES (1, ?)
		ON CONFLICT(singleton) DO UPDATE SET last_observed_unix_nano = MAX(last_observed_unix_nano, excluded.last_observed_unix_nano)`,
		effective.UnixNano()); err != nil {
		return time.Time{}, fmt.Errorf("write authorization clock: %w", safeError(err))
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, fmt.Errorf("commit authorization clock: %w", safeError(err))
	}
	return effective, nil
}

func (s *Store) CommitSnapshot(ctx context.Context, snapshot Snapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin snapshot transaction: %w", safeError(err))
	}
	defer tx.Rollback()
	snapshotKey, err := randomSnapshotKey()
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO snapshots(snapshot_key, schema_version, fabric_id, directory_version, grant_version, captured_at, state)
		VALUES (?, ?, ?, ?, ?, ?, 'staging')`,
		snapshotKey, snapshot.SchemaVersion, snapshot.FabricID, snapshot.DirectoryVersion,
		snapshot.GrantVersion, snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert state snapshot: %w", safeError(err))
	}
	snapshotID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read state snapshot ID: %w", safeError(err))
	}
	for _, device := range snapshot.Devices {
		previousAliases, _ := json.Marshal(device.PreviousAliases)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO devices(snapshot_id, device_id, alias, previous_aliases_json, display_name, state)
			VALUES (?, ?, ?, ?, ?, ?)`, snapshotID, device.ID, device.Alias, string(previousAliases), device.DisplayName, device.State); err != nil {
			return fmt.Errorf("insert snapshot Device: %w", safeError(err))
		}
		if device.IdentityPublicKey != "" {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO device_identity_keys(snapshot_id, device_id, identity_public_key)
				VALUES (?, ?, ?)`, snapshotID, device.ID, device.IdentityPublicKey); err != nil {
				return fmt.Errorf("insert snapshot Device identity: %w", safeError(err))
			}
		}
	}
	for _, capability := range snapshot.Capabilities {
		previousAliases, _ := json.Marshal(capability.PreviousAliases)
		features, _ := json.Marshal(capability.Features)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO capabilities(snapshot_id, capability_id, device_id, alias, previous_aliases_json, display_name, kind, state, features_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshotID, capability.ID, capability.DeviceID, capability.Alias,
			string(previousAliases), capability.DisplayName, capability.Kind, capability.State, string(features)); err != nil {
			return fmt.Errorf("insert snapshot Capability: %w", safeError(err))
		}
		if capability.SSHBinding != nil {
			binding, marshalErr := json.Marshal(capability.SSHBinding)
			if marshalErr != nil {
				return errors.New("encode snapshot SSH Capability binding")
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO capability_ssh_bindings(snapshot_id, capability_id, binding_json)
				VALUES (?, ?, ?)`, snapshotID, capability.ID, string(binding)); err != nil {
				return fmt.Errorf("insert snapshot SSH Capability binding: %w", safeError(err))
			}
			var previousVersion uint64
			var previousSignature string
			versionErr := tx.QueryRowContext(ctx, `
				SELECT binding_version, binding_signature FROM ssh_binding_versions
				WHERE fabric_id = ? AND device_id = ? AND capability_id = ?`,
				snapshot.FabricID, capability.DeviceID, capability.ID).Scan(&previousVersion, &previousSignature)
			if versionErr != nil && !errors.Is(versionErr, sql.ErrNoRows) {
				return fmt.Errorf("read SSH binding version: %w", safeError(versionErr))
			}
			if versionErr == nil && (capability.SSHBinding.BindingVersion < previousVersion ||
				capability.SSHBinding.BindingVersion == previousVersion && capability.SSHBinding.Signature != previousSignature) {
				return errors.New("SSH Capability binding version rollback or reuse was rejected")
			}
			if versionErr != nil || capability.SSHBinding.BindingVersion > previousVersion {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO ssh_binding_versions(fabric_id, device_id, capability_id, binding_version, binding_signature, updated_at)
					VALUES (?, ?, ?, ?, ?, ?)
					ON CONFLICT(fabric_id, device_id, capability_id) DO UPDATE SET
						binding_version = excluded.binding_version,
						binding_signature = excluded.binding_signature,
						updated_at = excluded.updated_at`, snapshot.FabricID, capability.DeviceID, capability.ID,
					capability.SSHBinding.BindingVersion, capability.SSHBinding.Signature, snapshot.CapturedAt.UTC().Format(time.RFC3339Nano)); err != nil {
					return fmt.Errorf("advance SSH binding version: %w", safeError(err))
				}
			}
		}
		if capability.DesktopProfile != nil {
			profile, marshalErr := json.Marshal(capability.DesktopProfile)
			if marshalErr != nil {
				return errors.New("encode snapshot Desktop Capability profile")
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO capability_desktop_profiles(snapshot_id, capability_id, profile_json)
				VALUES (?, ?, ?)`, snapshotID, capability.ID, string(profile)); err != nil {
				return fmt.Errorf("insert snapshot Desktop Capability profile: %w", safeError(err))
			}
		}
	}
	for _, grant := range snapshot.Grants {
		var validUntil any
		if grant.ValidUntil != nil {
			validUntil = grant.ValidUntil.UTC().Format(time.RFC3339Nano)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO grants(snapshot_id, grant_id, subject_device_id, capability_id, state, valid_until)
			VALUES (?, ?, ?, ?, ?, ?)`, snapshotID, grant.ID, grant.SubjectDeviceID, grant.CapabilityID, grant.State, validUntil); err != nil {
			return fmt.Errorf("insert snapshot Grant: %w", safeError(err))
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE snapshots SET state = 'committed' WHERE snapshot_id = ?`, snapshotID); err != nil {
		return fmt.Errorf("commit snapshot marker: %w", safeError(err))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO active_snapshot(singleton, snapshot_id) VALUES (1, ?)
		ON CONFLICT(singleton) DO UPDATE SET snapshot_id = excluded.snapshot_id`, snapshotID); err != nil {
		return fmt.Errorf("activate state snapshot: %w", safeError(err))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO authorization_clock(singleton, last_observed_unix_nano) VALUES (1, ?)
		ON CONFLICT(singleton) DO UPDATE SET last_observed_unix_nano = MAX(last_observed_unix_nano, excluded.last_observed_unix_nano)`,
		snapshot.CapturedAt.UnixNano()); err != nil {
		return fmt.Errorf("advance snapshot authorization clock: %w", safeError(err))
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM snapshots
		WHERE snapshot_id NOT IN (SELECT snapshot_id FROM snapshots WHERE state = 'committed' ORDER BY snapshot_id DESC LIMIT 5)`); err != nil {
		return fmt.Errorf("prune old state snapshots: %w", safeError(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state snapshot: %w", safeError(err))
	}
	return nil
}

func (s *Store) LoadLatestValidSnapshot(ctx context.Context) (Snapshot, bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.snapshot_id, CASE WHEN a.snapshot_id = s.snapshot_id THEN 1 ELSE 0 END
		FROM snapshots s LEFT JOIN active_snapshot a ON a.singleton = 1
		WHERE s.state = 'committed'
		ORDER BY CASE WHEN a.snapshot_id = s.snapshot_id THEN 0 ELSE 1 END, s.snapshot_id DESC`)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("list committed snapshots: %w", safeError(err))
	}
	type candidate struct {
		id     int64
		active bool
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.active); err != nil {
			rows.Close()
			return Snapshot{}, false, fmt.Errorf("read committed snapshot list: %w", safeError(err))
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, false, fmt.Errorf("close committed snapshot list: %w", safeError(err))
	}
	if len(candidates) == 0 {
		return Snapshot{}, false, ErrNoSnapshot
	}
	for _, item := range candidates {
		snapshot, err := s.loadSnapshot(ctx, item.id)
		if err == nil {
			err = s.applyControlRevocations(ctx, &snapshot)
		}
		if err == nil {
			err = snapshot.Validate()
		}
		if err == nil {
			err = s.validateSSHBindingVersions(ctx, snapshot)
		}
		if err == nil {
			if !item.active {
				if err := s.activateSnapshot(ctx, item.id); err != nil {
					return Snapshot{}, false, err
				}
			}
			return snapshot, !item.active, nil
		}
	}
	return Snapshot{}, false, errors.New("no valid committed state snapshot remains")
}

func (s *Store) validateSSHBindingVersions(ctx context.Context, snapshot Snapshot) error {
	for _, capability := range snapshot.Capabilities {
		if capability.SSHBinding == nil {
			continue
		}
		var bindingVersion uint64
		var bindingSignature string
		err := s.db.QueryRowContext(ctx, `
			SELECT binding_version, binding_signature FROM ssh_binding_versions
			WHERE fabric_id = ? AND device_id = ? AND capability_id = ?`,
			snapshot.FabricID, capability.DeviceID, capability.ID).Scan(&bindingVersion, &bindingSignature)
		if err != nil || capability.SSHBinding.BindingVersion != bindingVersion || capability.SSHBinding.Signature != bindingSignature {
			return errors.New("state snapshot SSH Capability binding is not the newest accepted version")
		}
	}
	return nil
}

func (s *Store) applyControlRevocations(ctx context.Context, snapshot *Snapshot) error {
	var directoryVersion, grantVersion uint64
	err := s.db.QueryRowContext(ctx, `SELECT directory_version, grant_version FROM control_versions WHERE singleton = 1`).Scan(&directoryVersion, &grantVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load control versions: %w", safeError(err))
	}
	if err == nil {
		if directoryVersion > snapshot.DirectoryVersion {
			snapshot.DirectoryVersion = directoryVersion
		}
		if grantVersion > snapshot.GrantVersion {
			snapshot.GrantVersion = grantVersion
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT device_id FROM device_revocations ORDER BY device_id`)
	if err != nil {
		return fmt.Errorf("load Device revocations: %w", safeError(err))
	}
	defer rows.Close()
	revoked := make(map[string]struct{})
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return fmt.Errorf("read Device revocation: %w", safeError(err))
		}
		revoked[deviceID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate Device revocations: %w", safeError(err))
	}
	for index := range snapshot.Devices {
		if _, exists := revoked[snapshot.Devices[index].ID]; exists {
			snapshot.Devices[index].State = "revoked"
		}
	}
	for index := range snapshot.Grants {
		if _, exists := revoked[snapshot.Grants[index].SubjectDeviceID]; exists {
			snapshot.Grants[index].State = "revoked"
		}
	}
	return nil
}

func (s *Store) activateSnapshot(ctx context.Context, snapshotID int64) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO active_snapshot(singleton, snapshot_id) VALUES (1, ?)
		ON CONFLICT(singleton) DO UPDATE SET snapshot_id = excluded.snapshot_id`, snapshotID); err != nil {
		return fmt.Errorf("recover active state snapshot: %w", safeError(err))
	}
	return nil
}

func (s *Store) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&result); err != nil {
		return fmt.Errorf("run state database integrity check: %w", safeError(err))
	}
	if result != "ok" {
		return errors.New("state database integrity check failed")
	}
	var foreignKeyViolation int
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`)
	if err := row.Scan(&foreignKeyViolation); err != nil {
		return fmt.Errorf("run state database foreign-key check: %w", safeError(err))
	}
	if foreignKeyViolation != 0 {
		return errors.New("state database foreign-key check failed")
	}
	return nil
}

func (s *Store) loadSnapshot(ctx context.Context, snapshotID int64) (Snapshot, error) {
	var snapshot Snapshot
	var capturedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT schema_version, fabric_id, directory_version, grant_version, captured_at
		FROM snapshots WHERE snapshot_id = ? AND state = 'committed'`, snapshotID).Scan(
		&snapshot.SchemaVersion, &snapshot.FabricID, &snapshot.DirectoryVersion, &snapshot.GrantVersion, &capturedAt,
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load state snapshot metadata: %w", safeError(err))
	}
	snapshot.CapturedAt, err = time.Parse(time.RFC3339Nano, capturedAt)
	if err != nil {
		return Snapshot{}, errors.New("state snapshot has an invalid capture time")
	}

	deviceRows, err := s.db.QueryContext(ctx, `
		SELECT d.device_id, d.alias, d.previous_aliases_json, d.display_name, d.state, k.identity_public_key
		FROM devices d
		LEFT JOIN device_identity_keys k ON k.snapshot_id = d.snapshot_id AND k.device_id = d.device_id
		WHERE d.snapshot_id = ? ORDER BY d.device_id`, snapshotID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load snapshot Devices: %w", safeError(err))
	}
	for deviceRows.Next() {
		var device contracts.Device
		var previousAliases string
		var identityPublicKey sql.NullString
		if err := deviceRows.Scan(&device.ID, &device.Alias, &previousAliases, &device.DisplayName, &device.State, &identityPublicKey); err != nil {
			deviceRows.Close()
			return Snapshot{}, fmt.Errorf("read snapshot Device: %w", safeError(err))
		}
		if identityPublicKey.Valid {
			device.IdentityPublicKey = identityPublicKey.String
		}
		if err := json.Unmarshal([]byte(previousAliases), &device.PreviousAliases); err != nil {
			deviceRows.Close()
			return Snapshot{}, errors.New("snapshot Device aliases are invalid")
		}
		snapshot.Devices = append(snapshot.Devices, device)
	}
	if err := deviceRows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("close snapshot Device rows: %w", safeError(err))
	}

	capabilityRows, err := s.db.QueryContext(ctx, `
		SELECT c.capability_id, c.device_id, c.alias, c.previous_aliases_json, c.display_name, c.kind, c.state, c.features_json, b.binding_json, d.profile_json
		FROM capabilities c
		LEFT JOIN capability_ssh_bindings b ON b.snapshot_id = c.snapshot_id AND b.capability_id = c.capability_id
		LEFT JOIN capability_desktop_profiles d ON d.snapshot_id = c.snapshot_id AND d.capability_id = c.capability_id
		WHERE c.snapshot_id = ? ORDER BY c.capability_id`, snapshotID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load snapshot Capabilities: %w", safeError(err))
	}
	for capabilityRows.Next() {
		var capability contracts.Capability
		var previousAliases, features string
		var bindingJSON, desktopProfileJSON sql.NullString
		if err := capabilityRows.Scan(&capability.ID, &capability.DeviceID, &capability.Alias, &previousAliases,
			&capability.DisplayName, &capability.Kind, &capability.State, &features, &bindingJSON, &desktopProfileJSON); err != nil {
			capabilityRows.Close()
			return Snapshot{}, fmt.Errorf("read snapshot Capability: %w", safeError(err))
		}
		if bindingJSON.Valid {
			var binding contracts.SSHCapabilityBinding
			if err := json.Unmarshal([]byte(bindingJSON.String), &binding); err != nil {
				capabilityRows.Close()
				return Snapshot{}, errors.New("snapshot SSH Capability binding is invalid")
			}
			capability.SSHBinding = &binding
		}
		if desktopProfileJSON.Valid {
			var profile contracts.DesktopProfile
			if err := json.Unmarshal([]byte(desktopProfileJSON.String), &profile); err != nil {
				capabilityRows.Close()
				return Snapshot{}, errors.New("snapshot Desktop Capability profile is invalid")
			}
			capability.DesktopProfile = &profile
		}
		if json.Unmarshal([]byte(previousAliases), &capability.PreviousAliases) != nil || json.Unmarshal([]byte(features), &capability.Features) != nil {
			capabilityRows.Close()
			return Snapshot{}, errors.New("snapshot Capability arrays are invalid")
		}
		snapshot.Capabilities = append(snapshot.Capabilities, capability)
	}
	if err := capabilityRows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("close snapshot Capability rows: %w", safeError(err))
	}

	grantRows, err := s.db.QueryContext(ctx, `
		SELECT grant_id, subject_device_id, capability_id, state, valid_until
		FROM grants WHERE snapshot_id = ? ORDER BY grant_id`, snapshotID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load snapshot Grants: %w", safeError(err))
	}
	for grantRows.Next() {
		var grant contracts.Grant
		var validUntil sql.NullString
		if err := grantRows.Scan(&grant.ID, &grant.SubjectDeviceID, &grant.CapabilityID, &grant.State, &validUntil); err != nil {
			grantRows.Close()
			return Snapshot{}, fmt.Errorf("read snapshot Grant: %w", safeError(err))
		}
		if validUntil.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, validUntil.String)
			if err != nil {
				grantRows.Close()
				return Snapshot{}, errors.New("snapshot Grant expiry is invalid")
			}
			grant.ValidUntil = &parsed
		}
		snapshot.Grants = append(snapshot.Grants, grant)
	}
	if err := grantRows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("close snapshot Grant rows: %w", safeError(err))
	}
	return snapshot, nil
}

// LoadRecentSessions returns only the bounded, redacted activity metadata used
// by the local Center. Command content, output, routes, endpoints, and secrets
// are never accepted by this table.
func (s *Store) LoadRecentSessions(ctx context.Context, limit int) ([]contracts.RecentSession, error) {
	limit = boundedRecentSessionLimit(limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, canonical_target, action, status, started_at
		FROM recent_activity
		ORDER BY sequence DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load recent activity: %w", safeError(err))
	}
	defer rows.Close()
	entries := make([]contracts.RecentSession, 0, limit)
	for rows.Next() {
		var entry contracts.RecentSession
		var startedAt string
		if err := rows.Scan(&entry.SessionID, &entry.CanonicalTarget, &entry.Action, &entry.Status, &startedAt); err != nil {
			return nil, fmt.Errorf("read recent activity: %w", safeError(err))
		}
		entry.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil || !validRecentSession(entry) {
			return nil, errors.New("recent activity is invalid")
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load recent activity: %w", safeError(err))
	}
	return entries, nil
}

func (s *Store) RecordRecentSession(ctx context.Context, entry contracts.RecentSession, limit int) error {
	if !validRecentSession(entry) {
		return errors.New("recent activity is invalid")
	}
	limit = boundedRecentSessionLimit(limit)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin recent-activity transaction: %w", safeError(err))
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO recent_activity(session_id, canonical_target, action, status, started_at)
		VALUES (?, ?, ?, ?, ?)`,
		entry.SessionID, entry.CanonicalTarget, entry.Action, entry.Status, entry.StartedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record recent activity: %w", safeError(err))
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM recent_activity
		WHERE sequence NOT IN (
			SELECT sequence FROM recent_activity ORDER BY sequence DESC LIMIT ?
		)`, limit); err != nil {
		return fmt.Errorf("prune recent activity: %w", safeError(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recent activity: %w", safeError(err))
	}
	return nil
}

func boundedRecentSessionLimit(limit int) int {
	if limit < 1 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func validRecentSession(entry contracts.RecentSession) bool {
	return entry.SessionID != "" && entry.CanonicalTarget != "" && entry.Action != "" && entry.Status != "" && !entry.StartedAt.IsZero()
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin state schema migration: %w", safeError(err))
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL) STRICT`,
		`CREATE TABLE IF NOT EXISTS snapshots(
			snapshot_id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_key TEXT NOT NULL UNIQUE,
			schema_version TEXT NOT NULL,
			fabric_id TEXT NOT NULL,
			directory_version INTEGER NOT NULL CHECK(directory_version > 0),
			grant_version INTEGER NOT NULL CHECK(grant_version > 0),
			captured_at TEXT NOT NULL,
			state TEXT NOT NULL CHECK(state IN ('staging', 'committed'))
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS devices(
			snapshot_id INTEGER NOT NULL REFERENCES snapshots(snapshot_id) ON DELETE CASCADE,
			device_id TEXT NOT NULL,
			alias TEXT NOT NULL,
			previous_aliases_json TEXT NOT NULL,
			display_name TEXT NOT NULL,
			state TEXT NOT NULL,
			PRIMARY KEY(snapshot_id, device_id)
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS capabilities(
			snapshot_id INTEGER NOT NULL,
			capability_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			alias TEXT NOT NULL,
			previous_aliases_json TEXT NOT NULL,
			display_name TEXT NOT NULL,
			kind TEXT NOT NULL CHECK(kind IN ('shell', 'desktop')),
			state TEXT NOT NULL,
			features_json TEXT NOT NULL,
			PRIMARY KEY(snapshot_id, capability_id),
			FOREIGN KEY(snapshot_id, device_id) REFERENCES devices(snapshot_id, device_id) ON DELETE CASCADE
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS device_identity_keys(
			snapshot_id INTEGER NOT NULL,
			device_id TEXT NOT NULL,
			identity_public_key TEXT NOT NULL,
			PRIMARY KEY(snapshot_id, device_id),
			FOREIGN KEY(snapshot_id, device_id) REFERENCES devices(snapshot_id, device_id) ON DELETE CASCADE
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS capability_ssh_bindings(
			snapshot_id INTEGER NOT NULL,
			capability_id TEXT NOT NULL,
			binding_json TEXT NOT NULL,
			PRIMARY KEY(snapshot_id, capability_id),
			FOREIGN KEY(snapshot_id, capability_id) REFERENCES capabilities(snapshot_id, capability_id) ON DELETE CASCADE
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS capability_desktop_profiles(
			snapshot_id INTEGER NOT NULL,
			capability_id TEXT NOT NULL,
			profile_json TEXT NOT NULL,
			PRIMARY KEY(snapshot_id, capability_id),
			FOREIGN KEY(snapshot_id, capability_id) REFERENCES capabilities(snapshot_id, capability_id) ON DELETE CASCADE
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS ssh_binding_versions(
			fabric_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			capability_id TEXT NOT NULL,
			binding_version INTEGER NOT NULL CHECK(binding_version > 0),
			binding_signature TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(fabric_id, device_id, capability_id)
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS grants(
			snapshot_id INTEGER NOT NULL,
			grant_id TEXT NOT NULL,
			subject_device_id TEXT NOT NULL,
			capability_id TEXT NOT NULL,
			state TEXT NOT NULL CHECK(state IN ('active', 'revoked')),
			valid_until TEXT,
			PRIMARY KEY(snapshot_id, grant_id),
			FOREIGN KEY(snapshot_id, subject_device_id) REFERENCES devices(snapshot_id, device_id) ON DELETE CASCADE,
			FOREIGN KEY(snapshot_id, capability_id) REFERENCES capabilities(snapshot_id, capability_id) ON DELETE CASCADE
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS active_snapshot(
			singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
			snapshot_id INTEGER NOT NULL REFERENCES snapshots(snapshot_id)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS control_state(
			singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
			schema_version TEXT NOT NULL,
			payload BLOB NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS authorization_clock(
			singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
			last_observed_unix_nano INTEGER NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS control_versions(
			singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
			directory_version INTEGER NOT NULL CHECK(directory_version > 0),
			grant_version INTEGER NOT NULL CHECK(grant_version > 0),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS device_revocations(
			device_id TEXT PRIMARY KEY,
			directory_version INTEGER NOT NULL CHECK(directory_version > 0),
			grant_version INTEGER NOT NULL CHECK(grant_version > 0),
			updated_at TEXT NOT NULL
		) WITHOUT ROWID, STRICT`,
		`CREATE TABLE IF NOT EXISTS recent_activity(
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL UNIQUE,
			canonical_target TEXT NOT NULL,
			action TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL
		) STRICT`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, datetime('now'))`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, datetime('now'))`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, datetime('now'))`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, datetime('now'))`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (5, datetime('now'))`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply state schema migration: %w", safeError(err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state schema migration: %w", safeError(err))
	}
	return nil
}

func randomSnapshotKey() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", errors.New("generate state snapshot key")
	}
	return "snapshot-" + hex.EncodeToString(buffer), nil
}

func safeError(err error) error {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return pathError.Err
	}
	return err
}
