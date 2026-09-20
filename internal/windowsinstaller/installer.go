package windowsinstaller

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	ManifestSchema = "pfremote.windows-release/v1"
	StateSchema    = "pfremote.windows-installation-state/v1"
	MarkerSchema   = "pfremote.windows-installation/v1"
)

type Manifest struct {
	SchemaVersion string         `json:"schema_version"`
	Product       string         `json:"product"`
	Version       string         `json:"version"`
	Architecture  string         `json:"architecture"`
	CreatedAt     string         `json:"created_at"`
	Files         []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type State struct {
	SchemaVersion string `json:"schema_version"`
	Current       string `json:"current"`
	Previous      string `json:"previous,omitempty"`
}

type marker struct {
	SchemaVersion string `json:"schema_version"`
	Product       string `json:"product"`
}

type Result struct {
	Current  string `json:"current"`
	Previous string `json:"previous,omitempty"`
	AppPath  string `json:"app_path"`
}

var removeVersion = os.RemoveAll

func Install(root, archivePath, manifestPath, expectedArchiveHash, expectedManifestHash string) (Result, error) {
	unlock, err := maintenanceLock(root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := noPending(root); err != nil {
		return Result{}, err
	}
	return install(root, archivePath, manifestPath, expectedArchiveHash, expectedManifestHash, true)
}

func install(root, archivePath, manifestPath, expectedArchiveHash, expectedManifestHash string, prune bool) (Result, error) {
	root, err := safeRoot(root)
	if err != nil {
		return Result{}, err
	}
	manifestBytes, err := verifiedFile(manifestPath, expectedManifestHash)
	if err != nil {
		return Result{}, errors.New("release manifest verification failed")
	}
	if actual, err := HashFile(archivePath); err != nil || actual != strings.ToLower(expectedArchiveHash) {
		return Result{}, errors.New("release payload verification failed")
	}
	manifest, err := parseManifest(manifestBytes)
	if err != nil {
		return Result{}, err
	}

	versions := filepath.Join(root, "versions")
	if err := os.MkdirAll(versions, 0o700); err != nil {
		return Result{}, errors.New("installation directory could not be created")
	}
	previousState, stateErr := loadState(root)
	if stateErr != nil {
		if _, err := os.Lstat(filepath.Join(root, "current.json")); !os.IsNotExist(err) {
			return Result{}, errors.New("existing installation state is invalid")
		}
	}
	// A long-lived Agent host may still have an older MCP executable open. That
	// must not block activating a verified upgrade; cleanup is retried after the
	// state switch and by later maintenance runs.
	if prune {
		_ = pruneObsoleteVersions(root, previousState.Current, previousState.Previous, manifest.Version)
	}
	versionRoot := filepath.Join(versions, manifest.Version)
	if _, err := os.Stat(versionRoot); err == nil {
		if err := verifyDirectory(versionRoot, manifest); err != nil {
			return Result{}, errors.New("installed version conflicts with the verified release")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, errors.New("installed version could not be inspected")
	} else {
		archive, openErr := zip.OpenReader(archivePath)
		if openErr != nil {
			return Result{}, errors.New("release payload could not be opened")
		}
		defer archive.Close()
		stage, stageErr := os.MkdirTemp(versions, ".installing-")
		if stageErr != nil {
			return Result{}, errors.New("installation staging directory could not be created")
		}
		stageOwned := true
		defer func() {
			if stageOwned {
				_ = os.RemoveAll(stage)
			}
		}()
		if err := extractVerified(archive.File, manifest, stage); err != nil {
			return Result{}, err
		}
		if err := os.Rename(stage, versionRoot); err != nil {
			return Result{}, errors.New("verified release could not be activated")
		}
		stageOwned = false
	}

	state := State{SchemaVersion: StateSchema, Current: manifest.Version}
	if previousState.Current != "" && previousState.Current != manifest.Version {
		state.Previous = previousState.Current
	} else {
		state.Previous = previousState.Previous
	}
	if err := writeJSONAtomic(filepath.Join(root, "current.json"), state); err != nil {
		return Result{}, err
	}
	if err := writeJSONAtomic(filepath.Join(root, "installation.json"), marker{SchemaVersion: MarkerSchema, Product: "PF Remote"}); err != nil {
		return Result{}, err
	}
	if prune {
		_ = pruneObsoleteVersions(root, state.Current, state.Previous)
	}
	return resultFor(root, state), nil
}

func pruneObsoleteVersions(root string, keep ...string) error {
	versionsRoot := filepath.Join(root, "versions")
	entries, err := os.ReadDir(versionsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("installed PF Remote versions could not be inspected")
	}
	retained := make(map[string]bool, len(keep))
	for _, version := range keep {
		if version != "" {
			retained[version] = true
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() || retained[entry.Name()] || strings.HasPrefix(entry.Name(), ".installing-") {
			continue
		}
		path := filepath.Join(versionsRoot, entry.Name())
		if err := removeVersion(path); err != nil {
			return errors.New("obsolete PF Remote test versions could not be cleaned up")
		}
	}
	return nil
}

func Rollback(root string) (Result, error) {
	unlock, err := maintenanceLock(root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := noPending(root); err != nil {
		return Result{}, err
	}
	return rollback(root)
}

func rollback(root string) (Result, error) {
	root, err := safeRoot(root)
	if err != nil {
		return Result{}, err
	}
	state, err := loadState(root)
	if err != nil || state.Previous == "" {
		return Result{}, errors.New("no verified previous version is available")
	}
	if _, err := os.Stat(filepath.Join(root, "versions", state.Previous, "PFRemoteCenter.exe")); err != nil {
		return Result{}, errors.New("previous version is unavailable")
	}
	next := State{SchemaVersion: StateSchema, Current: state.Previous, Previous: state.Current}
	if err := writeJSONAtomic(filepath.Join(root, "current.json"), next); err != nil {
		return Result{}, err
	}
	return resultFor(root, next), nil
}

func Current(root string) (Result, error) {
	root, err := safeRoot(root)
	if err != nil {
		return Result{}, err
	}
	state, err := loadState(root)
	if err != nil {
		return Result{}, err
	}
	result := resultFor(root, state)
	if _, err := os.Stat(result.AppPath); err != nil {
		return Result{}, errors.New("current PF Remote version is unavailable")
	}
	return result, nil
}

func Uninstall(root string) error {
	unlock, lockErr := maintenanceLock(root)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	if err := noPending(root); err != nil {
		return err
	}
	root, err := safeRoot(root)
	if err != nil {
		return err
	}
	input, err := os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil {
		return errors.New("PF Remote installation marker is unavailable")
	}
	var value marker
	if json.Unmarshal(input, &value) != nil || value.SchemaVersion != MarkerSchema || value.Product != "PF Remote" {
		return errors.New("PF Remote installation marker is invalid")
	}
	if err := os.RemoveAll(root); err != nil {
		return errors.New("PF Remote installation could not be removed")
	}
	return nil
}

func parseManifest(input []byte) (Manifest, error) {
	var value Manifest
	decoder := json.NewDecoder(strings.NewReader(string(input)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || value.SchemaVersion != ManifestSchema || value.Product != "PF Remote" ||
		!safeVersion(value.Version) || value.Architecture == "" || len(value.Files) == 0 {
		return Manifest{}, errors.New("release manifest is invalid")
	}
	previous := ""
	for _, file := range value.Files {
		clean := filepath.ToSlash(filepath.Clean(file.Path))
		if clean != file.Path || clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(file.Path) ||
			file.Size < 0 || len(file.SHA256) != 64 || strings.ToLower(file.SHA256) != file.SHA256 {
			return Manifest{}, errors.New("release manifest contains an unsafe file")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return Manifest{}, errors.New("release manifest contains an invalid hash")
		}
		if previous != "" && strings.Compare(strings.ToLower(file.Path), strings.ToLower(previous)) <= 0 {
			return Manifest{}, errors.New("release manifest files are not uniquely sorted")
		}
		previous = file.Path
	}
	return value, nil
}

func extractVerified(entries []*zip.File, manifest Manifest, destination string) error {
	byPath := make(map[string]*zip.File, len(entries))
	for _, entry := range entries {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 || filepath.ToSlash(filepath.Clean(entry.Name)) != entry.Name || strings.HasPrefix(entry.Name, "../") {
			return errors.New("release archive contains an unsafe entry")
		}
		if _, exists := byPath[entry.Name]; exists {
			return errors.New("release archive contains a duplicate entry")
		}
		byPath[entry.Name] = entry
	}
	if len(byPath) != len(manifest.Files) {
		return errors.New("release archive does not match its manifest")
	}
	for _, expected := range manifest.Files {
		entry, exists := byPath[expected.Path]
		if !exists || int64(entry.UncompressedSize64) != expected.Size {
			return errors.New("release archive does not match its manifest")
		}
		target := filepath.Join(destination, filepath.FromSlash(expected.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return errors.New("release directory could not be created")
		}
		input, err := entry.Open()
		if err != nil {
			return errors.New("release entry could not be opened")
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			input.Close()
			return errors.New("release entry could not be created")
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(output, hash), input)
		closeErr := output.Close()
		input.Close()
		if copyErr != nil || closeErr != nil || written != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
			return errors.New("release entry verification failed")
		}
	}
	return nil
}

func verifyDirectory(root string, manifest Manifest) error {
	paths := make([]string, 0, len(manifest.Files))
	for _, expected := range manifest.Files {
		path := filepath.Join(root, filepath.FromSlash(expected.Path))
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, copyErr := io.Copy(hash, input)
		input.Close()
		if copyErr != nil || size != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
			return errors.New("installed release verification failed")
		}
		paths = append(paths, expected.Path)
	}
	sort.Strings(paths)
	return nil
}

func verifiedFile(path, expected string) ([]byte, error) {
	if len(expected) != 64 {
		return nil, errors.New("expected release hash is invalid")
	}
	input, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(input)
	if hex.EncodeToString(hash[:]) != strings.ToLower(expected) {
		return nil, errors.New("release hash mismatch")
	}
	return input, nil
}

func loadState(root string) (State, error) {
	input, err := os.ReadFile(filepath.Join(root, "current.json"))
	if err != nil {
		return State{}, errors.New("PF Remote installation state is unavailable")
	}
	var value State
	if json.Unmarshal(input, &value) != nil || value.SchemaVersion != StateSchema || !safeVersion(value.Current) || (value.Previous != "" && !safeVersion(value.Previous)) {
		return State{}, errors.New("PF Remote installation state is invalid")
	}
	return value, nil
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("installation state directory could not be created")
	}
	input, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.New("installation state could not be encoded")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pfremote-state-")
	if err != nil {
		return errors.New("installation state could not be staged")
	}
	temporaryPath := temporary.Name()
	defer temporary.Close()
	defer os.Remove(temporaryPath)
	if temporary.Chmod(0o600) != nil || func() error { _, err := temporary.Write(append(input, '\n')); return err }() != nil || temporary.Sync() != nil || temporary.Close() != nil {
		return errors.New("installation state could not be written")
	}
	if err := replaceState(temporaryPath, path); err != nil {
		return errors.New("installation state could not be activated")
	}
	return nil
}

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?$`)

func safeVersion(value string) bool { return len(value) <= 80 && versionPattern.MatchString(value) }

func resultFor(root string, state State) Result {
	return Result{Current: state.Current, Previous: state.Previous, AppPath: filepath.Join(root, "versions", state.Current, "PFRemoteCenter.exe")}
}

func safeRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("installation root is required")
	}
	resolved, err := filepath.Abs(root)
	if err != nil || filepath.Dir(resolved) == resolved || filepath.Base(resolved) == "." {
		return "", errors.New("installation root is unsafe")
	}
	return filepath.Clean(resolved), nil
}

func HashFile(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
