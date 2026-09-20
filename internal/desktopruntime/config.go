// Package desktopruntime loads daemon-owned Desktop route and trust settings.
package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

const SchemaVersion = "pfremote.desktop-runtime/v1"

type File struct {
	SchemaVersion string  `json:"schema_version"`
	Targets       []Entry `json:"targets"`
}

type Entry struct {
	CanonicalTarget string `json:"canonical_target"`
	RouteID         string `json:"route_id"`
	Adapter         string `json:"adapter"`
	Address         string `json:"address"`
	Port            uint16 `json:"port"`
	VNCTrustFile    string `json:"vnc_trust_file,omitempty"`
	TailscaleNodeID string `json:"tailscale_node_id,omitempty"`
}

type Config struct {
	entries           map[string]Entry
	tailscaleVerifier TailscaleVerifier
}

type TailscaleVerifier interface {
	VerifyPeer(context.Context, string, string) error
}

func PathForState(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "desktop-runtime-v1.json")
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var value File
	if err := decoder.Decode(&value); err != nil {
		return Config{}, errors.New("decode Desktop runtime configuration")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Config{}, errors.New("Desktop runtime configuration has trailing content")
	}
	if value.SchemaVersion != SchemaVersion || len(value.Targets) == 0 || len(value.Targets) > 256 {
		return Config{}, errors.New("Desktop runtime configuration metadata is invalid")
	}
	config := Config{entries: make(map[string]Entry, len(value.Targets))}
	for _, entry := range value.Targets {
		if _, err := targetref.Parse(entry.CanonicalTarget); err != nil {
			return Config{}, errors.New("Desktop runtime configuration contains an invalid target")
		}
		candidate := route.Candidate{ID: entry.RouteID, Adapter: entry.Adapter, Network: "tcp", Address: entry.Address, Port: entry.Port}
		if (entry.Adapter != "lan" && entry.Adapter != "tailscale") || candidate.Validate() != nil ||
			(entry.Adapter == "tailscale" && entry.TailscaleNodeID == "") {
			return Config{}, errors.New("Desktop runtime configuration contains an invalid route")
		}
		if entry.VNCTrustFile != "" {
			absolute, err := filepath.Abs(entry.VNCTrustFile)
			if err != nil || !filepath.IsAbs(absolute) {
				return Config{}, errors.New("Desktop runtime configuration contains invalid trust material")
			}
			entry.VNCTrustFile = absolute
		}
		if _, exists := config.entries[entry.CanonicalTarget]; exists {
			return Config{}, errors.New("Desktop runtime configuration contains a duplicate target")
		}
		config.entries[entry.CanonicalTarget] = entry
	}
	return config, nil
}

func (c Config) WithTailscaleVerifier(verifier TailscaleVerifier) Config {
	c.tailscaleVerifier = verifier
	return c
}

func (c Config) Adapters(target string) []string {
	entry, exists := c.entries[target]
	if !exists {
		return nil
	}
	return []string{entry.Adapter}
}

func (c Config) HasTarget(target string) bool {
	_, exists := c.entries[target]
	return exists
}

func (c Config) Provider(adapter string) route.Provider {
	return adapterProvider{config: c, adapter: adapter}
}

func (c Config) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	entry, exists := c.entries[request.CanonicalTarget]
	if !exists {
		return nil, errors.New("Desktop route is unavailable")
	}
	if entry.Adapter == "tailscale" {
		if c.tailscaleVerifier == nil || c.tailscaleVerifier.VerifyPeer(ctx, entry.Address, entry.TailscaleNodeID) != nil {
			return nil, errors.New("Tailscale Desktop identity is unavailable")
		}
	}
	return acquisition{candidate: route.Candidate{ID: entry.RouteID, Adapter: entry.Adapter, Network: "tcp", Address: entry.Address, Port: entry.Port}}, nil
}

func (c Config) VNCTrustFile(canonicalTarget string) (string, error) {
	entry, exists := c.entries[canonicalTarget]
	if !exists || entry.VNCTrustFile == "" {
		return "", fmt.Errorf("VNC trust is unavailable")
	}
	return entry.VNCTrustFile, nil
}

type acquisition struct {
	candidate route.Candidate
}

func (a acquisition) Candidate() route.Candidate { return a.candidate }
func (acquisition) Close() error                 { return nil }

var _ route.Provider = Config{}

type adapterProvider struct {
	config  Config
	adapter string
}

func (p adapterProvider) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	entry, exists := p.config.entries[request.CanonicalTarget]
	if !exists || entry.Adapter != p.adapter {
		return nil, errors.New("Desktop route adapter is unavailable")
	}
	return p.config.Acquire(ctx, request)
}
