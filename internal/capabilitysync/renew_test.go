package capabilitysync

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type renewalGateway struct {
	syncGateway
	response enrollment.VersionSyncResponse
	err      error
	public   ed25519.PublicKey
	calls    int
}

func (g *renewalGateway) SyncVersions(_ context.Context, r enrollment.VersionSyncRequest) (enrollment.VersionSyncResponse, error) {
	g.calls++
	sig, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(g.public, enrollment.VersionSyncMessage(r), sig) {
		return g.response, errors.New("invalid identity signature")
	}
	return g.response, g.err
}
func renewalFixture(t *testing.T) (state.Snapshot, syncSigner, *renewalGateway, time.Time) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.DeviceIDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	snapshot := state.Snapshot{SchemaVersion: state.SnapshotSchema, FabricID: "fabric-renewal", DirectoryVersion: 2, GrantVersion: 3, CapturedAt: now.Add(-8 * 24 * time.Hour), Devices: []contracts.Device{{ID: id, Alias: "node", State: "online", IdentityPublicKey: base64.RawURLEncoding.EncodeToString(pub)}}}
	gateway := &renewalGateway{public: pub, response: enrollment.VersionSyncResponse{SchemaVersion: enrollment.SchemaVersion, FabricID: snapshot.FabricID, DeviceID: id, DeviceStatus: "active", SupportedClientMajor: 1, DirectoryVersion: 3, GrantVersion: 4}}
	return snapshot, syncSigner{id: id, public: pub, private: key}, gateway, now
}
func TestControlledNodeRenewsExpiredSelfWithExistingIdentity(t *testing.T) {
	before, signer, g, now := renewalFixture(t)
	after, result, err := (Synchronizer{Gateway: g, Now: func() time.Time { return now }}).Sync(context.Background(), before, signer)
	if err != nil || !result.Renewed || !after.CapturedAt.Equal(now) || !reflect.DeepEqual(before.Devices, after.Devices) || len(after.Grants) != 0 || g.calls != 1 {
		t.Fatalf("renewal result=%+v err=%v", result, err)
	}
	if !before.CapturedAt.Equal(now.Add(-8 * 24 * time.Hour)) {
		t.Fatal("original snapshot mutated")
	}
}
func TestControlledNodeRenewalFailsClosed(t *testing.T) {
	for _, name := range []string{"offline", "revoked", "wrong-device", "wrong-fabric", "backwards-directory", "backwards-grants", "schema", "client-major", "clock"} {
		t.Run(name, func(t *testing.T) {
			before, signer, g, now := renewalFixture(t)
			switch name {
			case "offline":
				g.err = errors.New("offline")
			case "revoked":
				g.response.DeviceStatus = "revoked"
			case "wrong-device":
				g.response.DeviceID = "device-other"
			case "wrong-fabric":
				g.response.FabricID = "fabric-other"
			case "backwards-directory":
				g.response.DirectoryVersion = 1
			case "backwards-grants":
				g.response.GrantVersion = 1
			case "schema":
				g.response.SchemaVersion = "unknown"
			case "client-major":
				g.response.SupportedClientMajor = 2
			case "clock":
				now = before.CapturedAt.Add(-time.Second)
			}
			after, result, err := (Synchronizer{Gateway: g, Now: func() time.Time { return now }}).Sync(context.Background(), before, signer)
			if err == nil || result.Renewed || !reflect.DeepEqual(before, after) {
				t.Fatalf("failure renewed snapshot: %+v %v", result, err)
			}
		})
	}
}
func TestControlledRenewalNeverRefreshesForeignAuthority(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		before, signer, g, now := renewalFixture(t)
		if foreign {
			before.Devices = append(before.Devices, contracts.Device{ID: "device-foreign", Alias: "foreign"})
		} else {
			before.Grants = []contracts.Grant{{ID: "grant-foreign"}}
		}
		after, result, err := (Synchronizer{Gateway: g, Now: func() time.Time { return now }}).Sync(context.Background(), before, signer)
		if err != nil || result.Renewed || g.calls != 0 || !reflect.DeepEqual(before, after) {
			t.Fatalf("foreign authority renewed: %+v %v", result, err)
		}
	}
}
