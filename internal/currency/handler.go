// Package currency lists the currencies users can pick for accounts, budgets,
// and their default currency.
package currency

import (
	"database/sql"
	"net/http"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
)

// Currency is an active ISO 4217 currency.
type Currency struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

// Handler serves GET /api/v1/currencies.
type Handler struct {
	db   *sql.DB
	auth *auth.Authenticator
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(db *sql.DB, authenticator *auth.Authenticator) *Handler {
	return &Handler{db: db, auth: authenticator}
}

// Register mounts the route on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/currencies", h.list)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.auth.Require(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT code, name, symbol FROM currencies WHERE is_active ORDER BY code`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list currencies")
		return
	}
	defer rows.Close()

	currencies := []Currency{}
	for rows.Next() {
		var c Currency
		if err := rows.Scan(&c.Code, &c.Name, &c.Symbol); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not list currencies")
			return
		}
		currencies = append(currencies, c)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"currencies": currencies})
}
