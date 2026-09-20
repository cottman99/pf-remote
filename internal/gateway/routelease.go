package gateway

import (
	"errors"
	"net/http"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/routelease"
)

func registerRouteLeaseHandlers(mux *http.ServeMux, manager *routelease.Manager) {
	registerRouteLeaseAction(mux, "POST /api/v1/route-leases", func(r *http.Request) (any, error) {
		var request routelease.Request
		if err := decodeJSONRequest(r, &request); err != nil {
			return nil, invalidRouteRequest()
		}
		return manager.Issue(r.Context(), request)
	})
	registerRouteLeaseAction(mux, "POST /api/v1/route-leases/release", func(r *http.Request) (any, error) {
		var request routelease.ReleaseRequest
		if err := decodeJSONRequest(r, &request); err != nil {
			return nil, invalidRouteRequest()
		}
		return manager.Release(r.Context(), request)
	})
}

func registerRouteLeaseAction(mux *http.ServeMux, pattern string, action func(*http.Request) (any, error)) {
	mux.Handle(pattern, requireSecureControlPlane(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, err := action(r)
		if err != nil {
			writeRouteLeaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})))
}

func invalidRouteRequest() *routelease.Fault {
	return &routelease.Fault{Code: "INVALID_ROUTE_REQUEST", Stage: "route", Summary: "The route lease request is not valid versioned JSON.", Remediation: "Send one supported JSON object with no unknown fields."}
}

func writeRouteLeaseError(w http.ResponseWriter, err error) {
	var routeFault *routelease.Fault
	if !errors.As(err, &routeFault) {
		routeFault = &routelease.Fault{Code: "ROUTE_UNAVAILABLE", Stage: "route", Summary: "The Gateway route is unavailable.", Remediation: "Inspect redacted route diagnostics and retry."}
	}
	status := http.StatusBadRequest
	switch routeFault.Code {
	case "INVALID_SIGNATURE", "ROUTE_NOT_AUTHORIZED":
		status = http.StatusForbidden
	case "ROUTE_LEASE_NOT_FOUND":
		status = http.StatusNotFound
	case "REQUEST_REPLAYED":
		status = http.StatusConflict
	case "ROUTE_NOT_CONFIGURED", "ROUTE_UNAVAILABLE", "ROUTE_CLEANUP_FAILED", "ROUTE_CAPACITY_REACHED", "ENTROPY_UNAVAILABLE":
		status = http.StatusServiceUnavailable
	}
	writeStandardError(w, status, &enrollment.Fault{Code: routeFault.Code, Stage: routeFault.Stage, Summary: routeFault.Summary, Remediation: routeFault.Remediation})
}
