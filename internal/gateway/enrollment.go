package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const maxControlRequestSize = 1 << 20

func registerEnrollmentHandlers(mux *http.ServeMux, manager *enrollment.Manager) {
	registerJSONAction(mux, "POST /api/v1/updates/sync", func(request enrollment.UpdateRequest) (any, error) {
		return manager.SyncUpdates(request)
	})
	registerJSONAction(mux, "POST /api/v1/owner/initialize", func(request enrollment.OwnerInitializationRequest) (any, error) {
		return manager.InitializeOwner(request)
	})
	registerJSONAction(mux, "POST /api/v1/device-authorizations", func(request enrollment.DeviceAuthorizationRequest) (any, error) {
		return manager.BeginDeviceAuthorization(request)
	})
	registerJSONAction(mux, "POST /api/v1/device-authorizations/approve", func(request enrollment.OwnerApprovalRequest) (any, error) {
		return manager.ApproveDevice(request)
	})
	registerJSONAction(mux, "POST /api/v1/device-authorizations/poll", func(request enrollment.DevicePollRequest) (any, error) {
		return manager.PollDevice(request)
	})
	registerJSONAction(mux, "POST /api/v1/devices/revoke", func(request enrollment.OwnerRevocationRequest) (any, error) {
		return manager.RevokeDevice(request)
	})
	registerJSONAction(mux, "POST /api/v1/versions/sync", func(request enrollment.VersionSyncRequest) (any, error) {
		return manager.SyncVersion(request)
	})
	registerJSONAction(mux, "POST /api/v1/capabilities/shell/publish", func(request enrollment.ShellCapabilityPublishRequest) (any, error) {
		return manager.PublishShellCapability(request)
	})
	registerJSONAction(mux, "POST /api/v1/capabilities/shell/list", func(request enrollment.ShellCapabilityListRequest) (any, error) {
		return manager.ListShellCapabilities(request)
	})
}

func registerJSONAction[Request any](mux *http.ServeMux, pattern string, action func(Request) (any, error)) {
	mux.Handle(pattern, requireSecureControlPlane(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request Request
		if err := decodeJSONRequest(r, &request); err != nil {
			writeStandardError(w, http.StatusBadRequest, &enrollment.Fault{
				Code: "INVALID_REQUEST", Stage: "gateway",
				Summary:     "The Gateway request is not valid versioned JSON.",
				Remediation: "Send one JSON object smaller than 1 MiB with no unknown fields.",
			})
			return
		}
		response, err := action(request)
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})))
}

func decodeJSONRequest(r *http.Request, destination any) error {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/json" {
		return errors.New("content type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxControlRequestSize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request contains trailing data")
	}
	return nil
}

func requireSecureControlPlane(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && !remoteIsLoopback(r.RemoteAddr) {
			writeStandardError(w, http.StatusForbidden, &enrollment.Fault{
				Code: "TLS_REQUIRED", Stage: "gateway",
				Summary:     "Remote control-plane requests require TLS.",
				Remediation: "Use HTTPS or call the development Gateway from the local machine.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func remoteIsLoopback(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return false
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func writeEnrollmentError(w http.ResponseWriter, err error) {
	var enrollmentFault *enrollment.Fault
	if !errors.As(err, &enrollmentFault) {
		enrollmentFault = &enrollment.Fault{
			Code: "INTERNAL_ERROR", Stage: "gateway",
			Summary:     "The Gateway could not complete the request.",
			Remediation: "Retry with a new request ID or inspect redacted Gateway diagnostics.",
		}
	}
	writeStandardError(w, enrollmentHTTPStatus(enrollmentFault.Code), enrollmentFault)
}

func writeStandardError(w http.ResponseWriter, status int, fault *enrollment.Fault) {
	writeJSON(w, status, contracts.Error{
		SchemaVersion: contracts.ErrorSchema, Code: fault.Code, Stage: fault.Stage,
		CorrelationID: gatewayCorrelationID(), Summary: fault.Summary, Remediation: fault.Remediation,
	})
}

func enrollmentHTTPStatus(code string) int {
	switch code {
	case "INVALID_SIGNATURE", "OWNER_IDENTITY_MISMATCH", "DEVICE_IDENTITY_MISMATCH", "DEVICE_REVOKED":
		return http.StatusForbidden
	case "OWNER_ALREADY_INITIALIZED", "OWNER_NOT_INITIALIZED", "DEVICE_ALREADY_ACTIVE", "DEVICE_ALREADY_REVOKED", "ACTIVATION_NOT_PENDING", "REQUEST_REPLAYED", "OWNER_DEVICE_REQUIRES_RECOVERY", "CAPABILITY_BINDING_STALE", "CAPABILITY_BINDING_CONFLICT":
		return http.StatusConflict
	case "DEVICE_NOT_FOUND", "INVALID_DEVICE_CODE", "INVALID_USER_CODE", "ACTIVATION_EXPIRED":
		return http.StatusNotFound
	case "USER_CODE_RATE_LIMITED", "SLOW_DOWN":
		return http.StatusTooManyRequests
	case "UNSUPPORTED_SCHEMA", "UNSUPPORTED_CLIENT_VERSION":
		return http.StatusUpgradeRequired
	case "AUTHORIZATION_PENDING":
		return http.StatusAccepted
	case "ENTROPY_UNAVAILABLE", "INTERNAL_ERROR":
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func gatewayCorrelationID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(buffer)
}
