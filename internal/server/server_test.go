package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ledger-api/internal/config"
)

// Router panics at startup when two handlers register conflicting patterns.
func TestRouterRegistersWithoutConflict(t *testing.T) {
	Router(config.Config{JWTSecret: "test", CORSAllowedOrigins: "*"}, nil)
}

func TestCORSPreflight(t *testing.T) {
	handler := cors(splitOrigins("http://localhost:8081"), http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/accounts", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8081" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != corsAllowedHeaders {
		t.Fatalf("Access-Control-Allow-Headers = %q", got)
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	handler := cors(splitOrigins("http://localhost:8081"), http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}
