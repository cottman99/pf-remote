package migration

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	frproute "github.com/cottman99/pf-remote/internal/route/frp"
)

var legacyGatewayOpaque = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)

// LegacyGatewayRoutes adapts the existing read-only Alibaba Cloud FRP service
// definitions into process-scoped visitors. The source proxy remains owned by
// the untouched legacy installation; PF Remote only owns and cleans up its
// temporary local visitor.
type LegacyGatewayRoutes struct {
	byTarget map[string]LegacyCenterRoute
}

func BindLegacyGatewayRoutes(source LegacyCenterCompatibility, canonicalByService map[string]string) LegacyGatewayRoutes {
	result := LegacyGatewayRoutes{byTarget: make(map[string]LegacyCenterRoute)}
	for serviceID, canonical := range canonicalByService {
		connection, exists := source.Connections[serviceID]
		if !exists {
			continue
		}
		for _, candidate := range connection.Routes {
			if validLegacyGatewayRoute(candidate) {
				result.byTarget[canonical] = candidate
				break
			}
		}
	}
	return result
}

func (r LegacyGatewayRoutes) HasTarget(target string) bool {
	_, exists := r.byTarget[target]
	return exists
}

func (r LegacyGatewayRoutes) rekeyTargets(targets map[string]string) LegacyGatewayRoutes {
	result := LegacyGatewayRoutes{byTarget: make(map[string]LegacyCenterRoute, len(r.byTarget))}
	for target, definition := range r.byTarget {
		if replacement, exists := targets[target]; exists {
			target = replacement
		}
		result.byTarget[target] = definition
	}
	return result
}

func (r LegacyGatewayRoutes) Provider(binary, stateDir string) route.Provider {
	return frproute.Adapter{
		Binary:   binary,
		StateDir: filepath.Join(stateDir, "legacy-gateway-visitors"),
		Client:   legacyGatewayLeaseClient{routes: r.byTarget},
	}
}

// ManagedProvider reuses the already-running, read-only legacy FRP visitors.
// It resolves an exact target by the server name and secret already present in
// the legacy catalog, then exposes only the matching loopback endpoint to the
// new Session. It never starts, stops, or edits the legacy service.
func (r LegacyGatewayRoutes) ManagedProvider(configPath string) route.Provider {
	return legacyManagedGatewayProvider{routes: r.byTarget, configPath: configPath}
}

type legacyManagedGatewayProvider struct {
	routes     map[string]LegacyCenterRoute
	configPath string
}

type legacyManagedGatewayAcquisition struct{ candidate route.Candidate }

func (a legacyManagedGatewayAcquisition) Candidate() route.Candidate { return a.candidate }
func (legacyManagedGatewayAcquisition) Close() error                 { return nil }

func (p legacyManagedGatewayProvider) Acquire(_ context.Context, request route.Request) (route.Acquisition, error) {
	definition, exists := p.routes[request.CanonicalTarget]
	if !exists || !validLegacyGatewayRoute(definition) {
		return nil, errors.New("legacy Gateway route is unavailable")
	}
	visitors, err := loadLegacyManagedVisitors(p.configPath)
	if err != nil {
		return nil, err
	}
	var matched []legacyManagedVisitor
	for _, visitor := range visitors {
		if visitor.ServerName == definition.ProxyName && visitor.SecretKey == definition.ProxySecret {
			matched = append(matched, visitor)
		}
	}
	if len(matched) != 1 {
		return nil, errors.New("legacy Gateway visitor identity is unavailable")
	}
	candidate := route.Candidate{
		ID: "legacy-managed-gateway", Adapter: "frp", Network: "tcp",
		Address: matched[0].BindAddr, Port: matched[0].BindPort,
	}
	if err := candidate.Validate(); err != nil {
		return nil, errors.New("legacy Gateway visitor endpoint is invalid")
	}
	return legacyManagedGatewayAcquisition{candidate: candidate}, nil
}

type legacyManagedVisitor struct {
	BindAddr   string
	BindPort   uint16
	ServerName string
	SecretKey  string
}

