package state

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const SnapshotSchema = "pfremote.state-snapshot/v1"

type Snapshot struct {
	SchemaVersion    string                 `json:"schema_version"`
	FabricID         string                 `json:"fabric_id"`
	DirectoryVersion uint64                 `json:"directory_version"`
	GrantVersion     uint64                 `json:"grant_version"`
	CapturedAt       time.Time              `json:"captured_at"`
	Devices          []contracts.Device     `json:"devices"`
	Capabilities     []contracts.Capability `json:"capabilities"`
	Grants           []contracts.Grant      `json:"grants"`
}

func SyntheticSnapshot(subjectDeviceID string, capturedAt time.Time) Snapshot {
	fixture := catalog.Synthetic()
	devices := append([]contracts.Device(nil), fixture.Devices...)
	foundSubject := false
	for _, device := range devices {
		if device.ID == subjectDeviceID {
			foundSubject = true
			break
		}
	}
	if !foundSubject {
		devices = append(devices, contracts.Device{
			ID: subjectDeviceID, Alias: "owner-controller",
			DisplayName: "Owner controller", State: "online",
		})
	}
	grants := make([]contracts.Grant, 0, len(fixture.Capabilities))
	for _, capability := range fixture.Capabilities {
		grants = append(grants, contracts.Grant{
			ID: syntheticGrantID(subjectDeviceID, capability.ID), SubjectDeviceID: subjectDeviceID,
			CapabilityID: capability.ID, State: "active",
		})
	}
	return Snapshot{
		SchemaVersion: SnapshotSchema, FabricID: fixture.FabricID,
		DirectoryVersion: 1, GrantVersion: 1, CapturedAt: capturedAt.UTC(),
		Devices: devices, Capabilities: append([]contracts.Capability(nil), fixture.Capabilities...), Grants: grants,
	}
}

func (s Snapshot) Validate() error {
	if s.SchemaVersion != SnapshotSchema {
		return errors.New("state snapshot has an unsupported schema")
	}
	if !validIdentifier(s.FabricID) || s.DirectoryVersion == 0 || s.GrantVersion == 0 || s.CapturedAt.IsZero() {
		return errors.New("state snapshot metadata is invalid")
	}
	devices := make(map[string]contracts.Device, len(s.Devices))
	for _, device := range s.Devices {
		if !validIdentifier(device.ID) || strings.TrimSpace(device.Alias) == "" {
			return errors.New("state snapshot contains an invalid Device")
		}
		if _, exists := devices[device.ID]; exists {
			return fmt.Errorf("state snapshot contains duplicate Device %q", device.ID)
		}
		if device.IdentityPublicKey != "" {
			publicKey, err := base64.RawURLEncoding.Strict().DecodeString(device.IdentityPublicKey)
			if err != nil || len(publicKey) != ed25519.PublicKeySize {
				return errors.New("state snapshot contains an invalid Device identity public key")
			}
			derived, err := identity.DeviceIDFromPublicKey(ed25519.PublicKey(publicKey))
			if err != nil || derived != device.ID {
				return errors.New("state snapshot Device ID does not match its identity public key")
			}
		}
		devices[device.ID] = device
	}
	capabilities := make(map[string]struct{}, len(s.Capabilities))
	for _, capability := range s.Capabilities {
		if !validIdentifier(capability.ID) || strings.TrimSpace(capability.Alias) == "" {
			return errors.New("state snapshot contains an invalid Capability")
		}
		if _, exists := devices[capability.DeviceID]; !exists {
			return errors.New("state snapshot Capability references an unknown Device")
		}
		if capability.Kind != contracts.CapabilityShell && capability.Kind != contracts.CapabilityDesktop {
			return errors.New("state snapshot contains an unsupported Capability kind")
		}
		if capability.SSHBinding != nil {
			device := devices[capability.DeviceID]
			if err := shellbinding.Verify(device, capability, s.FabricID); err != nil {
				return errors.New("state snapshot contains an invalid SSH Capability binding")
			}
		}
		if capability.DesktopProfile != nil {
			profile := capability.DesktopProfile
			authenticationValid := profile.Authentication == "" ||
				profile.Protocol == "rdp" && (profile.Authentication == "windows-sso" || profile.Authentication == "tailscale-device") ||
				profile.Protocol == "vnc" && (profile.Authentication == "x509-route-grant" || profile.Authentication == "tailscale-device" || profile.Authentication == "legacy-vnc-password") ||
				profile.Protocol == "external" && profile.Authentication == "legacy-private-executor"
			if capability.Kind != contracts.CapabilityDesktop ||
				(profile.Protocol != "rdp" && profile.Protocol != "vnc" && profile.Protocol != "external") ||
				(profile.RenderingEnvironment != "virtual" && profile.RenderingEnvironment != "physical") ||
				!authenticationValid {
				return errors.New("state snapshot contains an invalid Desktop Capability profile")
			}
		}
		if _, exists := capabilities[capability.ID]; exists {
			return fmt.Errorf("state snapshot contains duplicate Capability %q", capability.ID)
		}
		capabilities[capability.ID] = struct{}{}
	}
	grants := make(map[string]struct{}, len(s.Grants))
	for _, grant := range s.Grants {
		if !validIdentifier(grant.ID) || grant.State != "active" && grant.State != "revoked" {
			return errors.New("state snapshot contains an invalid Grant")
		}
		if _, exists := devices[grant.SubjectDeviceID]; !exists {
			return errors.New("state snapshot Grant references an unknown subject Device")
		}
		if _, exists := capabilities[grant.CapabilityID]; !exists {
			return errors.New("state snapshot Grant references an unknown Capability")
		}
		if _, exists := grants[grant.ID]; exists {
			return fmt.Errorf("state snapshot contains duplicate Grant %q", grant.ID)
		}
		grants[grant.ID] = struct{}{}
	}
	return nil
}

func syntheticGrantID(subjectDeviceID, capabilityID string) string {
	digest := sha256.Sum256([]byte(subjectDeviceID + "\x00" + capabilityID))
	return "grant-" + hex.EncodeToString(digest[:12])
}

func validIdentifier(value string) bool {
	if len(value) < 3 || len(value) > 96 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if !(character == '-' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}
