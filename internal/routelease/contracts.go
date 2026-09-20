// Package routelease defines the authenticated Gateway contract for one
// short-lived route acquisition.
package routelease

import (
	"strings"
	"time"
)

const SchemaVersion = "pfremote.route-lease/v1"

type Request struct {
	SchemaVersion   string `json:"schema_version"`
	SubjectDeviceID string `json:"subject_device_id"`
	CanonicalTarget string `json:"canonical_target"`
	RequestID       string `json:"request_id"`
	ClientVersion   string `json:"client_version"`
	Signature       string `json:"signature"`
}

type ReleaseRequest struct {
	SchemaVersion   string `json:"schema_version"`
	SubjectDeviceID string `json:"subject_device_id"`
	LeaseID         string `json:"lease_id"`
	RequestID       string `json:"request_id"`
	ClientVersion   string `json:"client_version"`
	Signature       string `json:"signature"`
}

type Lease struct {
	SchemaVersion   string    `json:"schema_version"`
	ID              string    `json:"lease_id"`
	CanonicalTarget string    `json:"canonical_target"`
	Adapter         string    `json:"adapter"`
	ServerAddress   string    `json:"server_address"`
	ServerPort      uint16    `json:"server_port"`
	ServerUser      string    `json:"server_user,omitempty"`
	ServerName      string    `json:"server_name"`
	SecretKey       string    `json:"secret_key"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type ReleaseResponse struct {
	SchemaVersion string `json:"schema_version"`
	LeaseID       string `json:"lease_id"`
	Status        string `json:"status"`
}

func RequestMessage(request Request) []byte {
	return canonicalMessage("pfremote-route-lease/v1", request.SubjectDeviceID, request.CanonicalTarget, request.RequestID, request.ClientVersion)
}

func ReleaseMessage(request ReleaseRequest) []byte {
	return canonicalMessage("pfremote-route-release/v1", request.SubjectDeviceID, request.LeaseID, request.RequestID, request.ClientVersion)
}

func canonicalMessage(domain string, fields ...string) []byte {
	return []byte(domain + "\n" + strings.Join(fields, "\n"))
}

type Fault struct {
	Code        string
	Stage       string
	Summary     string
	Remediation string
}

func (f *Fault) Error() string { return f.Summary }
