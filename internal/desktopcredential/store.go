// Package desktopcredential stores viewer credentials bound to immutable Desktop targets.
package desktopcredential

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const schemaVersion = "pfremote.desktop-credential/v1"

var ErrNotFound = errors.New("desktop credential is not configured")

type protector interface {
	Name() string
	Protect([]byte) ([]byte, error)
	Unprotect([]byte) ([]byte, error)
}

type Store struct {
	root      string
	protector protector
}

type document struct {
	SchemaVersion  string `json:"schema_version"`
	TargetDigest   string `json:"target_digest"`
	Protection     string `json:"protection"`
	CredentialBlob string `json:"credential_blob"`
}

func DefaultRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("locate desktop credential storage")
	}
	return filepath.Join(root, "PF Remote", "credentials", "desktop-v1"), nil
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("desktop credential root is required")
	}
	protection, err := newPlatformProtector()
	if err != nil {
		return nil, err
	}
	return &Store{root: filepath.Clean(root), protector: protection}, nil
}

func newStore(root string, protection protector) *Store {
	return &Store{root: filepath.Clean(root), protector: protection}
}

func (s *Store) Save(canonicalTarget, password string) error {
	canonicalTarget = strings.TrimSpace(canonicalTarget)
	if canonicalTarget == "" || password == "" {
		return errors.New("desktop target and credential are required")
	}
	plaintext := []byte(password)
	defer clear(plaintext)
	protected, err := s.protector.Protect(plaintext)
	if err != nil {
		return errors.New("protect desktop credential")
	}
	defer clear(protected)
	digest := targetDigest(canonicalTarget)
	encoded, err := json.Marshal(document{
		SchemaVersion:  schemaVersion,
		TargetDigest:   digest,
		Protection:     s.protector.Name(),
		CredentialBlob: base64.RawStdEncoding.EncodeToString(protected),
	})
	if err != nil {
		return errors.New("encode desktop credential")
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return errors.New("create desktop credential storage")
	}
	temporary, err := os.CreateTemp(s.root, ".desktop-credential-*.tmp")
	if err != nil {
		return errors.New("stage desktop credential")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return errors.New("protect desktop credential file")
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		_ = temporary.Close()
		return errors.New("write desktop credential")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("sync desktop credential")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close desktop credential")
	}
	destination := filepath.Join(s.root, digest+".json")
	if err := os.Rename(temporaryPath, destination); err != nil {
		return errors.New("publish desktop credential")
	}
	return nil
}

func (s *Store) Load(canonicalTarget string) ([]byte, error) {
	digest := targetDigest(strings.TrimSpace(canonicalTarget))
	content, err := os.ReadFile(filepath.Join(s.root, digest+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil || len(content) > 64<<10 {
		return nil, errors.New("read desktop credential")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var stored document
	if err := decoder.Decode(&stored); err != nil {
		return nil, errors.New("desktop credential state is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("desktop credential state contains trailing data")
	}
	if stored.SchemaVersion != schemaVersion || stored.TargetDigest != digest || stored.Protection != s.protector.Name() {
		return nil, errors.New("desktop credential state does not match this target")
	}
	protected, err := base64.RawStdEncoding.DecodeString(stored.CredentialBlob)
	if err != nil || len(protected) == 0 {
		return nil, errors.New("desktop credential state is invalid")
	}
	defer clear(protected)
	plaintext, err := s.protector.Unprotect(protected)
	if err != nil || len(plaintext) == 0 {
		clear(plaintext)
		return nil, errors.New("desktop credential could not be opened")
	}
	return plaintext, nil
}

func (s *Store) Configured(canonicalTarget string) bool {
	value, err := s.Load(canonicalTarget)
	clear(value)
	return err == nil
}

func targetDigest(target string) string {
	digest := sha256.Sum256([]byte(target))
	return hex.EncodeToString(digest[:])
}
