package enrollment

import (
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const (
	SchemaVersion          = "pfremote.enrollment/v1"
	SupportedClientMajor   = 1
	DefaultActivationTTL   = 10 * time.Minute
	DefaultPollingInterval = 5 * time.Second
)

type OwnerInitializationRequest struct {
	SchemaVersion  string `json:"schema_version"`
	FabricID       string `json:"fabric_id"`
	OwnerDeviceID  string `json:"owner_device_id"`
	OwnerPublicKey string `json:"owner_public_key"`
	ClientVersion  string `json:"client_version"`
	Signature      string `json:"signature"`
}

type OwnerInitializationResponse struct {
	SchemaVersion    string `json:"schema_version"`
	FabricID         string `json:"fabric_id"`
	OwnerDeviceID    string `json:"owner_device_id"`
	DirectoryVersion uint64 `json:"directory_version"`
}

type DeviceAuthorizationRequest struct {
	SchemaVersion   string `json:"schema_version"`
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	DevicePublicKey string `json:"device_public_key"`
	ClientVersion   string `json:"client_version"`
	RequestID       string `json:"request_id"`
	Signature       string `json:"signature"`
}

type DeviceAuthorizationResponse struct {
	SchemaVersion           string `json:"schema_version"`
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}

type OwnerApprovalRequest struct {
	SchemaVersion string `json:"schema_version"`
	OwnerDeviceID string `json:"owner_device_id"`
	UserCode      string `json:"user_code"`
	RequestID     string `json:"request_id"`
	ClientVersion string `json:"client_version"`
	Signature     string `json:"signature"`
}

type OwnerApprovalResponse struct {
	SchemaVersion string `json:"schema_version"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	Status        string `json:"status"`
}

type DevicePollRequest struct {
	SchemaVersion string `json:"schema_version"`
	DeviceCode    string `json:"device_code"`
	ClientVersion string `json:"client_version"`
}

type DeviceActivationResponse struct {
	SchemaVersion    string `json:"schema_version"`
	FabricID         string `json:"fabric_id"`
	DeviceID         string `json:"device_id"`
	Status           string `json:"status"`
	DirectoryVersion uint64 `json:"directory_version"`
	GrantVersion     uint64 `json:"grant_version"`
}

type OwnerRevocationRequest struct {
	SchemaVersion string `json:"schema_version"`
	OwnerDeviceID string `json:"owner_device_id"`
	DeviceID      string `json:"device_id"`
	RequestID     string `json:"request_id"`
	ClientVersion string `json:"client_version"`
	Signature     string `json:"signature"`
}

type OwnerRevocationResponse struct {
	SchemaVersion    string `json:"schema_version"`
	DeviceID         string `json:"device_id"`
	Status           string `json:"status"`
	DirectoryVersion uint64 `json:"directory_version"`
	GrantVersion     uint64 `json:"grant_version"`
}

type VersionSyncRequest struct {
	SchemaVersion string `json:"schema_version"`
	DeviceID      string `json:"device_id"`
	RequestID     string `json:"request_id"`
	ClientVersion string `json:"client_version"`
	Signature     string `json:"signature"`
}

type VersionSyncResponse struct {
	SchemaVersion        string `json:"schema_version"`
	FabricID             string `json:"fabric_id"`
	DeviceID             string `json:"device_id"`
	DeviceStatus         string `json:"device_status"`
	DirectoryVersion     uint64 `json:"directory_version"`
	GrantVersion         uint64 `json:"grant_version"`
	SupportedClientMajor int    `json:"supported_client_major"`
}

type ShellCapabilityPublishRequest struct {
	SchemaVersion string                         `json:"schema_version"`
	DeviceID      string                         `json:"device_id"`
	Binding       contracts.SSHCapabilityBinding `json:"binding"`
	RequestID     string                         `json:"request_id"`
	ClientVersion string                         `json:"client_version"`
	Signature     string                         `json:"signature"`
}

type ShellCapabilityPublishResponse struct {
	SchemaVersion    string `json:"schema_version"`
	DeviceID         string `json:"device_id"`
	CapabilityID     string `json:"capability_id"`
	DirectoryVersion uint64 `json:"directory_version"`
	Changed          bool   `json:"changed"`
}

type ShellCapabilityListRequest struct {
	SchemaVersion string `json:"schema_version"`
	OwnerDeviceID string `json:"owner_device_id"`
	RequestID     string `json:"request_id"`
	ClientVersion string `json:"client_version"`
	Signature     string `json:"signature"`
}

type ShellCapabilityListResponse struct {
	SchemaVersion    string                           `json:"schema_version"`
	FabricID         string                           `json:"fabric_id"`
	DirectoryVersion uint64                           `json:"directory_version"`
	Capabilities     []contracts.ShellCapabilityClaim `json:"capabilities"`
}

type Fault struct {
	Code        string
	Stage       string
	Summary     string
	Remediation string
}

func (f *Fault) Error() string { return f.Summary }
