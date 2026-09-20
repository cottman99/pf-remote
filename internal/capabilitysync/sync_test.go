package capabilitysync

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type syncSigner struct {
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func (s syncSigner) DeviceID() string             { return s.id }
func (s syncSigner) PublicKey() ed25519.PublicKey { return s.public }
func (s syncSigner) Sign(value []byte) ([]byte, error) {
	return ed25519.Sign(s.private, value), nil
}

type syncGateway struct {
	claim     contracts.ShellCapabilityClaim
	published int
}

func (g *syncGateway) PublishShellCapability(_ context.Context, request enrollment.ShellCapabilityPublishRequest) (enrollment.ShellCapabilityPublishResponse, error) {
	g.published++
	return enrollment.ShellCapabilityPublishResponse{SchemaVersion: enrollment.SchemaVersion, DeviceID: request.DeviceID, CapabilityID: request.Binding.CapabilityID, DirectoryVersion: 2, Changed: true}, nil
}

func (g *syncGateway) ListShellCapabilities(context.Context, enrollment.ShellCapabilityListRequest) (enrollment.ShellCapabilityListResponse, error) {
	return enrollment.ShellCapabilityListResponse{SchemaVersion: enrollment.SchemaVersion, FabricID: "fabric-sync", DirectoryVersion: 2, Capabilities: []contracts.ShellCapabilityClaim{g.claim}}, nil
}

func TestSynchronizerPublishesAndImportsExactTarget(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := syncSigner{id: deviceID, public: public, private: private}
	binding, err := shellbinding.Sign("fabric-sync", "shell-main", 1, []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: "AAAAC3NzaC1lZDI1NTE5AAAAIMvF3FK8rr2A2r9iVfj0x8l26LThB5GXxJ7XyQPRZP5Z"}}, signer)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := state.Snapshot{SchemaVersion: state.SnapshotSchema, FabricID: "fabric-sync", DirectoryVersion: 1, GrantVersion: 1, CapturedAt: time.Now(),
		Devices:      []contracts.Device{{ID: deviceID, Alias: "owner", DisplayName: "Owner", State: "online"}},
		Capabilities: []contracts.Capability{{ID: "shell-main", DeviceID: deviceID, Alias: "automation", DisplayName: "Automation", Kind: contracts.CapabilityShell, State: "setup-required", SSHBinding: binding}},
		Grants:       []contracts.Grant{{ID: "grant-shell", SubjectDeviceID: deviceID, CapabilityID: "shell-main", State: "active"}},
	}
	claim := contracts.ShellCapabilityClaim{DeviceID: deviceID, DevicePublicKey: base64.RawURLEncoding.EncodeToString(public), Binding: *binding}
	gateway := &syncGateway{claim: claim}
	ids := []string{"capability-request-0001", "capability-request-0002"}
	syncer := Synchronizer{Gateway: gateway, Pull: true, Random: func() (string, error) { id := ids[0]; ids = ids[1:]; return id, nil }}
	updated, result, err := syncer.Sync(context.Background(), snapshot, signer)
	if err != nil {
		t.Fatal(err)
	}
	if result.Published != 1 || result.Imported != 1 || gateway.published != 1 || updated.Capabilities[0].State != "available" {
		t.Fatalf("result=%#v published=%d updated=%#v", result, gateway.published, updated.Capabilities[0])
	}
}

func TestSynchronizerRejectsInvitationForDifferentFabricOrOwner(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	signer := syncSigner{id: deviceID, public: public, private: private}
	snapshot := state.Snapshot{SchemaVersion: state.SnapshotSchema, FabricID: "fabric-local"}
	syncer := Synchronizer{Gateway: &syncGateway{}, Pull: true, ExpectedFabricID: "fabric-other", ExpectedOwnerDeviceID: deviceID}
	if _, _, err := syncer.Sync(context.Background(), snapshot, signer); err == nil {
		t.Fatal("different Fabric invitation accepted")
	}
	syncer.ExpectedFabricID = snapshot.FabricID
	syncer.ExpectedOwnerDeviceID = "dev_different"
	if _, _, err := syncer.PullClaims(context.Background(), snapshot, signer); err == nil {
		t.Fatal("different Owner invitation accepted")
	}
}
