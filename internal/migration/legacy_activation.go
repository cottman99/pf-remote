package migration

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const LegacyActivationSchema = "pfremote.legacy-compatibility-activation/v1"

type legacyActivation struct {
	SchemaVersion string `json:"schema_version"`
	Source        string `json:"source"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

func LegacyActivationPathForState(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "legacy-compatibility-v1.json")
}

func DefaultLegacyCenterCatalogPath() (string, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return "", errors.New("the existing Windows Center location is unavailable")
	}
	return filepath.Join(programData, "PFRemoteCenter", "catalog.json"), nil
}

func DefaultLegacyManagedFRPConfigPath() (string, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return "", errors.New("the existing Windows Center location is unavailable")
	}
	return filepath.Join(programData, "PFRemoteCenter", "frpc-managed.toml"), nil
}

// EnableLegacyCenter writes only a source-kind marker. It copies no path,
// endpoint, account, or credential from the legacy installation.
func EnableLegacyCenter(path string) error {
	if active, err := LegacyCenterEnabled(path); err == nil && active {
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeLegacyActivation(path, true)
}

// DisableLegacyCenter changes only PF Remote's compatibility preference. The
// legacy catalog, services, connections, and credentials remain untouched.
func DisableLegacyCenter(path string) error {
	active, err := LegacyCenterEnabled(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !active {
		return nil
	}
	return writeLegacyActivation(path, false)
}

func writeLegacyActivation(path string, enabled bool) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return errors.New("legacy compatibility state could not be created")
	}
	temporary, err := os.CreateTemp(directory, ".legacy-compatibility-*.json")
	if err != nil {
		return errors.New("legacy compatibility state could not be created")
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return errors.New("legacy compatibility state could not be protected")
	}
	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(legacyActivation{SchemaVersion: LegacyActivationSchema, Source: "windows-center", Enabled: &enabled}); err != nil || temporary.Sync() != nil || temporary.Close() != nil {
		return errors.New("legacy compatibility state could not be written")
	}
	backupPath := ""
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("legacy compatibility state already exists but is invalid")
		}
		backup, backupErr := os.CreateTemp(directory, ".legacy-compatibility-previous-*.json")
		if backupErr != nil {
			return errors.New("legacy compatibility state could not be replaced")
		}
		backupPath = backup.Name()
		if closeErr := backup.Close(); closeErr != nil || os.Remove(backupPath) != nil {
			return errors.New("legacy compatibility state could not be replaced")
		}
		if renameErr := os.Rename(path, backupPath); renameErr != nil {
			return errors.New("legacy compatibility state could not be replaced")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("legacy compatibility state could not be inspected")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if backupPath != "" {
			_ = os.Rename(backupPath, path)
		}
		return errors.New("legacy compatibility state could not be activated")
	}
	keep = true
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}
	return nil
}

func LegacyCenterEnabled(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 4096 {
		return false, errors.New("legacy compatibility state is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return false, errors.New("legacy compatibility state could not be opened")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	var value legacyActivation
	if err := decoder.Decode(&value); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		value.SchemaVersion != LegacyActivationSchema || value.Source != "windows-center" {
		return false, errors.New("legacy compatibility state is invalid")
	}
	return value.Enabled == nil || *value.Enabled, nil
}
