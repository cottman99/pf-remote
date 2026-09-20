package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndVersion(t *testing.T) {
	handler := (Server{Version: "dev", Commit: "test"}).Handler()
	for _, path := range []string{"/healthz", "/api/v1/version"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s missing security headers", path)
		}
	}
}
