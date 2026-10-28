package capabilitysync

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/internal/state"
)

type versionGateway interface {
	SyncVersions(context.Context, enrollment.VersionSyncRequest) (enrollment.VersionSyncResponse, error)
}

// renewSelf cannot renew authority over another device or any cached grant.
// Existing controllers keep their separate authoritative directory pull path.
func (s Synchronizer) renewSelf(ctx context.Context, snapshot state.Snapshot, signer shellbinding.DeviceSigner) (state.Snapshot, bool, error) {
	if len(snapshot.Grants) != 0 || len(snapshot.Devices) != 1 || snapshot.Devices[0].ID != signer.DeviceID() {
		return snapshot, false, nil
	}
	if err := snapshot.Validate(); err != nil {
		return snapshot, false, err
	}
	if snapshot.Devices[0].State == "revoked" || snapshot.Devices[0].IdentityPublicKey != base64.RawURLEncoding.EncodeToString(signer.PublicKey()) {
		return snapshot, false, errors.New("local authorization identity does not match")
	}
	for _, c := range snapshot.Capabilities {
		if c.DeviceID != signer.DeviceID() {
			return snapshot, false, errors.New("local authorization contains a foreign capability")
		}
	}
	gateway, ok := s.Gateway.(versionGateway)
	if !ok {
		return snapshot, false, errors.New("connection service cannot confirm local authorization")
	}
	randomID := s.Random
	if randomID == nil {
		randomID = requestID
	}
	id, err := randomID()
	if err != nil {
		return snapshot, false, err
	}
	version := s.Version
	if version == "" {
		version = "1.0.0"
	}
	request := enrollment.VersionSyncRequest{SchemaVersion: enrollment.SchemaVersion, DeviceID: signer.DeviceID(), RequestID: id, ClientVersion: version}
	signature, err := signer.Sign(enrollment.VersionSyncMessage(request))
	if err != nil {
		return snapshot, false, err
	}
	request.Signature = enrollment.EncodeSignature(signature)
	response, err := gateway.SyncVersions(ctx, request)
	if err != nil {
		return snapshot, false, err
	}
	if response.SchemaVersion != enrollment.SchemaVersion || response.FabricID != snapshot.FabricID || response.DeviceID != signer.DeviceID() || response.DeviceStatus != "active" || response.SupportedClientMajor != 1 || response.DirectoryVersion < snapshot.DirectoryVersion || response.GrantVersion < snapshot.GrantVersion {
		return snapshot, false, errors.New("connection service did not confirm current local authorization")
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	if now.Before(snapshot.CapturedAt) {
		return snapshot, false, errors.New("clock rollback prevents authorization renewal")
	}
	snapshot.CapturedAt = now
	snapshot.DirectoryVersion = response.DirectoryVersion
	snapshot.GrantVersion = response.GrantVersion
	return snapshot, true, nil
}
