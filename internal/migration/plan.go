package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const (
	InventorySchema = "pfremote.legacy-inventory/v1"
	PlanSchema      = "pfremote.migration-plan/v1"
	MaxInputBytes   = 1 << 20
)

type Inventory struct {
	SchemaVersion string         `json:"schema_version"`
	Devices       []LegacyDevice `json:"devices"`
	Grants        []LegacyGrant  `json:"grants,omitempty"`
}

type LegacyDevice struct {
	LegacyRef    string             `json:"legacy_ref"`
	Name         string             `json:"name"`
	DisplayName  string             `json:"display_name"`
	Capabilities []LegacyCapability `json:"capabilities"`
}

type LegacyCapability struct {
	LegacyRef   string                   `json:"legacy_ref"`
	Name        string                   `json:"name"`
	DisplayName string                   `json:"display_name"`
	Kind        contracts.CapabilityKind `json:"kind"`
	PathCount   int                      `json:"path_count"`
}

type LegacyGrant struct {
	LegacyRef        string `json:"legacy_ref"`
	SubjectDeviceRef string `json:"subject_device_ref"`
	CapabilityRef    string `json:"capability_ref"`
	State            string `json:"state"`
}

type Plan struct {
	SchemaVersion string          `json:"schema_version"`
	SourceDigest  string          `json:"source_digest"`
	Devices       []PlannedDevice `json:"devices"`
	Grants        []PlannedGrant  `json:"grants,omitempty"`
	Warnings      []string        `json:"warnings,omitempty"`
}

type PlannedDevice struct {
	MappingKey   string              `json:"mapping_key"`
	LegacyRef    string              `json:"legacy_ref"`
	Alias        string              `json:"alias"`
	DisplayName  string              `json:"display_name"`
	Capabilities []PlannedCapability `json:"capabilities"`
}

type PlannedCapability struct {
	MappingKey  string                   `json:"mapping_key"`
	LegacyRef   string                   `json:"legacy_ref"`
	Alias       string                   `json:"alias"`
	DisplayName string                   `json:"display_name"`
	Kind        contracts.CapabilityKind `json:"kind"`
	PathCount   int                      `json:"path_count"`
}

type PlannedGrant struct {
	MappingKey           string `json:"mapping_key"`
	LegacyRef            string `json:"legacy_ref"`
	SubjectMappingKey    string `json:"subject_mapping_key"`
	CapabilityMappingKey string `json:"capability_mapping_key"`
	State                string `json:"state"`
}

func ParseAndPlan(input []byte) (Plan, error) {
	if len(input) == 0 || len(input) > MaxInputBytes {
		return Plan{}, errors.New("legacy inventory size is invalid")
	}
	var inventory Inventory
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Plan{}, errors.New("legacy inventory is invalid")
	}
	if err := validateInventory(inventory); err != nil {
		return Plan{}, err
	}
	canonical := inventory
	canonical.Devices = append([]LegacyDevice(nil), inventory.Devices...)
	sort.Slice(canonical.Devices, func(left, right int) bool {
		return canonical.Devices[left].LegacyRef < canonical.Devices[right].LegacyRef
	})
	for index := range canonical.Devices {
		canonical.Devices[index].Capabilities = append([]LegacyCapability(nil), canonical.Devices[index].Capabilities...)
		sort.Slice(canonical.Devices[index].Capabilities, func(left, right int) bool {
			return canonical.Devices[index].Capabilities[left].LegacyRef < canonical.Devices[index].Capabilities[right].LegacyRef
		})
	}
	canonical.Grants = append([]LegacyGrant(nil), inventory.Grants...)
	sort.Slice(canonical.Grants, func(left, right int) bool {
		return canonical.Grants[left].LegacyRef < canonical.Grants[right].LegacyRef
	})
	canonicalBytes, err := json.Marshal(canonical)
	if err != nil {
		return Plan{}, errors.New("canonicalize legacy inventory")
	}
	digest := sha256.Sum256(canonicalBytes)
	plan := Plan{SchemaVersion: PlanSchema, SourceDigest: "sha256:" + hex.EncodeToString(digest[:])}
	devices := canonical.Devices
	deviceKeys := make(map[string]string, len(devices))
	capabilityKeys := make(map[string]string)
	usedDeviceAliases := make(map[string]int)
	for _, device := range devices {
		deviceKey := mappingKey("device", device.LegacyRef)
		deviceKeys[device.LegacyRef] = deviceKey
		alias, collided := uniqueAlias(normalizeAlias(device.Name), usedDeviceAliases)
		if collided {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("Device name %q was disambiguated as %q", device.Name, alias))
		}
		planned := PlannedDevice{MappingKey: deviceKey, LegacyRef: device.LegacyRef, Alias: alias, DisplayName: strings.TrimSpace(device.DisplayName)}
		capabilities := append([]LegacyCapability(nil), device.Capabilities...)
		sort.Slice(capabilities, func(left, right int) bool { return capabilities[left].LegacyRef < capabilities[right].LegacyRef })
		usedCapabilityAliases := make(map[string]int)
		for _, capability := range capabilities {
			capabilityKey := mappingKey("capability", device.LegacyRef+"\x00"+capability.LegacyRef)
			capabilityKeys[capability.LegacyRef] = capabilityKey
			capabilityAlias, capabilityCollided := uniqueAlias(normalizeAlias(capability.Name), usedCapabilityAliases)
			if capabilityCollided {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("Capability name %q on %q was disambiguated as %q", capability.Name, alias, capabilityAlias))
			}
			planned.Capabilities = append(planned.Capabilities, PlannedCapability{
				MappingKey: capabilityKey, LegacyRef: capability.LegacyRef, Alias: capabilityAlias,
				DisplayName: strings.TrimSpace(capability.DisplayName), Kind: capability.Kind, PathCount: capability.PathCount,
			})
		}
		plan.Devices = append(plan.Devices, planned)
	}
	grants := canonical.Grants
	for _, grant := range grants {
		plan.Grants = append(plan.Grants, PlannedGrant{
			MappingKey: mappingKey("grant", grant.LegacyRef), LegacyRef: grant.LegacyRef,
			SubjectMappingKey: deviceKeys[grant.SubjectDeviceRef], CapabilityMappingKey: capabilityKeys[grant.CapabilityRef], State: grant.State,
		})
	}
	sort.Strings(plan.Warnings)
	return plan, nil
}

