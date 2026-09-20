package capabilitysync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/cottman99/pf-remote/internal/enrollment"
)

// LoadClaimsCache reads a bounded private cache of Device-signed Shell claims.
// The signatures are verified against the active catalog when the claims are
// applied; this loader only enforces the versioned transport envelope.
func LoadClaimsCache(path string) (enrollment.ShellCapabilityListResponse, error) {
	payload, err := readBoundedRegularFile(path)
	if err != nil {
		return enrollment.ShellCapabilityListResponse{}, err
	}
	var response enrollment.ShellCapabilityListResponse
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil || decoder.Decode(&struct{}{}) != io.EOF || response.SchemaVersion != enrollment.SchemaVersion || response.FabricID == "" || response.DirectoryVersion == 0 || len(response.Capabilities) == 0 {
		return enrollment.ShellCapabilityListResponse{}, errors.New("Shell capability cache is invalid")
	}
	return response, nil
}
