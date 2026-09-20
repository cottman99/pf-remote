// Package identity owns the local Device signing identity and its protected store.
package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	SchemaVersion   = "pfremote.device-identity/v1"
	Algorithm       = "Ed25519"
	maxDocumentSize = 64 << 10
)

type protector interface {
	Name() string
	Protect([]byte) ([]byte, error)
	Unprotect([]byte) ([]byte, error)
}

type Identity struct {
	deviceID   string
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func (i Identity) DeviceID() string { return i.deviceID }

func (i Identity) PublicKey() ed25519.PublicKey {
	return append(ed25519.PublicKey(nil), i.publicKey...)
}

func (i Identity) Sign(message []byte) ([]byte, error) {
	if len(i.privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("device identity is not initialized")
	}
	return ed25519.Sign(i.privateKey, message), nil
}

func Verify(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return len(publicKey) == ed25519.PublicKeySize && ed25519.Verify(publicKey, message, signature)
}

func DeviceIDFromPublicKey(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", errors.New("Ed25519 public key has an invalid length")
	}
	return deriveDeviceID(publicKey), nil
}

type Store struct {
	path      string
	protector protector
	random    io.Reader
}

type document struct {
	SchemaVersion  string `json:"schema_version"`
	DeviceID       string `json:"device_id"`
	Algorithm      string `json:"algorithm"`
	PublicKey      string `json:"public_key"`
	KeyProtection  string `json:"key_protection"`
	PrivateKeyBlob string `json:"private_key_blob"`
}

func NewStore(path string) (*Store, error) {
	protector, err := newPlatformProtector()
	if err != nil {
		return nil, fmt.Errorf("initialize identity protection: %w", safeError(err))
	}
	return newStore(path, protector, nil)
}

func newStore(path string, protector protector, random io.Reader) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("identity store path is required")
	}
	if protector == nil || strings.TrimSpace(protector.Name()) == "" {
		return nil, errors.New("identity protector is required")
	}
	return &Store{path: filepath.Clean(path), protector: protector, random: random}, nil
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", safeError(err))
	}
	return filepath.Join(configDir, "PF Remote", "identity", "device-identity-v1.json"), nil
}

func (s *Store) LoadOrCreate() (Identity, error) {
	identity, err := s.load()
	if err == nil {
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Identity{}, err
	}
	return s.create()
}

func (s *Store) load() (Identity, error) {
	info, err := os.Lstat(s.path)
	if err != nil {
		return Identity{}, fmt.Errorf("inspect identity store: %w", safeError(err))
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Identity{}, errors.New("identity store must be a regular file")
	}
	if err := validateStoredFile(info); err != nil {
		return Identity{}, err
	}

	file, err := os.Open(s.path)
	if err != nil {
		return Identity{}, fmt.Errorf("open identity store: %w", safeError(err))
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxDocumentSize+1))
	if err != nil {
		return Identity{}, fmt.Errorf("read identity store: %w", safeError(err))
	}
	if len(content) > maxDocumentSize {
		return Identity{}, errors.New("identity store exceeds the size limit")
	}
	return s.decode(content)
}

func (s *Store) decode(content []byte) (Identity, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var stored document
	if err := decoder.Decode(&stored); err != nil {
		return Identity{}, errors.New("identity store is not valid versioned JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Identity{}, errors.New("identity store contains trailing data")
	}
	if stored.SchemaVersion != SchemaVersion || stored.Algorithm != Algorithm {
		return Identity{}, errors.New("identity store uses an unsupported schema or algorithm")
	}
	if stored.KeyProtection != s.protector.Name() {
		return Identity{}, errors.New("identity store protection does not match this operating system")
	}

	publicKey, err := base64.RawStdEncoding.DecodeString(stored.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return Identity{}, errors.New("identity store contains an invalid public key")
	}
	protectedSeed, err := base64.RawStdEncoding.DecodeString(stored.PrivateKeyBlob)
	if err != nil || len(protectedSeed) == 0 {
		return Identity{}, errors.New("identity store contains an invalid protected key blob")
	}
	seed, err := s.protector.Unprotect(protectedSeed)
	clear(protectedSeed)
	if err != nil {
		return Identity{}, fmt.Errorf("unprotect device identity: %w", safeError(err))
	}
	defer clear(seed)
	if len(seed) != ed25519.SeedSize {
		return Identity{}, errors.New("identity store contains invalid protected key material")
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	derivedPublic := privateKey.Public().(ed25519.PublicKey)
	if !bytes.Equal(publicKey, derivedPublic) {
		clear(privateKey)
		return Identity{}, errors.New("identity store public and private keys do not match")
	}
	deviceID := deriveDeviceID(derivedPublic)
	if stored.DeviceID != deviceID {
		clear(privateKey)
		return Identity{}, errors.New("identity store device ID does not match its public key")
	}
	return Identity{deviceID: deviceID, publicKey: append(ed25519.PublicKey(nil), derivedPublic...), privateKey: privateKey}, nil
}

func (s *Store) create() (Identity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(s.random)
	if err != nil {
		return Identity{}, fmt.Errorf("generate device identity: %w", safeError(err))
	}
	defer clear(privateKey)
	seed := privateKey.Seed()
	protectedSeed, err := s.protector.Protect(seed)
	clear(seed)
	if err != nil {
		return Identity{}, fmt.Errorf("protect device identity: %w", safeError(err))
	}
	defer clear(protectedSeed)

	stored := document{
		SchemaVersion:  SchemaVersion,
		DeviceID:       deriveDeviceID(publicKey),
		Algorithm:      Algorithm,
		PublicKey:      base64.RawStdEncoding.EncodeToString(publicKey),
		KeyProtection:  s.protector.Name(),
		PrivateKeyBlob: base64.RawStdEncoding.EncodeToString(protectedSeed),
	}
	content, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return Identity{}, errors.New("encode identity store")
	}
	content = append(content, '\n')
	if err := s.publish(content); err != nil {
		if errors.Is(err, os.ErrExist) {
			return s.load()
		}
		return Identity{}, err
	}
	return s.load()
}

func (s *Store) publish(content []byte) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create identity directory: %w", safeError(err))
	}
	if err := protectDirectory(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".device-identity-*.tmp")
	if err != nil {
		return fmt.Errorf("create identity staging file: %w", safeError(err))
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	failed := true
	defer func() {
		if failed {
			_ = temp.Close()
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect identity staging file: %w", safeError(err))
	}
	if _, err := temp.Write(content); err != nil {
		return fmt.Errorf("write identity staging file: %w", safeError(err))
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync identity staging file: %w", safeError(err))
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close identity staging file: %w", safeError(err))
	}
	failed = false
	if err := os.Link(tempName, s.path); err != nil {
		return fmt.Errorf("publish identity store: %w", safeError(err))
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	return nil
}

func deriveDeviceID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:20])
	return "device-" + strings.ToLower(encoded)
}

func safeError(err error) error {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return pathError.Err
	}
	return err
}
