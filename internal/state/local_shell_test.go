package state

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type localShellSigner struct {
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func (s localShellSigner) DeviceID() string             { return s.id }
func (s localShellSigner) PublicKey() ed25519.PublicKey { return s.public }
func (s localShellSigner) Sign(value []byte) ([]byte, error) {
	return ed25519.Sign(s.private, value), nil
}

func TestBindLocalShellCapabilitiesAttestsOnlyTheLocalDevice(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := localShellSigner{id: deviceID, public: public, private: private}
	now := time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		SchemaVersion: SnapshotSchema, FabricID: "fabric-test", DirectoryVersion: 3, GrantVersion: 2, CapturedAt: now,
		Devices:      []contracts.Device{{ID: deviceID, Alias: "local", DisplayName: "Local", State: "online"}, {ID: "device-other", Alias: "other", DisplayName: "Other", State: "online"}},
		Capabilities: []contracts.Capability{{ID: "shell-local", DeviceID: deviceID, Alias: "shell", DisplayName: "Shell", Kind: contracts.CapabilityShell, State: "setup-required"}, {ID: "shell-other", DeviceID: "device-other", Alias: "shell", DisplayName: "Shell", Kind: contracts.CapabilityShell, State: "setup-required"}},
		Grants:       []contracts.Grant{{ID: "grant-local", SubjectDeviceID: deviceID, CapabilityID: "shell-local", State: "active"}},
	}
	keys := []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: syntheticStateHostKey}}
	updated, count, err := BindLocalShellCapabilities(snapshot, signer, keys)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || updated.DirectoryVersion != 4 || !updated.CapturedAt.Equal(now) {
		t.Fatalf("count=%d directory=%d captured=%v", count, updated.DirectoryVersion, updated.CapturedAt)
	}
	if updated.Devices[0].IdentityPublicKey != base64.RawURLEncoding.EncodeToString(public) {
		t.Fatal("local Device identity was not published")
	}
	if updated.Capabilities[0].SSHBinding == nil || updated.Capabilities[1].SSHBinding != nil {
		t.Fatalf("capabilities = %#v", updated.Capabilities)
	}
	if err := shellbinding.Verify(updated.Devices[0], updated.Capabilities[0], updated.FabricID); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CommitSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitSnapshot(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := store.LoadLatestValidSnapshot(context.Background())
	if err != nil || loaded.DirectoryVersion != 4 || loaded.Capabilities[0].SSHBinding == nil {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

const syntheticStateHostKey = "AAAAC3NzaC1lZDI1NTE5AAAAIMvF3FK8rr2A2r9iVfj0x8l26LThB5GXxJ7XyQPRZP5Z"
