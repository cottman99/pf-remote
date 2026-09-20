package migration

import "context"

type TailscalePeerResolver interface {
	ResolvePeer(context.Context, string) (string, error)
}

type TailscalePeerVerifier interface {
	VerifyPeer(context.Context, string, string) error
}

// ResolveLegacyTailscaleIdentities enriches private in-memory routes from one
// authenticated Tailscale directory snapshot. Unavailable/offline peers remain
// setup-required; no endpoint detail enters the returned count.
func ResolveLegacyTailscaleIdentities(ctx context.Context, source *LegacyCenterCompatibility, resolver TailscalePeerResolver) int {
	if source == nil || resolver == nil {
		return 0
	}
	resolved := 0
	for serviceID, connection := range source.Connections {
		for index := range connection.Routes {
			candidate := &connection.Routes[index]
			if candidate.Adapter != "tailscale" || candidate.Address == "" {
				continue
			}
			nodeID, err := resolver.ResolvePeer(ctx, candidate.Address)
			if err != nil || nodeID == "" {
				continue
			}
			candidate.NodeID = nodeID
			resolved++
		}
		source.Connections[serviceID] = connection
	}
	return resolved
}
