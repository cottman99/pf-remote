package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	maxGatewayRequestSize  = 1 << 20
	maxGatewayResponseSize = 1 << 20
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// Health verifies that the configured Gateway is accepting trusted HTTPS/HTTP
// requests without performing an authorization or capability mutation.
func (c Client) Health(ctx context.Context) error {
	base, err := ParseGatewayURL(c.BaseURL)
	if err != nil {
		return err
	}
	endpoint := base.ResolveReference(&url.URL{Path: "/healthz"})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return errors.New("Gateway health request could not be created")
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("Gateway is unavailable")
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxGatewayResponseSize+1))
	if err != nil || len(payload) > maxGatewayResponseSize || response.StatusCode != http.StatusOK {
		return errors.New("Gateway health response is unavailable")
	}
	var health struct {
		Status string `json:"status"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&health); err != nil || decoder.Decode(&struct{}{}) != io.EOF || health.Status != "ok" {
		return errors.New("Gateway health response is invalid")
	}
	return nil
}

func (c Client) InitializeOwner(ctx context.Context, request OwnerInitializationRequest) (OwnerInitializationResponse, error) {
	var response OwnerInitializationResponse
	if err := c.post(ctx, "/api/v1/owner/initialize", request, &response); err != nil {
		return OwnerInitializationResponse{}, err
	}
	return response, nil
}

func (c Client) RequestDeviceAuthorization(ctx context.Context, request DeviceAuthorizationRequest) (DeviceAuthorizationResponse, error) {
	var response DeviceAuthorizationResponse
	if err := c.post(ctx, "/api/v1/device-authorizations", request, &response); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	return response, nil
}

func (c Client) ApproveDevice(ctx context.Context, request OwnerApprovalRequest) (OwnerApprovalResponse, error) {
	var response OwnerApprovalResponse
	if err := c.post(ctx, "/api/v1/device-authorizations/approve", request, &response); err != nil {
		return OwnerApprovalResponse{}, err
	}
	return response, nil
}

func (c Client) PollDevice(ctx context.Context, request DevicePollRequest) (DeviceActivationResponse, error) {
	var response DeviceActivationResponse
	if err := c.post(ctx, "/api/v1/device-authorizations/poll", request, &response); err != nil {
		return DeviceActivationResponse{}, err
	}
	return response, nil
}

func (c Client) SyncVersions(ctx context.Context, request VersionSyncRequest) (VersionSyncResponse, error) {
	var response VersionSyncResponse
	if err := c.post(ctx, "/api/v1/versions/sync", request, &response); err != nil {
		return VersionSyncResponse{}, err
	}
	return response, nil
}

func (c Client) PublishShellCapability(ctx context.Context, request ShellCapabilityPublishRequest) (ShellCapabilityPublishResponse, error) {
	var response ShellCapabilityPublishResponse
	if err := c.post(ctx, "/api/v1/capabilities/shell/publish", request, &response); err != nil {
		return ShellCapabilityPublishResponse{}, err
	}
	return response, nil
}

func (c Client) ListShellCapabilities(ctx context.Context, request ShellCapabilityListRequest) (ShellCapabilityListResponse, error) {
	var response ShellCapabilityListResponse
	if err := c.post(ctx, "/api/v1/capabilities/shell/list", request, &response); err != nil {
		return ShellCapabilityListResponse{}, err
	}
	return response, nil
}

func (c Client) post(ctx context.Context, path string, request, response any) error {
	base, err := ParseGatewayURL(c.BaseURL)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > maxGatewayRequestSize {
		return errors.New("Gateway request is invalid")
	}
	endpoint := base.ResolveReference(&url.URL{Path: path})
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return errors.New("Gateway request could not be created")
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		return errors.New("Gateway is unavailable")
	}
	defer httpResponse.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxGatewayResponseSize+1))
	if err != nil || len(payload) > maxGatewayResponseSize {
		return errors.New("Gateway response could not be read safely")
	}
	if httpResponse.StatusCode != http.StatusOK {
		return errors.New("Gateway rejected the capability synchronization")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("Gateway response is invalid")
	}
	return nil
}

func ValidateGatewayURL(value string) error {
	_, err := ParseGatewayURL(value)
	return err
}

func ParseGatewayURL(value string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(value))
	if err != nil || base.Host == "" || (base.Scheme != "https" && !loopbackHTTP(base)) || base.RawQuery != "" || base.Fragment != "" || base.User != nil {
		return nil, errors.New("Gateway endpoint is invalid or insecure")
	}
	return base, nil
}

func loopbackHTTP(endpoint *url.URL) bool {
	if endpoint.Scheme != "http" {
		return false
	}
	host := endpoint.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
