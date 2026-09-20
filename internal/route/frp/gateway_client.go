package frp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/routelease"
)

const maxGatewayResponse = 64 << 10

type DeviceSigner interface {
	DeviceID() string
	Sign([]byte) ([]byte, error)
}

type GatewayClient struct {
	BaseURL       string
	HTTPClient    *http.Client
	Signer        DeviceSigner
	ClientVersion string
	NewRequestID  func() (string, error)
}

func (c GatewayClient) Acquire(ctx context.Context, request route.Request) (Lease, error) {
	if c.Signer == nil || request.SubjectDeviceID != c.Signer.DeviceID() || request.CanonicalTarget == "" {
		return Lease{}, errors.New("gateway route client identity mismatch")
	}
	requestID, err := c.newRequestID()
	if err != nil {
		return Lease{}, errors.New("gateway route request id unavailable")
	}
	payload := routelease.Request{SchemaVersion: routelease.SchemaVersion, SubjectDeviceID: request.SubjectDeviceID,
		CanonicalTarget: request.CanonicalTarget, RequestID: requestID, ClientVersion: c.ClientVersion}
	signature, err := c.Signer.Sign(routelease.RequestMessage(payload))
	if err != nil {
		return Lease{}, errors.New("gateway route request signing failed")
	}
	payload.Signature = base64.RawURLEncoding.EncodeToString(signature)
	var lease routelease.Lease
	if err := c.post(ctx, "/api/v1/route-leases", payload, &lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (c GatewayClient) Release(ctx context.Context, leaseID string) error {
	if c.Signer == nil {
		return errors.New("gateway route client identity unavailable")
	}
	requestID, err := c.newRequestID()
	if err != nil {
		return errors.New("gateway route release id unavailable")
	}
	payload := routelease.ReleaseRequest{SchemaVersion: routelease.SchemaVersion, SubjectDeviceID: c.Signer.DeviceID(),
		LeaseID: leaseID, RequestID: requestID, ClientVersion: c.ClientVersion}
	signature, err := c.Signer.Sign(routelease.ReleaseMessage(payload))
	if err != nil {
		return errors.New("gateway route release signing failed")
	}
	payload.Signature = base64.RawURLEncoding.EncodeToString(signature)
	var response routelease.ReleaseResponse
	return c.post(ctx, "/api/v1/route-leases/release", payload, &response)
}

func (c GatewayClient) post(ctx context.Context, path string, payload, response any) error {
	base, err := url.Parse(c.BaseURL)
	if err != nil || !secureGatewayURL(base) {
		return errors.New("gateway route endpoint is not secure")
	}
	endpoint, err := base.Parse(path)
	if err != nil {
		return errors.New("gateway route endpoint is invalid")
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return errors.New("gateway route request encoding failed")
	}
	requestBody := append([]byte(nil), content...)
	clear(content)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(requestBody))
	if err != nil {
		clear(requestBody)
		return errors.New("gateway route request unavailable")
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	httpResponse, err := client.Do(httpRequest)
	clear(requestBody)
	if err != nil {
		return errors.New("gateway route request failed")
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(httpResponse.Body, maxGatewayResponse))
		return fmt.Errorf("gateway rejected the route request with status %d", httpResponse.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(httpResponse.Body, maxGatewayResponse+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil {
		return errors.New("gateway route response is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("gateway route response contains trailing data")
	}
	return nil
}

func secureGatewayURL(value *url.URL) bool {
	if value == nil || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return false
	}
	if strings.EqualFold(value.Scheme, "https") {
		return value.Host != ""
	}
	if !strings.EqualFold(value.Scheme, "http") {
		return false
	}
	host := value.Hostname()
	address := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || address != nil && address.IsLoopback()
}

func (c GatewayClient) newRequestID() (string, error) {
	if c.NewRequestID != nil {
		return c.NewRequestID()
	}
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "request-" + hex.EncodeToString(value), nil
}

var _ LeaseClient = GatewayClient{}
