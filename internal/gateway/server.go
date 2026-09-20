package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/routelease"
)

type Server struct {
	Version     string
	Commit      string
	Enrollment  *enrollment.Manager
	RouteLeases *routelease.Manager
}

func (s Server) Handler() http.Handler {
	manager := s.Enrollment
	if manager == nil {
		manager = enrollment.NewManager()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": "pfremote.gateway-version/v1",
			"version":        s.Version,
			"commit":         s.Commit,
			"phase":          "identity-catalog-grants",
		})
	})
	registerEnrollmentHandlers(mux, manager)
	if s.RouteLeases != nil {
		registerRouteLeaseHandlers(mux, s.RouteLeases)
	}
	return securityHeaders(mux)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
