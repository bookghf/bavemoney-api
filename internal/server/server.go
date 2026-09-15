// Package server wires the routes and returns the configured HTTP server.
package server

import (
	"database/sql"
	"net/http"

	"ledger-api/internal/account"
	"ledger-api/internal/admin"
	"ledger-api/internal/attachment"
	"ledger-api/internal/auth"
	"ledger-api/internal/budget"
	"ledger-api/internal/category"
	"ledger-api/internal/config"
	"ledger-api/internal/device_token"
	"ledger-api/internal/report"
	"ledger-api/internal/transaction"
	"ledger-api/internal/user"
)

// registrar is implemented by every domain handler.
type registrar interface {
	Register(mux *http.ServeMux)
}

// New builds the router and returns an http.Server ready to listen.
func New(cfg config.Config, db *sql.DB) *http.Server {
	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: Router(cfg, db),
	}
}

// Router builds the API router.
func Router(cfg config.Config, db *sql.DB) http.Handler {
	authenticator := auth.New(cfg.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)

	handlers := []registrar{
		user.NewHandler(user.NewRepository(db), authenticator, auth.NewRefreshStore(db)),
		account.NewHandler(account.NewRepository(db), authenticator),
		category.NewHandler(category.NewRepository(db), authenticator),
		budget.NewHandler(budget.NewRepository(db), authenticator),
		transaction.NewHandler(transaction.NewRepository(db), authenticator),
		device_token.NewHandler(device_token.NewRepository(db), authenticator),
		attachment.NewHandler(attachment.NewRepository(db), authenticator),
		report.NewHandler(report.NewRepository(db), authenticator),
		admin.NewHandler(db, authenticator),
	}
	for _, handler := range handlers {
		handler.Register(mux)
	}

	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
