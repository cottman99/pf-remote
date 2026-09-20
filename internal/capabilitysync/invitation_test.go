package capabilitysync

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
)

func TestSignedInvitationImportsProtectedConnectionSettings(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := syncSigner{id: deviceID, public: public, private: private}
	now := time.Now().UTC()
	invitation, err := SignInvitation("http://127.0.0.1:43210", "fabric-invited", "owner", "invitation-nonce-0001", now.Add(time.Hour), signer)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(invitation)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	invitePath := filepath.Join(dir, "join.pfremote-link")
	configPath := filepath.Join(dir, "config", "connection-service-v1.json")
	if err := os.WriteFile(invitePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := ImportInvitationFile(invitePath, configPath, now)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Pull || config.FabricID != "fabric-invited" || config.GatewayURL != "http://127.0.0.1:43210" {
		t.Fatalf("config = %#v", config)
	}
	loaded, err := LoadConfig(configPath)
	if err != nil || loaded != config {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	invitation.GatewayURL = "http://127.0.0.1:43211"
	tampered, _ := json.Marshal(invitation)
	if err := os.WriteFile(invitePath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportInvitationFile(invitePath, filepath.Join(dir, "tampered.json"), now); err == nil {
		t.Fatal("tampered invitation accepted")
	}
}

func TestInvitationRejectsRemotePlaintextAndExpiry(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, _ := identity.DeviceIDFromPublicKey(public)
	signer := syncSigner{id: deviceID, public: public, private: private}
	if _, err := SignInvitation("http://192.0.2.20", "fabric-test", "device", "nonce-0001", time.Now().Add(time.Hour), signer); err == nil {
		t.Fatal("remote plaintext invitation accepted")
	}
	if _, err := SignInvitation("https://gateway.example.invalid", "fabric-test", "device", "nonce-0002", time.Now().Add(-time.Minute), signer); err == nil {
		t.Fatal("expired invitation accepted")
	}
}

func TestSaveConfigSafelyReplacesExistingSettings(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "connection", "connection-service-v1.json")
	first := Config{SchemaVersion: ConfigSchema, GatewayURL: "http://127.0.0.1:43210", FabricID: "fabric-first", OwnerDeviceID: deviceID, OwnerPublicKey: base64.RawURLEncoding.EncodeToString(public), Pull: true}
	second := first
	second.FabricID = "fabric-second"
	if err := SaveConfig(path, first); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(path, second); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil || loaded != second {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("replacement residue = %#v", entries)
	}
}

func TestConfigRejectsPrivateCAOnPlaintextGateway(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{SchemaVersion: ConfigSchema, GatewayURL: "http://127.0.0.1:43210", GatewayCAPEM: "not-a-certificate", FabricID: "fabric-test", OwnerDeviceID: deviceID, OwnerPublicKey: base64.RawURLEncoding.EncodeToString(public)}
	if err := SaveConfig(filepath.Join(t.TempDir(), "config.json"), config); err == nil {
		t.Fatal("private CA on plaintext Gateway was accepted")
	}
}
