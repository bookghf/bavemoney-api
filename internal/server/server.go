// Package server wires the routes and returns the configured HTTP server.
package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"ledger-api/internal/account"
	"ledger-api/internal/admin"
	"ledger-api/internal/attachment"
	"ledger-api/internal/auth"
	"ledger-api/internal/budget"
	"ledger-api/internal/category"
	"ledger-api/internal/config"
	"ledger-api/internal/currency"
	"ledger-api/internal/device_token"
	"ledger-api/internal/report"
	"ledger-api/internal/transaction"
	"ledger-api/internal/user"
)

// registrar is implemented by every domain handler.
type registrar interface {
	Register(mux *http.ServeMux)
}

// New builds the router and returns an http.Server ready to listen. The
// timeouts stop slow or stalled clients from holding connections open.
func New(cfg config.Config, db *sql.DB) *http.Server {
	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           Router(cfg, db),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
}

// credentialPaths are rate limited per client to slow down password guessing.
// Token refresh is left out: it presents an unguessable token, not a password.
var credentialPaths = []string{
	"/api/v1/auth/login",
	"/api/v1/auth/register",
	"/api/v1/admin/auth/login",
	"/api/v1/me/password",
	"/api/v1/me/reset",
}

// Router builds the API router.
func Router(cfg config.Config, db *sql.DB) http.Handler {
	authenticator := auth.New(cfg.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health(db))

	handlers := []registrar{
		user.NewHandler(user.NewRepository(db), authenticator, auth.NewRefreshStore(db)),
		account.NewHandler(account.NewRepository(db), authenticator),
		category.NewHandler(category.NewRepository(db), authenticator),
		budget.NewHandler(budget.NewRepository(db), authenticator),
		transaction.NewHandler(transaction.NewRepository(db), authenticator),
		device_token.NewHandler(device_token.NewRepository(db), authenticator),
		attachment.NewHandler(attachment.NewRepository(db), authenticator),
		report.NewHandler(report.NewRepository(db), authenticator),
		currency.NewHandler(db, authenticator),
		admin.NewHandler(db, authenticator),
	}
	for _, handler := range handlers {
		handler.Register(mux)
	}

	limiter := newRateLimiter(20, 20)
	return observe(cors(splitOrigins(cfg.CORSAllowedOrigins), limiter.limit(credentialPaths, mux)))
}

// health reports "ok" only when the database answers, so orchestrators stop
// routing to an instance that lost its connection.
func health(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				http.Error(w, "database unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