func loadLegacyManagedVisitors(path string) ([]legacyManagedVisitor, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1024*1024 {
		return nil, errors.New("legacy Gateway visitor configuration is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("legacy Gateway visitor configuration could not be inspected")
	}
	defer file.Close()

	var visitors []legacyManagedVisitor
	var current *legacyManagedVisitor
	flush := func() {
		if current != nil {
			visitors = append(visitors, *current)
			current = nil
		}
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[[visitors]]" {
			flush()
			current = &legacyManagedVisitor{}
			continue
		}
		if strings.HasPrefix(line, "[[") {
			flush()
			continue
		}
		if current == nil || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "bindAddr":
			current.BindAddr = unquoteLegacyTOML(value)
		case "bindPort":
			port, parseErr := strconv.ParseUint(value, 10, 16)
			if parseErr != nil {
				return nil, errors.New("legacy Gateway visitor configuration is invalid")
			}
			current.BindPort = uint16(port)
		case "serverName":
			current.ServerName = unquoteLegacyTOML(value)
		case "secretKey":
			current.SecretKey = unquoteLegacyTOML(value)
		}
	}
	flush()
	if err := scanner.Err(); err != nil || len(visitors) == 0 {
		return nil, errors.New("legacy Gateway visitor configuration is invalid")
	}
	for index := range visitors {
		if visitors[index].BindAddr == "" {
			visitors[index].BindAddr = "127.0.0.1"
		}
		if visitors[index].BindAddr != "127.0.0.1" && visitors[index].BindAddr != "::1" {
			return nil, fmt.Errorf("legacy Gateway visitor %d is not loopback-bound", index+1)
		}
		if visitors[index].BindPort == 0 || !legacyGatewayOpaque.MatchString(visitors[index].ServerName) || !legacyGatewayOpaque.MatchString(visitors[index].SecretKey) {
			return nil, errors.New("legacy Gateway visitor configuration is incomplete")
		}
	}
	return visitors, nil
}

func unquoteLegacyTOML(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			return unquoted
		}
	}
	return ""
}

type legacyGatewayLeaseClient struct {
	routes map[string]LegacyCenterRoute
}

func (c legacyGatewayLeaseClient) Acquire(_ context.Context, request route.Request) (frproute.Lease, error) {
	definition, exists := c.routes[request.CanonicalTarget]
	if !exists || !validLegacyGatewayRoute(definition) {
		return frproute.Lease{}, errors.New("legacy Gateway route is unavailable")
	}
	identifier := make([]byte, 12)
	if _, err := rand.Read(identifier); err != nil {
		return frproute.Lease{}, errors.New("legacy Gateway route identity is unavailable")
	}
	expires := time.Now().UTC().Add(2 * time.Minute)
	if request.AuthorizationExpiry.Before(expires) {
		expires = request.AuthorizationExpiry.UTC()
	}
	return frproute.Lease{
		SchemaVersion:   frproute.LeaseSchema,
		ID:              "legacy-" + hex.EncodeToString(identifier),
		CanonicalTarget: request.CanonicalTarget,
		Adapter:         "frp",
		ServerAddress:   definition.Address,
		ServerPort:      uint16(definition.ControlPort),
		ServerName:      definition.ProxyName,
		SecretKey:       definition.ProxySecret,
		ExpiresAt:       expires,
	}, nil
}

func (legacyGatewayLeaseClient) Release(context.Context, string) error { return nil }

func validLegacyGatewayRoute(candidate LegacyCenterRoute) bool {
	if candidate.Adapter != "legacy-gateway" || candidate.ControlPort < 1 || candidate.ControlPort > 65535 ||
		!legacyGatewayOpaque.MatchString(candidate.ProxyName) || !legacyGatewayOpaque.MatchString(candidate.ProxySecret) {
		return false
	}
	return (route.Candidate{ID: "legacy-gateway", Adapter: "frp", Network: "tcp", Address: candidate.Address, Port: uint16(candidate.ControlPort)}).Validate() == nil
}
