package recovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/state"
)

var (
	ErrWrongFabric = errors.New("recovery file belongs to a different PF Remote Fabric")
	ErrRollback    = errors.New("recovery file is not newer than the current Gateway state")
)

type Summary struct {
	FabricID         string
	OwnerDeviceID    string
	DirectoryVersion uint64
	GrantVersion     uint64
	DeviceCount      int
	CapabilityCount  int
	GrantCount       int
}

func Export(ctx context.Context, store *state.Store, password string) ([]byte, Summary, error) {
	return export(ctx, store, password, DefaultIterations)
}

func export(ctx context.Context, store *state.Store, password string, iterations int) ([]byte, Summary, error) {
	if store == nil {
		return nil, Summary{}, errors.New("Gateway state is required")
	}
	control, err := store.LoadControlState(ctx, enrollment.PersistentStateSchema)
	if err != nil || len(control) == 0 {
		return nil, Summary{}, errors.New("Gateway enrollment state is not ready for recovery export")
	}
	prepared, enrollmentSummary, err := enrollment.PrepareRecoveryState(control)
	if err != nil {
		return nil, Summary{}, err
	}
	snapshot, _, err := store.LoadLatestValidSnapshot(ctx)
	if err != nil {
		return nil, Summary{}, errors.New("Gateway device directory is not ready for recovery export")
	}
	if err := validatePair(enrollmentSummary, snapshot); err != nil {
		return nil, Summary{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, Summary{}, errors.New("encode Gateway directory for recovery")
	}
	exportedAt := time.Now().UTC()
	payload := Payload{
		SchemaVersion: PayloadSchema, FabricID: snapshot.FabricID, OwnerDeviceID: enrollmentSummary.OwnerDeviceID,
		DirectoryVersion: snapshot.DirectoryVersion, GrantVersion: snapshot.GrantVersion, ExportedAt: exportedAt,
		EnrollmentState: prepared, Snapshot: snapshotJSON,
	}
	encoded, err := seal(payload, password, iterations, rand.Reader, exportedAt)
	if err != nil {
		return nil, Summary{}, err
	}
	return encoded, summarize(enrollmentSummary, snapshot), nil
}

// RestoreToNewDatabase validates and decrypts everything in a staging database,
// then publishes it only if the requested destination does not already exist.
func RestoreToNewDatabase(ctx context.Context, destination string, encoded []byte, password string) (Summary, error) {
	payload, err := Open(encoded, password)
	if err != nil {
		return Summary{}, err
	}
	prepared, enrollmentSummary, err := enrollment.PrepareRecoveryState(payload.EnrollmentState)
	if err != nil || !bytes.Equal(prepared, payload.EnrollmentState) {
		return Summary{}, ErrCannotOpen
	}
	var snapshot state.Snapshot
	decoder := json.NewDecoder(bytes.NewReader(payload.Snapshot))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil || decoder.Decode(&struct{}{}) != io.EOF || snapshot.Validate() != nil {
		return Summary{}, ErrCannotOpen
	}
	if payload.FabricID != snapshot.FabricID || payload.OwnerDeviceID != enrollmentSummary.OwnerDeviceID ||
		payload.DirectoryVersion != snapshot.DirectoryVersion || payload.GrantVersion != snapshot.GrantVersion ||
		validatePair(enrollmentSummary, snapshot) != nil {
		return Summary{}, ErrCannotOpen
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil || filepath.Ext(absDestination) == "" {
		return Summary{}, errors.New("recovery destination is invalid")
	}
	_, destinationErr := os.Stat(absDestination)
	if destinationErr != nil && !errors.Is(destinationErr, os.ErrNotExist) {
		return Summary{}, errors.New("recovery destination cannot be inspected")
	}
	if err := os.MkdirAll(filepath.Dir(absDestination), 0o700); err != nil {
		return Summary{}, errors.New("create recovery destination")
	}
	staging, err := os.CreateTemp(filepath.Dir(absDestination), ".pfremote-recovery-*.db")
	if err != nil {
		return Summary{}, errors.New("create recovery staging file")
	}
	stagingPath := staging.Name()
	if err := staging.Close(); err != nil {
		os.Remove(stagingPath)
		return Summary{}, errors.New("close recovery staging file")
	}
	if err := os.Remove(stagingPath); err != nil {
		return Summary{}, errors.New("prepare recovery staging file")
	}
	defer removeSQLiteFiles(stagingPath)
	stagingStore, err := state.Open(stagingPath)
	if err != nil {
		return Summary{}, errors.New("initialize recovery staging state")
	}
	failed := true
	defer func() {
		if failed {
			stagingStore.Close()
		}
	}()
	if err := stagingStore.SaveControlState(ctx, enrollment.PersistentStateSchema, payload.EnrollmentState); err != nil {
		return Summary{}, errors.New("stage recovered enrollment state")
	}
	deviceIDs := make([]string, 0, len(enrollmentSummary.DeviceStatuses))
	for deviceID := range enrollmentSummary.DeviceStatuses {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)
	for _, deviceID := range deviceIDs {
		if err := stagingStore.SaveControlStateWithDeviceStatus(ctx, enrollment.PersistentStateSchema, payload.EnrollmentState,
			deviceID, enrollmentSummary.DeviceStatuses[deviceID], snapshot.DirectoryVersion, snapshot.GrantVersion); err != nil {
			return Summary{}, errors.New("stage recovered Device status")
		}
	}
	if err := stagingStore.CommitSnapshot(ctx, snapshot); err != nil {
		return Summary{}, errors.New("stage recovered device directory")
	}
	if err := stagingStore.IntegrityCheck(ctx); err != nil {
		return Summary{}, errors.New("validate recovered Gateway state")
	}
	if err := stagingStore.Close(); err != nil {
		return Summary{}, errors.New("finish recovered Gateway state")
	}
	failed = false
	if errors.Is(destinationErr, os.ErrNotExist) {
		if err := os.Link(stagingPath, absDestination); err != nil {
			return Summary{}, fmt.Errorf("publish recovered Gateway state: %w", err)
		}
		if err := os.Remove(stagingPath); err != nil {
			return Summary{}, errors.New("finalize recovered Gateway state")
		}
		return summarize(enrollmentSummary, snapshot), nil
	}
	liveStore, err := state.Open(absDestination)
	if err != nil {
		return Summary{}, errors.New("open current Gateway state")
	}
	defer liveStore.Close()
	currentControl, err := liveStore.LoadControlState(ctx, enrollment.PersistentStateSchema)
	if err != nil {
		return Summary{}, errors.New("inspect current Gateway state")
	}
	if len(currentControl) != 0 {
		_, current, err := enrollment.PrepareRecoveryState(currentControl)
		if err != nil {
			return Summary{}, errors.New("current Gateway state is invalid")
		}
		if current.FabricID != payload.FabricID {
			return Summary{}, ErrWrongFabric
		}
		if payload.DirectoryVersion < current.DirectoryVersion || payload.GrantVersion < current.GrantVersion ||
			payload.DirectoryVersion == current.DirectoryVersion && payload.GrantVersion == current.GrantVersion {
			return Summary{}, ErrRollback
		}
	}
	if err := liveStore.ReplaceFromDatabase(ctx, stagingPath); err != nil {
		return Summary{}, errors.New("replace Gateway state from validated recovery")
	}
	return summarize(enrollmentSummary, snapshot), nil
}

func validatePair(enrollmentSummary enrollment.RecoverySummary, snapshot state.Snapshot) error {
	if enrollmentSummary.FabricID != snapshot.FabricID || enrollmentSummary.DirectoryVersion != snapshot.DirectoryVersion ||
		enrollmentSummary.GrantVersion != snapshot.GrantVersion {
		return errors.New("Gateway identity and device directory versions do not match")
	}
	ownerFound := false
	for _, device := range snapshot.Devices {
		if device.ID == enrollmentSummary.OwnerDeviceID {
			ownerFound = true
		}
		if enrollmentSummary.DeviceStatuses[device.ID] == "revoked" && device.State != "revoked" {
			return errors.New("Gateway revocation state does not match its device directory")
		}
	}
	if !ownerFound {
		return errors.New("Gateway Owner Device is missing from its device directory")
	}
	return nil
}

func summarize(enrollmentSummary enrollment.RecoverySummary, snapshot state.Snapshot) Summary {
	return Summary{FabricID: snapshot.FabricID, OwnerDeviceID: enrollmentSummary.OwnerDeviceID,
		DirectoryVersion: snapshot.DirectoryVersion, GrantVersion: snapshot.GrantVersion,
		DeviceCount: len(snapshot.Devices), CapabilityCount: len(snapshot.Capabilities), GrantCount: len(snapshot.Grants)}
}

func removeSQLiteFiles(path string) {
	os.Remove(path)
	os.Remove(path + "-wal")
	os.Remove(path + "-shm")
}
