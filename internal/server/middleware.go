package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"ledger-api/internal/httpx"
)

// statusWriter remembers the status code a handler wrote.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// observe gives every request an X-Request-ID, logs one structured line per
// request, and turns a panicking handler into a 500 instead of a dropped
// connection.
func observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic", "request_id", id, "error", recovered, "stack", string(debug.Stack()))
				httpx.WriteError(sw, http.StatusInternalServerError, "internal error")
			}
			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			}
			slog.Log(r.Context(), level, "request",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()
		next.ServeHTTP(sw, r)
	})
}

func newRequestID() string {
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

// rateLimiter is a per-client token bucket guarding the credential endpoints
// against password guessing. It is in-memory, so each API instance keeps its
// own counts.
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*bucket
	rate    float64 // tokens per second
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute, burst int) *rateLimiter {
	return &rateLimiter{clients: map[string]*bucket{}, rate: float64(perMinute) / 60, burst: float64(burst)}
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Forget idle clients so the map can not grow without bound.
	if len(l.clients) > 10_000 {
		for k, b := range l.clients {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.clients, k)
			}
		}
	}

	b, ok := l.clients[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.clients[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// limit applies the limiter to POSTs whose path starts with one of prefixes.
// Each endpoint has its own bucket per client, so hammering one (say, a reset
// with wrong passwords) can not lock everyone on that IP out of logging in.
func (l *rateLimiter) limit(prefixes []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(r.URL.Path, prefix) && r.Method == http.MethodPost {
				if !l.allow(clientIP(r)+"|"+prefix, time.Now()) {
					w.Header().Set("Retry-After", "60")
					httpx.WriteError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
					return
				}
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the connection's remote address. X-Forwarded-For is ignored on
// purpose: without a trusted proxy in front, any client could forge it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
