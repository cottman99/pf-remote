package windowsinstaller

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const pendingName = "pending-upgrade.json"

// Activate must restart the exact version, then verify its local health. It must
// return only after completion and be safe to retry after a process interruption.
// This transaction rolls back program files, not user data: callers must first
// establish that both versions can use the same data, and drain active sessions.
type Activate func(Result) error

type pendingUpgrade struct {
	Schema string `json:"schema_version"`
	Before State  `json:"before"`
}

func maintenanceLock(root string) (func(), error) {
	root, err := safeRoot(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return nil, errors.New("maintenance directory unavailable")
	}
	// Outside root so a locked installation can still be uninstalled on Windows.
	f, err := os.OpenFile(root+".maintenance.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("installation maintenance unavailable")
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, errors.New("installation maintenance already in progress")
	}
	return func() { f.Close() }, nil
}

func noPending(root string) error {
	_, err := os.Lstat(filepath.Join(root, pendingName))
	if os.IsNotExist(err) {
		return nil
	}
	return errors.New("interrupted upgrade must be recovered before maintenance")
}

// Upgrade retains both older versions until activation succeeds. On any failure
// after recording intent it restores the exact original state and activates it.
func Upgrade(root, archive, manifest, archiveHash, manifestHash string, activate Activate) (Result, error) {
	if activate == nil {
		return Result{}, errors.New("upgrade requires a health-checked activator")
	}
	unlock, err := maintenanceLock(root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err = noPending(root); err != nil {
		return Result{}, err
	}
	before, err := loadState(root)
	if err != nil {
		return Result{}, err
	}
	if _, err = Current(root); err != nil {
		return Result{}, err
	}
	// Reject invalid packages before creating intent or restarting anything.
	input, err := verifiedFile(manifest, manifestHash)
	if err != nil {
		return Result{}, err
	}
	_, err = parseManifest(input)
	if err != nil {
		return Result{}, err
	}
	if hash, err := HashFile(archive); err != nil || hash != archiveHash {
		return Result{}, errors.New("release payload verification failed")
	}
	if err = writeJSONAtomic(filepath.Join(root, pendingName), pendingUpgrade{"pfremote.pending-upgrade/v1", before}); err != nil {
		return Result{}, err
	}
	next, installErr := install(root, archive, manifest, archiveHash, manifestHash, false)
	if installErr == nil {
		installErr = activate(next)
	}
	if installErr != nil {
		restored, recoveryErr := recoverUpgrade(root, activate)
		if recoveryErr != nil {
			return Result{}, errors.New("upgrade failed; recovery remains pending")
		}
		return restored, errors.New("upgrade failed; previous version restored")
	}
	if err = os.Remove(filepath.Join(root, pendingName)); err != nil {
		return Result{}, errors.New("upgrade completion could not be recorded")
	}
	_ = pruneObsoleteVersions(root, next.Current, next.Previous)
	return next, nil
}

// RecoverUpgrade is called before normal startup. With no pending transaction it
// returns the current version without invoking activation. Bad journals fail closed.
func RecoverUpgrade(root string, activate Activate) (Result, error) {
	if activate == nil {
		return Result{}, errors.New("recovery requires a health-checked activator")
	}
	unlock, err := maintenanceLock(root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := noPending(root); err == nil {
		return Current(root)
	}
	return recoverUpgrade(root, activate)
}

func recoverUpgrade(root string, activate Activate) (Result, error) {
	input, err := os.ReadFile(filepath.Join(root, pendingName))
	if err != nil || len(input) > 4096 {
		return Result{}, errors.New("upgrade recovery record unavailable")
	}
	var pending pendingUpgrade
	if json.Unmarshal(input, &pending) != nil || pending.Schema != "pfremote.pending-upgrade/v1" || pending.Before.SchemaVersion != StateSchema || !safeVersion(pending.Before.Current) || (pending.Before.Previous != "" && !safeVersion(pending.Before.Previous)) {
		return Result{}, errors.New("upgrade recovery record invalid")
	}
	old := resultFor(root, pending.Before)
	info, err := os.Stat(old.AppPath)
	if err != nil || !info.Mode().IsRegular() {
		return Result{}, errors.New("previous application unavailable")
	}
	if err = writeJSONAtomic(filepath.Join(root, "current.json"), pending.Before); err != nil {
		return Result{}, err
	}
	if err = activate(old); err != nil {
		return Result{}, errors.New("previous version has not recovered")
	}
	if err = os.Remove(filepath.Join(root, pendingName)); err != nil {
		return Result{}, errors.New("recovery completion could not be recorded")
	}
	return old, nil
}
