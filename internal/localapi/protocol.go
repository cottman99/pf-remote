package localapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const SchemaVersion = "pfremote.local-api/v1"

type Request struct {
	SchemaVersion string   `json:"schema_version"`
	Action        string   `json:"action"`
	Target        string   `json:"target,omitempty"`
	Task          string   `json:"task,omitempty"`
	Constraints   []string `json:"constraints,omitempty"`
	Command       []string `json:"command,omitempty"`
	Credential    string   `json:"credential,omitempty"`
	RouteAdapter  string   `json:"route_adapter,omitempty"`
}

type Response struct {
	SchemaVersion string           `json:"schema_version"`
	Result        any              `json:"result,omitempty"`
	Error         *contracts.Error `json:"error,omitempty"`
}

type ServiceProvider func() (actions.Service, error)

type Handler struct {
	Service  actions.Service
	Provider ServiceProvider
	Gate     *ActionGate
	Updates  func(context.Context, string) (any, error)
}

func NewHandler() Handler {
	return Handler{Provider: func() (actions.Service, error) {
		return actions.Service{}, errors.New("local action service is not configured")
	}}
}

func (h Handler) Handle(request Request) Response {
	return h.HandleContext(context.Background(), request)
}

func (h Handler) HandleContext(ctx context.Context, request Request) Response {
	if request.SchemaVersion != SchemaVersion {
		return failed("UNSUPPORTED_VERSION", "local-api", "Unsupported local API schema version.", "Update PF Remote components together.")
	}
	if request.Action == "update-status" || request.Action == "check-updates" || request.Action == "notify-updates" {
		if h.Updates == nil {
			return failed("UPDATE_UNAVAILABLE", "updates", "Update controls are unavailable.", "Update this client first.")
		}
		result, err := h.Updates(ctx, request.Action)
		if err != nil {
			return failed("UPDATE_UNAVAILABLE", "updates", "The connection service could not receive the notification.", "Check the connection service and retry.")
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	}
	if h.Gate != nil && (request.Action == "connect" || request.Action == "exec" || request.Action == "open" || request.Action == "save-desktop-credential-and-open") {
		leave, ok := h.Gate.Enter()
		if !ok {
			return failed("UPDATE_IN_PROGRESS", "maintenance", "PF Remote is updating. Try again shortly.", "Wait for the update to finish.")
		}
		defer leave()
	}
	service := h.Service
	if h.Provider != nil {
		var err error
		service, err = h.Provider()
		if err != nil {
			return failed("STATE_UNAVAILABLE", "state", "PF Remote could not load a valid local authorization snapshot.", "Repair local state or refresh it from the Gateway, then retry.")
		}
	}
	switch request.Action {
	case "list":
		return Response{SchemaVersion: SchemaVersion, Result: service.List()}
	case "inspect":
		result, err := service.Inspect(request.Target)
		if err != nil {
			return failed("TARGET_NOT_FOUND", "resolve", err.Error(), "Refresh the catalog or choose a visible target.")
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	case "context":
		result, err := service.Context(request.Target, request.Task, request.Constraints)
		if err != nil {
			return failed("TARGET_NOT_FOUND", "resolve", err.Error(), "Refresh the catalog or choose a visible target.")
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	case "doctor":
		return Response{SchemaVersion: SchemaVersion, Result: service.Doctor()}
	case "connect":
		result, err := service.Connect(ctx, request.Target)
		if err != nil {
			return actionFailure(err)
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	case "exec":
		result, err := service.Exec(ctx, request.Target, request.Command)
		if err != nil {
			return actionFailure(err)
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	case "open":
		result, err := service.OpenVia(ctx, request.Target, request.RouteAdapter)
		if err != nil {
			return actionFailure(err)
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	case "save-desktop-credential-and-open":
		result, err := service.SaveDesktopCredentialAndOpenVia(ctx, request.Target, request.Credential, request.RouteAdapter)
		request.Credential = ""
		if err != nil {
			return actionFailure(err)
		}
		return Response{SchemaVersion: SchemaVersion, Result: result}
	default:
		return failed("UNKNOWN_ACTION", "local-api", "Unknown local action: "+request.Action, "Use a versioned action supported by this daemon.")
	}
}

func actionFailure(err error) Response {
	var fault *actions.Fault
	if errors.As(err, &fault) {
		return failed(fault.Code, fault.Stage, fault.Summary, fault.Remediation)
	}
	return failed("REMOTE_ACTION_FAILED", "session", "PF Remote could not complete the remote task.", "Check the computer status and try again.")
}

func failed(code, stage, summary, remediation string) Response {
	return Response{SchemaVersion: SchemaVersion, Error: &contracts.Error{
		SchemaVersion: contracts.ErrorSchema, Code: code, Stage: stage,
		CorrelationID: correlationID(), Summary: summary, Remediation: remediation,
	}}
}

func correlationID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(b)
}

func Encode(response Response) ([]byte, error) { return json.Marshal(response) }
