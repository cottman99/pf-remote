package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

// LegacyEndpointRoutes binds private LAN and Tailscale endpoints to canonical
// PF Remote targets. All fields are deliberately private so the binding cannot
// become an outward JSON artifact.
type LegacyEndpointRoutes struct {
	byAdapter         map[string]map[string][]legacyBoundEndpoint
	tailscaleVerifier TailscalePeerVerifier
}

type legacyBoundEndpoint struct {
	endpoint route.Endpoint
	nodeID   string
}

// BindLegacyEndpointRoutes creates a read-only, process-local route view. The
// caller supplies the reviewed service-to-target identity binding; this
// function never guesses a target from a display name.
func BindLegacyEndpointRoutes(source LegacyCenterCompatibility, canonicalByService map[string]string) (LegacyEndpointRoutes, error) {
	if len(canonicalByService) == 0 {
		return LegacyEndpointRoutes{}, errors.New("legacy route identity bindings are missing")
	}
	result := LegacyEndpointRoutes{byAdapter: map[string]map[string][]legacyBoundEndpoint{
		"tailscale": {},
		"lan":       {},
	}}
	seenTargets := make(map[string]struct{}, len(canonicalByService))
	for serviceID, canonical := range canonicalByService {
		connection, exists := source.Connections[serviceID]
		if !exists {
			return LegacyEndpointRoutes{}, errors.New("legacy route binding references an unknown service")
		}
		if _, err := targetref.Parse(canonical); err != nil {
			return LegacyEndpointRoutes{}, errors.New("legacy route binding contains an invalid target")
		}
		if _, duplicate := seenTargets[canonical]; duplicate {
			return LegacyEndpointRoutes{}, errors.New("legacy route binding contains a duplicate target")
		}
		seenTargets[canonical] = struct{}{}
		if strings.EqualFold(strings.TrimSpace(connection.Protocol), "external") {
			// The reviewed local application owns its own connection lifecycle;
			// legacy endpoint hints are neither required nor authoritative.
			continue
		}
		for _, privateRoute := range connection.Routes {
			if privateRoute.Adapter != "tailscale" && privateRoute.Adapter != "lan" {
				continue
			}
			endpoint, err := legacyEndpoint(canonical, privateRoute)
			if err != nil {
				// One stale fallback must not hide valid sibling actions. The
				// affected capability remains setup-required when no valid route
				// can make its authenticated profile available.
				continue
			}
			result.byAdapter[privateRoute.Adapter][canonical] = append(result.byAdapter[privateRoute.Adapter][canonical], legacyBoundEndpoint{endpoint: endpoint, nodeID: privateRoute.NodeID})
		}
	}
	return result, nil
}

func (r LegacyEndpointRoutes) WithTailscaleVerifier(verifier TailscalePeerVerifier) LegacyEndpointRoutes {
	r.tailscaleVerifier = verifier
	return r
}

func (r LegacyEndpointRoutes) rekeyTargets(targets map[string]string) LegacyEndpointRoutes {
	result := LegacyEndpointRoutes{byAdapter: make(map[string]map[string][]legacyBoundEndpoint, len(r.byAdapter)), tailscaleVerifier: r.tailscaleVerifier}
	for adapter, byTarget := range r.byAdapter {
		result.byAdapter[adapter] = make(map[string][]legacyBoundEndpoint, len(byTarget))
		for target, endpoints := range byTarget {
			if replacement, exists := targets[target]; exists {
				target = replacement
			}
			result.byAdapter[adapter][target] = append([]legacyBoundEndpoint(nil), endpoints...)
		}
	}
	return result
}

// Provider returns one adapter-specific provider suitable for route.Selector.
// Acquisition is read-only and does not probe, connect, or change the network.
func (r LegacyEndpointRoutes) Provider(adapter string) route.Provider {
	return legacyEndpointProvider{adapter: adapter, endpoints: r.byAdapter[adapter], tailscaleVerifier: r.tailscaleVerifier}
}

