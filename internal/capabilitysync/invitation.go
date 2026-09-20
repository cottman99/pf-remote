package capabilitysync

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
)

const (
	InvitationSchema = "pfremote.connection-invitation/v1"
	ConfigSchema     = "pfremote.connection-service/v1"
	maxInviteSize    = 64 << 10
	invitationDomain = "PFREMOTE-CONNECTION-INVITATION-V1"
)

type Invitation struct {
	SchemaVersion  string    `json:"schema_version"`
	GatewayURL     string    `json:"gateway_url"`
	FabricID       string    `json:"fabric_id"`
	OwnerDeviceID  string    `json:"owner_device_id"`
	OwnerPublicKey string    `json:"owner_public_key"`
	Preset         string    `json:"preset"`
	ExpiresAt      time.Time `json:"expires_at"`
	Nonce          string    `json:"nonce"`
	Signature      string    `json:"signature"`
}

type Config struct {
	SchemaVersion  string `json:"schema_version"`
	GatewayURL     string `json:"gateway_url"`
	GatewayCAPEM   string `json:"gateway_ca_pem,omitempty"`
	FabricID       string `json:"fabric_id"`
	OwnerDeviceID  string `json:"owner_device_id"`
	OwnerPublicKey string `json:"owner_public_key"`
	Pull           bool   `json:"pull"`
}

func SignInvitation(gatewayURL, fabricID, preset, nonce string, expiresAt time.Time, signer shellbinding.DeviceSigner) (Invitation, error) {
	if signer == nil {
		return Invitation{}, errors.New("Owner identity is required")
	}
	invitation := Invitation{SchemaVersion: InvitationSchema, GatewayURL: strings.TrimSpace(gatewayURL), FabricID: fabricID, OwnerDeviceID: signer.DeviceID(), OwnerPublicKey: base64.RawURLEncoding.EncodeToString(signer.PublicKey()), Preset: preset, ExpiresAt: expiresAt.UTC(), Nonce: nonce}
	if err := validateInvitation(invitation, time.Now().UTC(), false); err != nil {
		return Invitation{}, err
	}
	signature, err := signer.Sign(invitationMessage(invitation))
	if err != nil {
		return Invitation{}, errors.New("connection invitation could not be signed")
	}
	invitation.Signature = base64.RawURLEncoding.EncodeToString(signature)
	return invitation, nil
}

func ImportInvitationFile(invitationPath, configPath string, now time.Time) (Config, error) {
	config, err := ReadInvitationFile(invitationPath, now)
	if err != nil {
		return Config{}, err
	}
	if err := SaveConfig(configPath, config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func ReadInvitationFile(invitationPath string, now time.Time) (Config, error) {
	payload, err := readBoundedRegularFile(invitationPath)
	if err != nil {
		return Config{}, err
	}
	var invitation Invitation
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&invitation); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Config{}, errors.New("connection invitation is invalid")
	}
	if err := validateInvitation(invitation, now.UTC(), true); err != nil {
		return Config{}, err
	}
	config := Config{SchemaVersion: ConfigSchema, GatewayURL: invitation.GatewayURL, FabricID: invitation.FabricID, OwnerDeviceID: invitation.OwnerDeviceID, OwnerPublicKey: invitation.OwnerPublicKey, Pull: invitation.Preset == "owner"}
	return config, nil
}

