package migration

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/cottman99/pf-remote/internal/route"
)

// LocalRouting is private deployment state, never an exported target or lease.
// An absent file retains compatibility with existing installations.
type LocalRouting struct {
	SchemaVersion   string `json:"schema_version"`
	CatalogPath     string `json:"catalog_path,omitempty"`
	FRPConfigPath   string `json:"frp_config_path,omitempty"`
	RequiredAdapter string `json:"required_adapter,omitempty"`
}

func LoadLocalRouting(statePath string) (LocalRouting, error) {
	var value LocalRouting
	path := filepath.Join(filepath.Dir(statePath), "local-routing-v1.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return value, errors.New("private routing settings are unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return value, errors.New("private routing settings are unavailable")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16385))
	d.DisallowUnknownFields()
	if d.Decode(&value) != nil || d.Decode(&struct{}{}) != io.EOF ||
		value.SchemaVersion != "pfremote.local-routing/v1" ||
		(value.RequiredAdapter != "" && value.RequiredAdapter != "frp") {
		return LocalRouting{}, errors.New("private routing settings are invalid")
	}
	for _, p := range []string{value.CatalogPath, value.FRPConfigPath} {
		if p == "" {
			continue
		}
		i, err := os.Lstat(p)
		if !filepath.IsAbs(p) || err != nil || !i.Mode().IsRegular() {
			return LocalRouting{}, errors.New("private routing source is unavailable")
		}
	}
	if value.RequiredAdapter == "frp" && value.FRPConfigPath == "" {
		return LocalRouting{}, errors.New("required Gateway routing is not configured")
	}
	return value, nil
}

// Select never silently substitutes a direct route when Gateway is required.
func (s LocalRouting) Select(providers []route.NamedProvider, health *route.HealthTracker) route.Selector {
	if s.RequiredAdapter == "" {
		return route.Selector{Providers: providers, EvaluateAll: true, Health: health}
	}
	var selected []route.NamedProvider
	for _, p := range providers {
		if p.Adapter == s.RequiredAdapter {
			selected = append(selected, p)
		}
	}
	return route.Selector{Providers: selected}
}
