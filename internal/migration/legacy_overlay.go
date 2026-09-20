package migration

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

// ProjectLegacyCenterOverlay adds one separately maintained legacy-shaped
// catalog to an existing candidate. The base Fabric, subject Device, existing
// targets, and process-local private bindings remain authoritative.
func ProjectLegacyCenterOverlay(base LegacyCandidate, source LegacyCenterCompatibility, subjectDeviceID string, capturedAt time.Time, verifier TailscalePeerVerifier) (LegacyCandidate, error) {
	if err := base.Snapshot.Validate(); err != nil || strings.TrimSpace(subjectDeviceID) == "" || capturedAt.IsZero() {
		return LegacyCandidate{}, errors.New("legacy overlay metadata is invalid")
	}
	if !snapshotHasDevice(base.Snapshot, subjectDeviceID) {
		return LegacyCandidate{}, errors.New("legacy overlay subject Device is unavailable")
	}
	overlay, err := projectLegacyCenterCandidate(source, subjectDeviceID, capturedAt, verifier, base.Snapshot.FabricID)
	if err != nil {
		return LegacyCandidate{}, err
	}
	mergedSnapshot, err := mergeLegacySnapshots(base.Snapshot, overlay.Snapshot, subjectDeviceID, capturedAt)
	if err != nil {
		return LegacyCandidate{}, err
	}
	routes, err := mergeLegacyEndpointRoutes(base.Routes, overlay.Routes)
	if err != nil {
		return LegacyCandidate{}, err
	}
	gateway, err := mergeLegacyGatewayRoutes(base.Gateway, overlay.Gateway)
	if err != nil {
		return LegacyCandidate{}, err
	}
	external, err := mergeLegacyExternalDesktop(base.External, overlay.External)
	if err != nil {
		return LegacyCandidate{}, err
	}
	shell, err := mergeLegacyShellConnections(base.Shell, overlay.Shell)
	if err != nil {
		return LegacyCandidate{}, err
	}
	return LegacyCandidate{Snapshot: mergedSnapshot, Routes: routes, Gateway: gateway, External: external, Shell: shell}, nil
}

func snapshotHasDevice(snapshot state.Snapshot, deviceID string) bool {
	for _, device := range snapshot.Devices {
		if device.ID == deviceID {
			return true
		}
	}
	return false
}

func mergeLegacySnapshots(base, overlay state.Snapshot, subjectDeviceID string, capturedAt time.Time) (state.Snapshot, error) {
	if base.FabricID != overlay.FabricID {
		return state.Snapshot{}, errors.New("legacy overlay Fabric identity conflicts with the base")
	}
	merged := base
	merged.CapturedAt = capturedAt.UTC()
	merged.Devices = append([]contracts.Device(nil), base.Devices...)
	merged.Capabilities = append([]contracts.Capability(nil), base.Capabilities...)
	merged.Grants = append([]contracts.Grant(nil), base.Grants...)

	devicesByID := make(map[string]contracts.Device, len(base.Devices))
	deviceAliases := make(map[string]struct{}, len(base.Devices))
	for _, device := range base.Devices {
		devicesByID[device.ID] = device
		deviceAliases[strings.ToLower(device.Alias)] = struct{}{}
	}
	capabilityIDs := make(map[string]struct{}, len(base.Capabilities))
	for _, capability := range base.Capabilities {
		capabilityIDs[capability.ID] = struct{}{}
	}
	grantIDs := make(map[string]struct{}, len(base.Grants))
	for _, grant := range base.Grants {
		grantIDs[grant.ID] = struct{}{}
	}
	overlaySubjectSeen := false
	for _, device := range overlay.Devices {
		if device.ID == subjectDeviceID && !overlaySubjectSeen {
			overlaySubjectSeen = true
			continue
		}
		if existing, exists := devicesByID[device.ID]; exists {
			if existing.Alias != device.Alias || existing.DisplayName != device.DisplayName || existing.State != device.State || existing.IdentityPublicKey != device.IdentityPublicKey {
				return state.Snapshot{}, errors.New("legacy overlay Device identity conflicts with the base")
			}
			continue
		}
		if _, exists := deviceAliases[strings.ToLower(device.Alias)]; exists {
			return state.Snapshot{}, errors.New("legacy overlay Device alias conflicts with the base")
		}
		devicesByID[device.ID] = device
		deviceAliases[strings.ToLower(device.Alias)] = struct{}{}
		merged.Devices = append(merged.Devices, device)
	}
	if !overlaySubjectSeen {
		return state.Snapshot{}, errors.New("legacy overlay subject Device is unavailable")
	}
	for _, capability := range overlay.Capabilities {
		if _, exists := capabilityIDs[capability.ID]; exists {
			return state.Snapshot{}, errors.New("legacy overlay Capability identity conflicts with the base")
		}
		if _, exists := devicesByID[capability.DeviceID]; !exists {
			return state.Snapshot{}, errors.New("legacy overlay Capability Device is unavailable")
		}
		capabilityIDs[capability.ID] = struct{}{}
		merged.Capabilities = append(merged.Capabilities, capability)
	}
	for _, grant := range overlay.Grants {
		if _, exists := grantIDs[grant.ID]; exists {
			return state.Snapshot{}, errors.New("legacy overlay Grant identity conflicts with the base")
		}
		grantIDs[grant.ID] = struct{}{}
		merged.Grants = append(merged.Grants, grant)
	}
	if err := merged.Validate(); err != nil {
		return state.Snapshot{}, errors.New("legacy overlay produced an invalid snapshot")
	}
	return merged, nil
}