func validateInventory(inventory Inventory) error {
	if inventory.SchemaVersion != InventorySchema || len(inventory.Devices) == 0 || len(inventory.Devices) > 1024 || len(inventory.Grants) > 16384 {
		return errors.New("legacy inventory metadata is invalid")
	}
	deviceRefs := make(map[string]struct{}, len(inventory.Devices))
	capabilityRefs := make(map[string]struct{})
	capabilityCount := 0
	for _, device := range inventory.Devices {
		if !validOpaqueRef(device.LegacyRef) || !validName(device.Name) || !validDisplayName(device.DisplayName) || len(device.Capabilities) == 0 {
			return errors.New("legacy inventory contains an invalid Device")
		}
		if _, exists := deviceRefs[device.LegacyRef]; exists {
			return errors.New("legacy inventory contains a duplicate Device reference")
		}
		deviceRefs[device.LegacyRef] = struct{}{}
		capabilityCount += len(device.Capabilities)
		if capabilityCount > 8192 {
			return errors.New("legacy inventory contains too many Capabilities")
		}
		for _, capability := range device.Capabilities {
			if !validOpaqueRef(capability.LegacyRef) || !validName(capability.Name) || !validDisplayName(capability.DisplayName) ||
				(capability.Kind != contracts.CapabilityShell && capability.Kind != contracts.CapabilityDesktop) || capability.PathCount < 1 || capability.PathCount > 8 {
				return errors.New("legacy inventory contains an invalid Capability")
			}
			if _, exists := capabilityRefs[capability.LegacyRef]; exists {
				return errors.New("legacy inventory contains a duplicate Capability reference")
			}
			capabilityRefs[capability.LegacyRef] = struct{}{}
		}
	}
	grantRefs := make(map[string]struct{}, len(inventory.Grants))
	for _, grant := range inventory.Grants {
		if !validOpaqueRef(grant.LegacyRef) || (grant.State != "active" && grant.State != "revoked") {
			return errors.New("legacy inventory contains an invalid Grant")
		}
		if _, exists := grantRefs[grant.LegacyRef]; exists {
			return errors.New("legacy inventory contains a duplicate Grant reference")
		}
		grantRefs[grant.LegacyRef] = struct{}{}
		if _, exists := deviceRefs[grant.SubjectDeviceRef]; !exists {
			return errors.New("legacy inventory Grant references an unknown Device")
		}
		if _, exists := capabilityRefs[grant.CapabilityRef]; !exists {
			return errors.New("legacy inventory Grant references an unknown Capability")
		}
	}
	return nil
}

func validOpaqueRef(value string) bool {
	if len(value) < 3 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("-_.:/", character)) {
			return false
		}
	}
	return true
}

func validName(value string) bool        { return validVisibleText(value, 64) }
func validDisplayName(value string) bool { return validVisibleText(value, 128) }

func validVisibleText(value string, maximum int) bool {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) < 1 || len([]rune(value)) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func normalizeAlias(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	lastDash := false
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if valid {
			result.WriteRune(character)
			lastDash = false
		} else if !lastDash && result.Len() > 0 {
			result.WriteByte('-')
			lastDash = true
		}
	}
	alias := strings.Trim(result.String(), "-")
	if alias == "" {
		alias = "unnamed"
	}
	if len(alias) > 48 {
		alias = strings.Trim(alias[:48], "-")
	}
	return alias
}

func uniqueAlias(base string, used map[string]int) (string, bool) {
	if used[base] == 0 {
		used[base] = 1
		return base, false
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if used[candidate] == 0 {
			used[candidate] = 1
			return candidate, true
		}
	}
}

func mappingKey(kind, legacyRef string) string {
	digest := sha256.Sum256([]byte(kind + "\x00" + legacyRef))
	return "migration-" + kind + "-" + hex.EncodeToString(digest[:8])
}
