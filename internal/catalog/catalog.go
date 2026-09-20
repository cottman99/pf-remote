package catalog

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

const DefaultAuthorizationTTL = 7 * 24 * time.Hour

// Catalog is an authorization-filtered in-memory view loaded from a committed
// state snapshot. Synthetic returns the deterministic clean-room fixture.
type Catalog struct {
	FabricID        string
	Devices         []contracts.Device
	Capabilities    []contracts.Capability
	Grants          []contracts.Grant
	SubjectDeviceID string
	CapturedAt      time.Time
	Now             func() time.Time
}

func New(fabricID string, devices []contracts.Device, capabilities []contracts.Capability, grants []contracts.Grant, subjectDeviceID string, capturedAt time.Time, now func() time.Time) Catalog {
	if now == nil {
		now = time.Now
	}
	return Catalog{
		FabricID: fabricID, Devices: append([]contracts.Device(nil), devices...),
		Capabilities: append([]contracts.Capability(nil), capabilities...),
		Grants:       append([]contracts.Grant(nil), grants...), SubjectDeviceID: subjectDeviceID,
		CapturedAt: capturedAt.UTC(), Now: now,
	}
}

func Synthetic() Catalog {
	return Catalog{
		FabricID:   "fabric-demo",
		CapturedAt: time.Now().UTC(), Now: time.Now,
		Devices: []contracts.Device{
			{ID: "device-controller", Alias: "workstation", DisplayName: "Demo workstation", State: "online"},
			{ID: "device-compute", Alias: "compute-node", PreviousAliases: []string{"lab-node"}, DisplayName: "Demo compute node", State: "online"},
		},
		Capabilities: []contracts.Capability{
			{ID: "shell-main", DeviceID: "device-compute", Alias: "shell", DisplayName: "Shell", Kind: contracts.CapabilityShell, State: "available"},
			{ID: "desktop-main", DeviceID: "device-compute", Alias: "desktop", DisplayName: "Engineering desktop", Kind: contracts.CapabilityDesktop, State: "available", Features: []string{"clipboard", "dynamic-resolution"}, DesktopProfile: &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}},
			{ID: "desktop-screen", DeviceID: "device-compute", Alias: "screen", DisplayName: "Current screen", Kind: contracts.CapabilityDesktop, State: "available", Features: []string{"clipboard"}, DesktopProfile: &contracts.DesktopProfile{Protocol: "vnc", RenderingEnvironment: "physical", Authentication: "tailscale-device"}},
			{ID: "shell-local", DeviceID: "device-controller", Alias: "shell", DisplayName: "Local shell", Kind: contracts.CapabilityShell, State: "available"},
		},
	}
}

func (c Catalog) List() []contracts.Target {
	result := make([]contracts.Target, 0, len(c.Capabilities))
	if c.SubjectDeviceID != "" {
		subject, exists := c.device(c.SubjectDeviceID)
		if !exists || strings.EqualFold(subject.State, "revoked") {
			return result
		}
	}
	now := c.currentTime()
	cacheAuthorization := c.authorizationAt(now, c.CapturedAt.Add(DefaultAuthorizationTTL))
	grantedCapabilities := make(map[string]contracts.Authorization)
	for _, grant := range c.Grants {
		if grant.SubjectDeviceID == c.SubjectDeviceID && grant.State == "active" {
			validUntil := cacheAuthorization.ValidUntil
			if grant.ValidUntil != nil && grant.ValidUntil.Before(validUntil) {
				validUntil = grant.ValidUntil.UTC()
			}
			authorization := c.authorizationAt(now, validUntil)
			if authorization.Status != "active" {
				continue
			}
			if previous, exists := grantedCapabilities[grant.CapabilityID]; !exists || authorization.ValidUntil.After(previous.ValidUntil) {
				grantedCapabilities[grant.CapabilityID] = authorization
			}
		}
	}
	for _, capability := range c.Capabilities {
		authorization, granted := grantedCapabilities[capability.ID]
		if c.SubjectDeviceID == "" && cacheAuthorization.Status == "active" {
			authorization, granted = cacheAuthorization, true
		}
		if !granted {
			continue
		}
		device, ok := c.device(capability.DeviceID)
		if !ok || strings.EqualFold(device.State, "revoked") {
			continue
		}
		result = append(result, makeTarget(c.FabricID, device, capability, authorization))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Alias < result[j].Alias })
	return result
}

func (c Catalog) Authorization() contracts.Authorization {
	return c.authorizationAt(c.currentTime(), c.CapturedAt.Add(DefaultAuthorizationTTL))
}

func (c Catalog) currentTime() time.Time {
	if c.Now == nil {
		return time.Now().UTC()
	}
	return c.Now().UTC()
}

func (c Catalog) authorizationAt(now, validUntil time.Time) contracts.Authorization {
	remaining := int64(validUntil.Sub(now) / time.Second)
	status := "active"
	if !now.Before(validUntil) {
		status = "expired"
		remaining = 0
	}
	return contracts.Authorization{Status: status, ValidUntil: validUntil.UTC(), RemainingSeconds: remaining}
}

func (c Catalog) Resolve(input string) (contracts.Target, error) {
	if strings.HasPrefix(strings.TrimSpace(input), targetref.Scheme+"://") {
		r, err := targetref.Parse(input)
		if err != nil {
			return contracts.Target{}, err
		}
		if r.FabricID != c.FabricID {
			return contracts.Target{}, fmt.Errorf("fabric not found")
		}
		for _, target := range c.List() {
			if target.Device.ID == r.DeviceID && target.Capability.ID == r.CapabilityID {
				return target, nil
			}
		}
		return contracts.Target{}, fmt.Errorf("target not found")
	}

	want := strings.ToLower(strings.TrimSpace(input))
	for _, target := range c.List() {
		aliases := []string{target.Alias}
		for _, oldDevice := range target.Device.PreviousAliases {
			aliases = append(aliases, oldDevice+"/"+target.Capability.Alias)
		}
		for _, oldCapability := range target.Capability.PreviousAliases {
			aliases = append(aliases, target.Device.Alias+"/"+oldCapability)
		}
		for _, alias := range aliases {
			if strings.ToLower(alias) == want {
				return target, nil
			}
		}
	}
	return contracts.Target{}, fmt.Errorf("target not found")
}

func (c Catalog) ResolveAuthorized(subjectDeviceID, input string) (contracts.Target, error) {
	if strings.TrimSpace(subjectDeviceID) == "" || subjectDeviceID != c.SubjectDeviceID {
		return contracts.Target{}, fmt.Errorf("subject is not authorized for this catalog")
	}
	return c.Resolve(input)
}

func (c Catalog) device(id string) (contracts.Device, bool) {
	for _, device := range c.Devices {
		if device.ID == id {
			return device, true
		}
	}
	return contracts.Device{}, false
}

func makeTarget(fabricID string, device contracts.Device, capability contracts.Capability, authorization contracts.Authorization) contracts.Target {
	r := targetref.Reference{FabricID: fabricID, DeviceID: device.ID, CapabilityID: capability.ID}
	return contracts.Target{
		Canonical: r.String(), Alias: device.Alias + "/" + capability.Alias,
		Device: device, Capability: capability, Granted: true, Authorization: authorization,
	}
}
