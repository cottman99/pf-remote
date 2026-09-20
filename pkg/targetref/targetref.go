// Package targetref implements PF Remote's stable target identifier.
package targetref

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const Scheme = "pfremote"

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Reference identifies one immutable capability in one fabric.
type Reference struct {
	FabricID     string `json:"fabric_id"`
	DeviceID     string `json:"device_id"`
	CapabilityID string `json:"capability_id"`
}

// Parse validates a canonical PF Remote target URI.
func Parse(raw string) (Reference, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Reference{}, fmt.Errorf("parse target: %w", err)
	}
	if u.Scheme != Scheme || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return Reference{}, fmt.Errorf("target must use %s:// and contain no query or fragment", Scheme)
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 4 || parts[0] != "devices" || parts[2] != "capabilities" {
		return Reference{}, fmt.Errorf("target path must be /devices/<id>/capabilities/<id>")
	}
	deviceID, err := url.PathUnescape(parts[1])
	if err != nil {
		return Reference{}, fmt.Errorf("decode device id: %w", err)
	}
	capabilityID, err := url.PathUnescape(parts[3])
	if err != nil {
		return Reference{}, fmt.Errorf("decode capability id: %w", err)
	}
	r := Reference{FabricID: u.Host, DeviceID: deviceID, CapabilityID: capabilityID}
	if err := r.Validate(); err != nil {
		return Reference{}, err
	}
	return r, nil
}

// Validate applies the public identifier grammar.
func (r Reference) Validate() error {
	for name, value := range map[string]string{
		"fabric id": r.FabricID, "device id": r.DeviceID, "capability id": r.CapabilityID,
	} {
		if !validID.MatchString(value) {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	return nil
}

func (r Reference) String() string {
	return fmt.Sprintf("%s://%s/devices/%s/capabilities/%s", Scheme, r.FabricID, r.DeviceID, r.CapabilityID)
}
