// Package contracts contains versioned, secret-free public response types.
package contracts

import "time"

const (
	CatalogSchema       = "pfremote.catalog/v1"
	InspectSchema       = "pfremote.inspect/v1"
	ContextSchema       = "pfremote.context/v1"
	DoctorSchema        = "pfremote.doctor/v1"
	ErrorSchema         = "pfremote.error/v1"
	ShellActionSchema   = "pfremote.shell-action/v1"
	DesktopActionSchema = "pfremote.desktop-action/v1"

	SSHCapabilityBindingSchema = "pfremote.ssh-capability-binding/v1"
)

type CapabilityKind string

const (
	CapabilityShell   CapabilityKind = "shell"
	CapabilityDesktop CapabilityKind = "desktop"
)

type Device struct {
	ID                string   `json:"id"`
	Alias             string   `json:"alias"`
	PreviousAliases   []string `json:"previous_aliases,omitempty"`
	DisplayName       string   `json:"display_name"`
	State             string   `json:"state"`
	IdentityPublicKey string   `json:"identity_public_key,omitempty"`
}

type SSHHostKey struct {
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

type SSHCapabilityBinding struct {
	SchemaVersion  string       `json:"schema_version"`
	FabricID       string       `json:"fabric_id"`
	DeviceID       string       `json:"device_id"`
	CapabilityID   string       `json:"capability_id"`
	BindingVersion uint64       `json:"binding_version"`
	HostKeys       []SSHHostKey `json:"host_keys"`
	Signature      string       `json:"signature"`
}

type ShellCapabilityClaim struct {
	DeviceID        string               `json:"device_id"`
	DevicePublicKey string               `json:"device_public_key"`
	Binding         SSHCapabilityBinding `json:"binding"`
}

type DesktopProfile struct {
	Protocol             string `json:"protocol"`
	RenderingEnvironment string `json:"rendering_environment"`
	Authentication       string `json:"authentication,omitempty"`
	VisualEffectsPolicy  string `json:"visual_effects_policy,omitempty"`
}

type Capability struct {
	ID              string                `json:"id"`
	DeviceID        string                `json:"device_id"`
	Alias           string                `json:"alias"`
	PreviousAliases []string              `json:"previous_aliases,omitempty"`
	DisplayName     string                `json:"display_name"`
	Kind            CapabilityKind        `json:"kind"`
	State           string                `json:"state"`
	Features        []string              `json:"features,omitempty"`
	SSHBinding      *SSHCapabilityBinding `json:"ssh_binding,omitempty"`
	DesktopProfile  *DesktopProfile       `json:"desktop_profile,omitempty"`
}

type Grant struct {
	ID              string     `json:"id"`
	SubjectDeviceID string     `json:"subject_device_id"`
	CapabilityID    string     `json:"capability_id"`
	State           string     `json:"state"`
	ValidUntil      *time.Time `json:"valid_until,omitempty"`
}

type Authorization struct {
	Status           string    `json:"status"`
	ValidUntil       time.Time `json:"valid_until"`
	RemainingSeconds int64     `json:"remaining_seconds"`
}

type Target struct {
	Canonical       string        `json:"canonical"`
	Alias           string        `json:"alias"`
	Device          Device        `json:"device"`
	Capability      Capability    `json:"capability"`
	Granted         bool          `json:"granted"`
	Authorization   Authorization `json:"authorization"`
	LocalSetupState string        `json:"local_setup_state,omitempty"`
	RouteOptions    []RouteOption `json:"route_options,omitempty"`
}

// RouteOption is safe, address-free connection-path metadata for one exact
// target. Adapter identifiers are stable machine values; the native UI owns
// localized user-facing labels.
type RouteOption struct {
	Adapter string `json:"adapter"`
	Status  string `json:"status"`
	Order   int    `json:"order"`
}

type CatalogResponse struct {
	SchemaVersion  string          `json:"schema_version"`
	FabricID       string          `json:"fabric_id"`
	Targets        []Target        `json:"targets"`
	Authorization  Authorization   `json:"authorization"`
	RecentSessions []RecentSession `json:"recent_sessions,omitempty"`
}

type RecentSession struct {
	SessionID       string    `json:"session_id"`
	CanonicalTarget string    `json:"canonical_target"`
	Action          string    `json:"action"`
	Status          string    `json:"status"`
	StartedAt       time.Time `json:"started_at"`
}

type InspectResponse struct {
	SchemaVersion string `json:"schema_version"`
	Target        Target `json:"target"`
}

type ContextResponse struct {
	SchemaVersion string   `json:"schema_version"`
	Target        Target   `json:"target"`
	Envelope      string   `json:"envelope"`
	Task          string   `json:"task,omitempty"`
	Constraints   []string `json:"constraints,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Code    string `json:"code,omitempty"`
}

type DoctorResponse struct {
	SchemaVersion string  `json:"schema_version"`
	Overall       string  `json:"overall"`
	Checks        []Check `json:"checks"`
}

type Error struct {
	SchemaVersion string `json:"schema_version"`
	Code          string `json:"code"`
	Stage         string `json:"stage"`
	CorrelationID string `json:"correlation_id"`
	Summary       string `json:"summary"`
	Remediation   string `json:"remediation"`
}

type ShellActionResponse struct {
	SchemaVersion string `json:"schema_version"`
	Action        string `json:"action"`
	Status        string `json:"status"`
	Target        Target `json:"target"`
	SessionID     string `json:"session_id"`
	ExitCode      int    `json:"exit_code"`
	Output        string `json:"output,omitempty"`
	ErrorOutput   string `json:"error_output,omitempty"`
}

type DesktopActionResponse struct {
	SchemaVersion        string `json:"schema_version"`
	Action               string `json:"action"`
	Status               string `json:"status"`
	Target               Target `json:"target"`
	SessionID            string `json:"session_id"`
	Protocol             string `json:"protocol"`
	RenderingEnvironment string `json:"rendering_environment"`
	RouteAdapter         string `json:"route_adapter,omitempty"`
}
