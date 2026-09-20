package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

// legacyCenterCatalog mirrors the observed stable shape needed by the local
// compatibility adapter. Infrastructure-bearing values remain inside this
// private process boundary and never enter exported inventory or context.
type legacyCenterCatalog struct {
	DeviceID     string                `json:"deviceId"`
	DefaultRoute string                `json:"defaultRoute"`
	GeneratedAt  int64                 `json:"generatedAt"`
	Services     []legacyCenterService `json:"services"`
}

type legacyCenterService struct {
	ID         string           `json:"id"`
	DeviceID   string           `json:"deviceId"`
	DeviceName string           `json:"deviceName"`
	Name       string           `json:"name"`
	Kind       string           `json:"kind"`
	Username   *string          `json:"username"`
	Aliyun     *legacyAliyun    `json:"aliyun"`
	Tailscale  *legacyTailscale `json:"tailscale"`
	WebURL     *string          `json:"webUrl"`
	External   json.RawMessage  `json:"external"`
	LAN        *legacyLAN       `json:"lan"`
}

type legacyAliyun struct {
	ServerAddress string `json:"serverAddress"`
	ServerPort    int    `json:"serverPort"`
	ProxyName     string `json:"proxyName"`
	ProxySecret   string `json:"proxySecret"`
	GatewayPort   int    `json:"gatewayPort"`
}

type legacyTailscale struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type legacyLAN struct {
	Hosts []string `json:"hosts"`
	Port  int      `json:"port"`
}

// LegacyCenterCompatibility keeps the full private connection material only
// inside the local migration/runtime boundary. Connections are deliberately
// excluded from JSON so UI, Agent, reports, and exported inventories cannot
// serialize them by accident.
type LegacyCenterCompatibility struct {
	Inventory   Inventory                         `json:"inventory"`
	Connections map[string]LegacyCenterConnection `json:"-"`
}

type LegacyCenterConnection struct {
	Protocol string              `json:"-"`
	Username string              `json:"-"`
	Routes   []LegacyCenterRoute `json:"-"`
}

type LegacyCenterRoute struct {
	Adapter     string `json:"-"`
	Address     string `json:"-"`
	Port        int    `json:"-"`
	ControlPort int    `json:"-"`
	ProxyName   string `json:"-"`
	ProxySecret string `json:"-"`
	URL         string `json:"-"`
	PrivateJSON string `json:"-"`
	NodeID      string `json:"-"`
}

// ExportLegacyCenterCatalog converts one explicitly selected legacy Center
// catalog to the secret-free migration inventory. It never returns route,
// address, port, username, URL, or credential values.
func ExportLegacyCenterCatalog(input []byte) (Inventory, error) {
	compatibility, err := LoadLegacyCenterCatalog(input)
	if err != nil {
		return Inventory{}, err
	}
	return compatibility.Inventory, nil
}

// LoadLegacyCenterCatalog reads the complete private source for local use. It
// retains connection material in memory while producing the separately safe
// inventory used by UI and Agent-facing flows.
func LoadLegacyCenterCatalog(input []byte) (LegacyCenterCompatibility, error) {
	if len(input) == 0 || len(input) > MaxInputBytes {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog size is invalid")
	}
	var source legacyCenterCatalog
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog is invalid")
	}
	if len(source.Services) == 0 || len(source.Services) > 8192 {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog metadata is invalid")
	}

	services := append([]legacyCenterService(nil), source.Services...)
	sort.Slice(services, func(left, right int) bool {
		if services[left].DeviceID == services[right].DeviceID {
			return services[left].ID < services[right].ID
		}
		return services[left].DeviceID < services[right].DeviceID
	})
	inventory := Inventory{SchemaVersion: InventorySchema}
	connections := make(map[string]LegacyCenterConnection, len(services))
	deviceIndex := make(map[string]int)
	serviceIDs := make(map[string]struct{}, len(services))
	for _, service := range services {
		if !validOpaqueRef(service.ID) || !validOpaqueRef(service.DeviceID) || !validVisibleText(service.DeviceName, 128) || !validVisibleText(service.Name, 128) {
			return LegacyCenterCompatibility{}, errors.New("legacy Center catalog contains invalid visible identity data")
		}
		if _, exists := serviceIDs[service.ID]; exists {
			return LegacyCenterCompatibility{}, errors.New("legacy Center catalog contains a duplicate service")
		}
		serviceIDs[service.ID] = struct{}{}
		kind, err := legacyCapabilityKind(service.Kind)
		if err != nil {
			return LegacyCenterCompatibility{}, err
		}
		routes := legacyCenterRoutes(service)
		paths := len(routes)
		if paths == 0 {
			return LegacyCenterCompatibility{}, errors.New("legacy Center catalog contains a service without an observable path")
		}
		username := ""
		if service.Username != nil {
			username = *service.Username
		}
		connections[service.ID] = LegacyCenterConnection{Protocol: service.Kind, Username: username, Routes: routes}

		index, exists := deviceIndex[service.DeviceID]
		if !exists {
			index = len(inventory.Devices)
			deviceIndex[service.DeviceID] = index
			inventory.Devices = append(inventory.Devices, LegacyDevice{
				LegacyRef: service.DeviceID, Name: service.DeviceName, DisplayName: service.DeviceName,
			})
		} else if inventory.Devices[index].DisplayName != service.DeviceName {
			return LegacyCenterCompatibility{}, errors.New("legacy Center catalog gives one computer conflicting names")
		}
		inventory.Devices[index].Capabilities = append(inventory.Devices[index].Capabilities, LegacyCapability{
			LegacyRef: service.ID, Name: service.Name, DisplayName: service.Name,
			Kind: kind, PathCount: paths,
		})
	}
	if err := validateInventory(inventory); err != nil {
		return LegacyCenterCompatibility{}, err
	}
	return LegacyCenterCompatibility{Inventory: inventory, Connections: connections}, nil
}

func legacyCapabilityKind(value string) (contracts.CapabilityKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ssh":
		return contracts.CapabilityShell, nil
	case "rdp", "vnc":
		return contracts.CapabilityDesktop, nil
	case "external":
		return contracts.CapabilityDesktop, nil
	default:
		return "", errors.New("legacy Center catalog contains an unsupported capability kind")
	}
}

func legacyCenterRoutes(service legacyCenterService) []LegacyCenterRoute {
	var routes []LegacyCenterRoute
	if service.Aliyun != nil {
		routes = append(routes, LegacyCenterRoute{
			Adapter: "legacy-gateway", Address: service.Aliyun.ServerAddress,
			Port: service.Aliyun.GatewayPort, ControlPort: service.Aliyun.ServerPort,
			ProxyName: service.Aliyun.ProxyName, ProxySecret: service.Aliyun.ProxySecret,
		})
	}
	if service.Tailscale != nil {
		routes = append(routes, LegacyCenterRoute{Adapter: "tailscale", Address: service.Tailscale.Host, Port: service.Tailscale.Port})
	}
	if service.WebURL != nil && strings.TrimSpace(*service.WebURL) != "" {
		routes = append(routes, LegacyCenterRoute{Adapter: "legacy-web", URL: *service.WebURL})
	}
	if trimmed := strings.TrimSpace(string(service.External)); trimmed != "" && trimmed != "null" {
		routes = append(routes, LegacyCenterRoute{Adapter: "legacy-external", PrivateJSON: trimmed})
	}
	if service.LAN != nil {
		for _, host := range service.LAN.Hosts {
			routes = append(routes, LegacyCenterRoute{Adapter: "lan", Address: host, Port: service.LAN.Port})
		}
	}
	return routes
}