// Adapters reports only address-free route identifiers that are configured
// for the exact canonical target. It does not probe or expose endpoints.
func (r LegacyEndpointRoutes) Adapters(target string) []string {
	var result []string
	for _, adapter := range []string{"lan", "tailscale"} {
		if len(r.byAdapter[adapter][target]) > 0 {
			result = append(result, adapter)
		}
	}
	return result
}

// AvailableAdapters reports configured direct routes whose exact endpoint is
// reachable now. It keeps addresses private while allowing the native UI to
// disable stale manual choices instead of presenting them as working paths.
func (r LegacyEndpointRoutes) AvailableAdapters(ctx context.Context, target string, timeout time.Duration) map[string]bool {
	result := make(map[string]bool)
	for _, adapter := range r.Adapters(target) {
		provider := route.ReachableProvider{Provider: r.Provider(adapter), Timeout: timeout}
		acquired, err := provider.Acquire(ctx, route.Request{CanonicalTarget: target})
		if acquired != nil {
			_ = acquired.Close()
		}
		result[adapter] = err == nil
	}
	return result
}

func legacyEndpoint(canonical string, privateRoute LegacyCenterRoute) (route.Endpoint, error) {
	if strings.TrimSpace(privateRoute.Address) == "" || privateRoute.Port < 1 || privateRoute.Port > 65535 {
		return route.Endpoint{}, errors.New("legacy endpoint route is invalid")
	}
	digest := sha256.Sum256([]byte(privateRoute.Adapter + "\x00" + canonical + "\x00" + privateRoute.Address + "\x00" + strconv.Itoa(privateRoute.Port)))
	endpoint := route.Endpoint{
		ID:      "legacy-route-" + hex.EncodeToString(digest[:8]),
		Address: privateRoute.Address,
		Port:    uint16(privateRoute.Port),
	}
	candidate := route.Candidate{ID: endpoint.ID, Adapter: privateRoute.Adapter, Network: "tcp", Address: endpoint.Address, Port: endpoint.Port}
	if err := candidate.Validate(); err != nil {
		return route.Endpoint{}, errors.New("legacy endpoint route is invalid")
	}
	return endpoint, nil
}

type legacyEndpointProvider struct {
	adapter           string
	endpoints         map[string][]legacyBoundEndpoint
	tailscaleVerifier TailscalePeerVerifier
}

func (p legacyEndpointProvider) Acquire(ctx context.Context, request route.Request) (route.Acquisition, error) {
	if p.adapter != "tailscale" && p.adapter != "lan" {
		return nil, errors.New("legacy endpoint provider is not configured")
	}
	endpoints := p.endpoints[request.CanonicalTarget]
	if len(endpoints) == 0 {
		return nil, errors.New("legacy endpoint is unavailable")
	}
	selected := endpoints[0]
	// A peer offline when the private catalog was cached has no node ID yet.
	// Resolve only that missing identity from the authenticated directory at
	// acquisition time. Never replace an already pinned node ID after a mismatch.
	if p.adapter == "tailscale" && selected.nodeID == "" {
		if resolver, ok := p.tailscaleVerifier.(TailscalePeerResolver); ok {
			selected.nodeID, _ = resolver.ResolvePeer(ctx, selected.endpoint.Address)
		}
	}
	if p.adapter == "tailscale" && (selected.nodeID == "" || p.tailscaleVerifier == nil || p.tailscaleVerifier.VerifyPeer(ctx, selected.endpoint.Address, selected.nodeID) != nil) {
		return nil, errors.New("legacy Tailscale endpoint identity is unavailable")
	}
	candidate := route.Candidate{
		ID: selected.endpoint.ID, Adapter: p.adapter, Network: "tcp",
		Address: selected.endpoint.Address, Port: selected.endpoint.Port,
	}
	return legacyEndpointAcquisition{candidate: candidate}, nil
}

type legacyEndpointAcquisition struct{ candidate route.Candidate }

func (a legacyEndpointAcquisition) Candidate() route.Candidate { return a.candidate }
func (legacyEndpointAcquisition) Close() error                 { return nil }
