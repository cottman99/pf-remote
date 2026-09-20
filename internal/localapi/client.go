package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

type Client struct {
	dial func(context.Context) (net.Conn, error)
}

type RemoteError struct {
	Failure contracts.Error
}

func (e *RemoteError) Error() string { return e.Failure.Summary }

func NewClient() Client { return Client{dial: dial} }

func (c Client) Updates(ctx context.Context, action string) (json.RawMessage, error) {
	var response json.RawMessage
	err := c.call(ctx, Request{Action: action}, &response)
	return response, err
}

func (c Client) List(ctx context.Context) (contracts.CatalogResponse, error) {
	var response contracts.CatalogResponse
	err := c.call(ctx, Request{Action: "list"}, &response)
	return response, err
}

func (c Client) Inspect(ctx context.Context, target string) (contracts.InspectResponse, error) {
	var response contracts.InspectResponse
	err := c.call(ctx, Request{Action: "inspect", Target: target}, &response)
	return response, err
}

func (c Client) Context(ctx context.Context, target, task string, constraints []string) (contracts.ContextResponse, error) {
	var response contracts.ContextResponse
	err := c.call(ctx, Request{Action: "context", Target: target, Task: task, Constraints: constraints}, &response)
	return response, err
}

func (c Client) Doctor(ctx context.Context) (contracts.DoctorResponse, error) {
	var response contracts.DoctorResponse
	err := c.call(ctx, Request{Action: "doctor"}, &response)
	return response, err
}

func (c Client) Connect(ctx context.Context, target string) (contracts.ShellActionResponse, error) {
	var response contracts.ShellActionResponse
	err := c.call(ctx, Request{Action: "connect", Target: target}, &response)
	return response, err
}

func (c Client) Exec(ctx context.Context, target string, command []string) (contracts.ShellActionResponse, error) {
	var response contracts.ShellActionResponse
	err := c.call(ctx, Request{Action: "exec", Target: target, Command: append([]string(nil), command...)}, &response)
	return response, err
}

func (c Client) Open(ctx context.Context, target string) (contracts.DesktopActionResponse, error) {
	return c.OpenVia(ctx, target, "")
}

func (c Client) OpenVia(ctx context.Context, target, routeAdapter string) (contracts.DesktopActionResponse, error) {
	var response contracts.DesktopActionResponse
	err := c.call(ctx, Request{Action: "open", Target: target, RouteAdapter: routeAdapter}, &response)
	return response, err
}

func (c Client) SaveDesktopCredentialAndOpen(ctx context.Context, target, credential string) (contracts.DesktopActionResponse, error) {
	return c.SaveDesktopCredentialAndOpenVia(ctx, target, credential, "")
}

func (c Client) SaveDesktopCredentialAndOpenVia(ctx context.Context, target, credential, routeAdapter string) (contracts.DesktopActionResponse, error) {
	var response contracts.DesktopActionResponse
	err := c.call(ctx, Request{Action: "save-desktop-credential-and-open", Target: target, Credential: credential, RouteAdapter: routeAdapter}, &response)
	return response, err
}

func (c Client) call(ctx context.Context, request Request, destination any) error {
	if c.dial == nil {
		return errors.New("local API dialer is unavailable")
	}
	connection, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("connect to PF Remote daemon: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			return fmt.Errorf("set local API deadline: %w", err)
		}
	}
	request.SchemaVersion = SchemaVersion
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return fmt.Errorf("send local API request: %w", err)
	}
	var response struct {
		SchemaVersion string           `json:"schema_version"`
		Result        json.RawMessage  `json:"result,omitempty"`
		Error         *contracts.Error `json:"error,omitempty"`
	}
	decoder := json.NewDecoder(connection)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("read local API response: %w", err)
	}
	if response.SchemaVersion != SchemaVersion {
		return errors.New("local API returned an unsupported schema")
	}
	if response.Error != nil {
		return &RemoteError{Failure: *response.Error}
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return errors.New("local API returned an empty result")
	}
	if err := json.Unmarshal(response.Result, destination); err != nil {
		return fmt.Errorf("decode local API result: %w", err)
	}
	return nil
}