func LoadConfig(path string) (Config, error) {
	payload, err := readBoundedRegularFile(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || decoder.Decode(&struct{}{}) != io.EOF || validateConfig(config) != nil {
		return Config{}, errors.New("connection service settings are invalid")
	}
	return config, nil
}

func SaveConfig(path string, config Config) error {
	if validateConfig(config) != nil {
		return errors.New("connection service settings are invalid")
	}
	directory := filepath.Dir(filepath.Clean(path))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return errors.New("connection service settings could not be created")
	}
	temporary, err := os.CreateTemp(directory, ".connection-service-*.json")
	if err != nil {
		return errors.New("connection service settings could not be created")
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
		return errors.New("connection service settings could not be protected")
	}
	if err := json.NewEncoder(temporary).Encode(config); err != nil || temporary.Sync() != nil || temporary.Close() != nil {
		return errors.New("connection service settings could not be saved")
	}
	targetPath := filepath.Clean(path)
	backupPath := ""
	if info, statErr := os.Lstat(targetPath); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("connection service settings target is unsafe")
		}
		backup, backupErr := os.CreateTemp(directory, ".connection-service-previous-*.json")
		if backupErr != nil {
			return errors.New("connection service settings could not be replaced")
		}
		backupPath = backup.Name()
		if closeErr := backup.Close(); closeErr != nil || os.Remove(backupPath) != nil {
			return errors.New("connection service settings could not be replaced")
		}
		if renameErr := os.Rename(targetPath, backupPath); renameErr != nil {
			return errors.New("connection service settings could not be replaced")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return errors.New("connection service settings target could not be inspected")
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		if backupPath != "" {
			_ = os.Rename(backupPath, targetPath)
		}
		return errors.New("connection service settings could not be activated")
	}
	keep = true
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}
	return nil
}

func DefaultConfigPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("connection service settings location is unavailable")
	}
	return filepath.Join(root, "PF Remote", "connection", "connection-service-v1.json"), nil
}

func validateInvitation(invitation Invitation, now time.Time, verifySignature bool) error {
	if invitation.SchemaVersion != InvitationSchema || enrollment.ValidateGatewayURL(invitation.GatewayURL) != nil || invitation.FabricID == "" || (invitation.Preset != "owner" && invitation.Preset != "device") || invitation.Nonce == "" || len(invitation.Nonce) > 256 || !now.Before(invitation.ExpiresAt) {
		return errors.New("connection invitation is invalid or expired")
	}
	publicKey, err := base64.RawURLEncoding.Strict().DecodeString(invitation.OwnerPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("connection invitation Owner identity is invalid")
	}
	deviceID, err := identity.DeviceIDFromPublicKey(ed25519.PublicKey(publicKey))
	if err != nil || deviceID != invitation.OwnerDeviceID {
		return errors.New("connection invitation Owner identity does not match")
	}
	if verifySignature {
		signature, err := base64.RawURLEncoding.Strict().DecodeString(invitation.Signature)
		if err != nil || !identity.Verify(ed25519.PublicKey(publicKey), invitationMessage(invitation), signature) {
			return errors.New("connection invitation signature is invalid")
		}
	}
	return nil
}

func validateConfig(config Config) error {
	if config.SchemaVersion != ConfigSchema || enrollment.ValidateGatewayURL(config.GatewayURL) != nil || config.FabricID == "" || config.OwnerDeviceID == "" || config.OwnerPublicKey == "" {
		return errors.New("invalid config")
	}
	publicKey, err := base64.RawURLEncoding.Strict().DecodeString(config.OwnerPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("invalid config")
	}
	deviceID, err := identity.DeviceIDFromPublicKey(ed25519.PublicKey(publicKey))
	if err != nil || deviceID != config.OwnerDeviceID {
		return errors.New("invalid config")
	}
	if config.GatewayCAPEM != "" {
		if !strings.HasPrefix(strings.TrimSpace(config.GatewayURL), "https://") {
			return errors.New("invalid config")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(config.GatewayCAPEM)) {
			return errors.New("invalid config")
		}
	}
	return nil
}

func invitationMessage(invitation Invitation) []byte {
	return []byte(strings.Join([]string{invitationDomain, invitation.GatewayURL, invitation.FabricID, invitation.OwnerDeviceID, invitation.OwnerPublicKey, invitation.Preset, invitation.ExpiresAt.UTC().Format(time.RFC3339Nano), invitation.Nonce}, "\n"))
}

func readBoundedRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxInviteSize {
		return nil, errors.New("connection invitation or settings file is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("connection invitation or settings file could not be opened")
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxInviteSize+1))
	if err != nil || len(payload) == 0 || len(payload) > maxInviteSize {
		return nil, errors.New("connection invitation or settings file could not be read safely")
	}
	return payload, nil
}