type overlayPeerKey struct {
	address string
	nodeID  string
}

type overlayTailscaleVerifier struct {
	byPeer map[overlayPeerKey]TailscalePeerVerifier
}

func (v overlayTailscaleVerifier) VerifyPeer(ctx context.Context, address, nodeID string) error {
	verifier := v.byPeer[overlayPeerKey{address: address, nodeID: nodeID}]
	if verifier == nil {
		return errors.New("legacy Tailscale endpoint identity is unavailable")
	}
	return verifier.VerifyPeer(ctx, address, nodeID)
}

func mergeLegacyEndpointRoutes(base, overlay LegacyEndpointRoutes) (LegacyEndpointRoutes, error) {
	result := LegacyEndpointRoutes{byAdapter: make(map[string]map[string][]legacyBoundEndpoint)}
	verifiers := make(map[overlayPeerKey]TailscalePeerVerifier)
	for _, input := range []LegacyEndpointRoutes{base, overlay} {
		for adapter, byTarget := range input.byAdapter {
			if result.byAdapter[adapter] == nil {
				result.byAdapter[adapter] = make(map[string][]legacyBoundEndpoint)
			}
			for target, endpoints := range byTarget {
				if _, exists := result.byAdapter[adapter][target]; exists {
					return LegacyEndpointRoutes{}, errors.New("legacy overlay endpoint route conflicts with the base")
				}
				result.byAdapter[adapter][target] = append([]legacyBoundEndpoint(nil), endpoints...)
				if adapter == "tailscale" {
					for _, endpoint := range endpoints {
						key := overlayPeerKey{address: endpoint.endpoint.Address, nodeID: endpoint.nodeID}
						if _, exists := verifiers[key]; !exists {
							verifiers[key] = input.tailscaleVerifier
						}
					}
				}
			}
		}
	}
	result.tailscaleVerifier = overlayTailscaleVerifier{byPeer: verifiers}
	return result, nil
}

func mergeLegacyGatewayRoutes(base, overlay LegacyGatewayRoutes) (LegacyGatewayRoutes, error) {
	result := LegacyGatewayRoutes{byTarget: make(map[string]LegacyCenterRoute, len(base.byTarget)+len(overlay.byTarget))}
	for target, definition := range base.byTarget {
		result.byTarget[target] = definition
	}
	for target, definition := range overlay.byTarget {
		if _, exists := result.byTarget[target]; exists {
			return LegacyGatewayRoutes{}, errors.New("legacy overlay Gateway route conflicts with the base")
		}
		result.byTarget[target] = definition
	}
	return result, nil
}

func mergeLegacyExternalDesktop(base, overlay LegacyExternalDesktop) (LegacyExternalDesktop, error) {
	result := base
	result.commands = make(map[string]legacyExternalDefinition, len(base.commands)+len(overlay.commands))
	for target, definition := range base.commands {
		result.commands[target] = definition
	}
	for target, definition := range overlay.commands {
		if _, exists := result.commands[target]; exists {
			return LegacyExternalDesktop{}, errors.New("legacy overlay external Desktop conflicts with the base")
		}
		result.commands[target] = definition
	}
	return result, nil
}

func mergeLegacyShellConnections(base, overlay LegacyShellConnections) (LegacyShellConnections, error) {
	result := LegacyShellConnections{users: make(map[string]string, len(base.users)+len(overlay.users))}
	for target, user := range base.users {
		result.users[target] = user
	}
	for target, user := range overlay.users {
		if _, exists := result.users[target]; exists {
			return LegacyShellConnections{}, errors.New("legacy overlay Shell binding conflicts with the base")
		}
		result.users[target] = user
	}
	return result, nil
}
