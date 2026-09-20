// Package capabilitysync moves Device-signed capability identity through the
// Gateway without making routes or addresses part of target identity.
package capabilitysync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/internal/state"
)

type Gateway interface {
	PublishShellCapability(context.Context, enrollment.ShellCapabilityPublishRequest) (enrollment.ShellCapabilityPublishResponse, error)
	ListShellCapabilities(context.Context, enrollment.ShellCapabilityListRequest) (enrollment.ShellCapabilityListResponse, error)
}

type Synchronizer struct {
	Gateway               Gateway
	Random                func() (string, error)
	Version               string
	Pull                  bool
	ExpectedFabricID      string
	ExpectedOwnerDeviceID string
}

type Result struct {
	Published int
	Imported  int
}

// Sync publishes bindings owned by the local Device and then attempts an
// Owner-authorized full read. A non-Owner Device is expected to be denied the
// read and still completes its publish path.
func (s Synchronizer) Sync(ctx context.Context, snapshot state.Snapshot, signer shellbinding.DeviceSigner) (state.Snapshot, Result, error) {
	if s.Gateway == nil || signer == nil {
		return snapshot, Result{}, errors.New("capability synchronization is not configured")
	}
	if s.ExpectedFabricID != "" && snapshot.FabricID != s.ExpectedFabricID {
		return snapshot, Result{}, errors.New("connection invitation does not match this Fabric")
	}
	version := s.Version
	if version == "" {
		version = "1.0.0"
	}
	randomID := s.Random
	if randomID == nil {
		randomID = requestID
	}
	result := Result{}
	for _, capability := range snapshot.Capabilities {
		if capability.DeviceID != signer.DeviceID() || capability.SSHBinding == nil {
			continue
		}
		id, err := randomID()
		if err != nil {
			return snapshot, result, errors.New("capability synchronization request identity is unavailable")
		}
		request := enrollment.ShellCapabilityPublishRequest{SchemaVersion: enrollment.SchemaVersion, DeviceID: signer.DeviceID(), Binding: *capability.SSHBinding, RequestID: id, ClientVersion: version}
		signature, err := signer.Sign(enrollment.ShellCapabilityPublishMessage(request))
		if err != nil {
			return snapshot, result, errors.New("capability publication could not be signed")
		}
		request.Signature = enrollment.EncodeSignature(signature)
		if _, err := s.Gateway.PublishShellCapability(ctx, request); err != nil {
			return snapshot, result, err
		}
		result.Published++
	}
	if !s.Pull {
		return snapshot, result, nil
	}
	updated, imported, err := s.PullClaims(ctx, snapshot, signer)
	if err != nil {
		return snapshot, result, err
	}
	result.Imported = imported
	return updated, result, nil
}

func (s Synchronizer) PullClaims(ctx context.Context, snapshot state.Snapshot, signer shellbinding.DeviceSigner) (state.Snapshot, int, error) {
	if s.Gateway == nil || signer == nil {
		return snapshot, 0, errors.New("capability synchronization is not configured")
	}
	if s.ExpectedFabricID != "" && snapshot.FabricID != s.ExpectedFabricID {
		return snapshot, 0, errors.New("connection invitation does not match this Fabric")
	}
	if s.ExpectedOwnerDeviceID != "" && signer.DeviceID() != s.ExpectedOwnerDeviceID {
		return snapshot, 0, errors.New("connection invitation does not match this Owner")
	}
	version := s.Version
	if version == "" {
		version = "1.0.0"
	}
	randomID := s.Random
	if randomID == nil {
		randomID = requestID
	}
	id, err := randomID()
	if err != nil {
		return snapshot, 0, errors.New("capability synchronization request identity is unavailable")
	}
	list := enrollment.ShellCapabilityListRequest{SchemaVersion: enrollment.SchemaVersion, OwnerDeviceID: signer.DeviceID(), RequestID: id, ClientVersion: version}
	signature, err := signer.Sign(enrollment.ShellCapabilityListMessage(list))
	if err != nil {
		return snapshot, 0, errors.New("capability directory request could not be signed")
	}
	list.Signature = enrollment.EncodeSignature(signature)
	response, err := s.Gateway.ListShellCapabilities(ctx, list)
	if err != nil {
		return snapshot, 0, err
	}
	updated, imported, err := state.ApplyShellCapabilityClaims(snapshot, response.FabricID, response.DirectoryVersion, response.Capabilities)
	if err != nil {
		return snapshot, 0, err
	}
	return updated, imported, nil
}

func requestID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "capability-" + hex.EncodeToString(value), nil
}
