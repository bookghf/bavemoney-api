package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterRefills(t *testing.T) {
	l := newRateLimiter(60, 2) // one token per second
	now := time.Now()
	if !l.allow("a", now) || !l.allow("a", now) {
		t.Fatal("burst should allow two requests")
	}
	if l.allow("a", now) {
		t.Fatal("third immediate request should be limited")
	}
	if !l.allow("b", now) {
		t.Fatal("other clients have their own bucket")
	}
	if !l.allow("a", now.Add(time.Second)) {
		t.Fatal("bucket should refill after a second")
	}
}

func TestLimitOnlyGuardsCredentialPosts(t *testing.T) {
	l := newRateLimiter(1, 1)
	handler := l.limit([]string{"/api/v1/auth/"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	call := func(method, path string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Code
	}
	call(http.MethodPost, "/api/v1/auth/login")
	if got := call(http.MethodPost, "/api/v1/auth/login"); got != http.StatusTooManyRequests {
		t.Errorf("second login = %d, want 429", got)
	}
	if got := call(http.MethodGet, "/api/v1/accounts"); got != http.StatusOK {
		t.Errorf("other route = %d, want 200", got)
	}
}

func TestObserveRecoversPanics(t *testing.T) {
	handler := observe(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID")
	}
}
