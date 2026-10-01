package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// The optional sidecar leaves the original catalog readable by older clients.
// Each workspace inherits only the exact, owner-configured RDP service's routes.
func sharedDesktopCatalog(path string, input []byte) ([]byte, error) {
	path = filepath.Join(filepath.Dir(path), "shared-desktops-v1.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return input, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > MaxInputBytes {
		return nil, errors.New("shared desktop settings are invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("shared desktop settings are unavailable")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxInputBytes+1))
	if err != nil || len(data) > MaxInputBytes {
		return nil, errors.New("shared desktop settings could not be read safely")
	}
	return mergeSharedDesktopCatalog(input, data)
}

func mergeSharedDesktopCatalog(input, data []byte) ([]byte, error) {
	var settings struct {
		Schema     string `json:"schema_version"`
		Workspaces []struct {
			ServiceID string `json:"service_id"`
			Display   int    `json:"display"`
			Name      string `json:"display_name"`
		} `json:"workspaces"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&settings) != nil || decoder.Decode(&struct{}{}) != io.EOF || settings.Schema != "pfremote.shared-desktops/v1" || len(settings.Workspaces) < 1 || len(settings.Workspaces) > 99 {
		return nil, errors.New("shared desktop settings are invalid")
	}
	var source legacyCenterCatalog
	decoder = json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&source) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("legacy Center catalog is invalid")
	}
	original := append([]legacyCenterService(nil), source.Services...)
	for _, w := range settings.Workspaces {
		if w.Display < 1 || w.Display > 99 || !validVisibleText(w.Name, 128) {
			return nil, errors.New("shared desktop workspace is invalid")
		}
		found := false
		for _, base := range original {
			if base.ID != w.ServiceID {
				continue
			}
			if base.Kind != "rdp" || base.RDPSession != "" {
				return nil, errors.New("shared desktop base must be an ordinary RDP service")
			}
			base.ID += "-shared-" + strconv.Itoa(w.Display)
			base.Name = w.Name
			base.RDPSession = "pfremote-shared-" + strconv.Itoa(w.Display)
			source.Services = append(source.Services, base)
			found = true
			break
		}
		if !found {
			return nil, errors.New("shared desktop base service is unavailable")
		}
	}
	merged, err := json.Marshal(source)
	if err != nil {
		return nil, errors.New("shared desktop settings could not be merged")
	}
	// Apply all identity, duplicate, route and bounded-size validation together.
	if _, err := LoadLegacyCenterCatalog(merged); err != nil {
		return nil, err
	}
	return merged, nil
}
