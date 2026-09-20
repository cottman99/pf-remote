package migration

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

// LegacyCandidate is a safe target catalog paired with private process-local
// routes. Snapshot may be presented to the local action core; Routes may not be
// serialized or persisted as public control-plane state.
type LegacyCandidate struct {
	Snapshot state.Snapshot         `json:"snapshot"`
	Routes   LegacyEndpointRoutes   `json:"-"`
	External LegacyExternalDesktop  `json:"-"`
	Gateway  LegacyGatewayRoutes    `json:"-"`
	Shell    LegacyShellConnections `json:"-"`
}

// ProjectLegacyCenterCandidate gives the existing legacy computers stable PF
// Remote identities without changing the source installation. Actions whose
// identity requirements are not yet proven remain visibly setup-required.
func ProjectLegacyCenterCandidate(source LegacyCenterCompatibility, subjectDeviceID string, capturedAt time.Time) (LegacyCandidate, error) {
	return ProjectLegacyCenterCandidateWithTailscale(source, subjectDeviceID, capturedAt, nil)
}

func ProjectLegacyCenterCandidateWithTailscale(source LegacyCenterCompatibility, subjectDeviceID string, capturedAt time.Time, verifier TailscalePeerVerifier) (LegacyCandidate, error) {
	return projectLegacyCenterCandidate(source, subjectDeviceID, capturedAt, verifier, "")
}

func projectLegacyCenterCandidate(source LegacyCenterCompatibility, subjectDeviceID string, capturedAt time.Time, verifier TailscalePeerVerifier, fabricID string) (LegacyCandidate, error) {
	if strings.TrimSpace(subjectDeviceID) == "" || capturedAt.IsZero() {
		return LegacyCandidate{}, errors.New("legacy candidate metadata is invalid")
	}
	inventoryJSON, err := json.Marshal(source.Inventory)
	if err != nil {
		return LegacyCandidate{}, errors.New("legacy inventory could not be planned")
	}
	plan, err := ParseAndPlan(inventoryJSON)
	if err != nil {
		return LegacyCandidate{}, err
	}
	digest := strings.TrimPrefix(plan.SourceDigest, "sha256:")
	if len(digest) < 16 {
		return LegacyCandidate{}, errors.New("legacy inventory digest is invalid")
	}
	if fabricID == "" {
		fabricID = "fabric-legacy-" + digest[:16]
	}
	snapshot := state.Snapshot{
		SchemaVersion: state.SnapshotSchema, FabricID: fabricID,
		DirectoryVersion: 1, GrantVersion: 1, CapturedAt: capturedAt.UTC(),
		Devices: []contracts.Device{{ID: subjectDeviceID, Alias: "owner-controller", DisplayName: "Owner controller", State: "online"}},
	}
	canonicalByService := make(map[string]string)
	external := LegacyExternalDesktop{}
	shell := LegacyShellConnections{}
	for _, plannedDevice := range plan.Devices {
		snapshot.Devices = append(snapshot.Devices, contracts.Device{
			ID: plannedDevice.MappingKey, Alias: plannedDevice.Alias,
			DisplayName: plannedDevice.DisplayName, State: "online",
		})
		for _, plannedCapability := range plannedDevice.Capabilities {
			connection, exists := source.Connections[plannedCapability.LegacyRef]
			if !exists {
				return LegacyCandidate{}, errors.New("legacy candidate is missing private connection material")
			}
			capability := contracts.Capability{
				ID: plannedCapability.MappingKey, DeviceID: plannedDevice.MappingKey,
				Alias: plannedCapability.Alias, DisplayName: plannedCapability.DisplayName,
				Kind: plannedCapability.Kind, State: "setup-required",
			}
			canonical := targetref.Reference{
				FabricID: fabricID, DeviceID: plannedDevice.MappingKey, CapabilityID: plannedCapability.MappingKey,
			}.String()
			if capability.Kind == contracts.CapabilityShell {
				_ = shell.bind(canonical, connection)
			}
			if profile, ready := legacyDesktopProfile(connection); ready {
				capability.DesktopProfile = profile
				capability.State = "available"
				if profile.Protocol == "external" && !external.bind(canonical, connection) {
					capability.DesktopProfile = nil
					capability.State = "setup-required"
				}
			}
			snapshot.Capabilities = append(snapshot.Capabilities, capability)
			snapshot.Grants = append(snapshot.Grants, contracts.Grant{
				ID:              "grant-" + plannedCapability.MappingKey,
				SubjectDeviceID: subjectDeviceID, CapabilityID: plannedCapability.MappingKey, State: "active",
			})
			canonicalByService[plannedCapability.LegacyRef] = canonical
		}
	}
	if err := snapshot.Validate(); err != nil {
		return LegacyCandidate{}, errors.New("legacy candidate snapshot is invalid")
	}
	routes, err := BindLegacyEndpointRoutes(source, canonicalByService)
	if err != nil {
		return LegacyCandidate{}, err
	}
	gateway := BindLegacyGatewayRoutes(source, canonicalByService)
	return LegacyCandidate{Snapshot: snapshot, Routes: routes.WithTailscaleVerifier(verifier), Gateway: gateway, External: external, Shell: shell}, nil
}

func legacyDesktopProfile(connection LegacyCenterConnection) (*contracts.DesktopProfile, bool) {
	switch strings.ToLower(strings.TrimSpace(connection.Protocol)) {
	case "rdp":
		for _, candidate := range connection.Routes {
			if candidate.Adapter == "lan" && candidate.Address != "" && candidate.Port > 0 || validLegacyGatewayRoute(candidate) {
				return &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}, true
			}
		}
		for _, candidate := range connection.Routes {
			if candidate.Adapter == "tailscale" && candidate.Address != "" && candidate.Port > 0 && candidate.NodeID != "" {
				return &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "tailscale-device"}, true
			}
		}
		if hasEndpointRoute(connection.Routes) {
			return &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}, true
		}
	case "vnc":
		if hasEndpointRoute(connection.Routes) || hasLegacyGatewayRoute(connection.Routes) {
			return &contracts.DesktopProfile{Protocol: "vnc", RenderingEnvironment: "virtual", Authentication: "legacy-vnc-password"}, true
		}
	case "external":
		if _, ready := legacyExternalDefinitionFor(connection); ready {
			return &contracts.DesktopProfile{Protocol: "external", RenderingEnvironment: "physical", Authentication: "legacy-private-executor"}, true
		}
	}
	return nil, false
}

func hasLegacyGatewayRoute(routes []LegacyCenterRoute) bool {
	for _, candidate := range routes {
		if validLegacyGatewayRoute(candidate) {
			return true
		}
	}
	return false
}

func hasEndpointRoute(routes []LegacyCenterRoute) bool {
	for _, candidate := range routes {
		if validLegacyEndpointRoute(candidate) && (candidate.Adapter == "lan" || candidate.Adapter == "tailscale" && candidate.NodeID != "") {
			return true
		}
	}
	return false
}

func validLegacyEndpointRoute(candidate LegacyCenterRoute) bool {
	if candidate.Port < 1 || candidate.Port > 65535 {
		return false
	}
	return (route.Candidate{
		ID: "legacy-compatibility", Adapter: candidate.Adapter, Network: "tcp",
		Address: candidate.Address, Port: uint16(candidate.Port),
	}).Validate() == nil
}
