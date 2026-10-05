package account

import (
	"errors"
	"net/http"
	"strings"

	"ledger-api/internal/auth"
	"ledger-api/internal/database"
	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

const basePath = "/api/v1/accounts"

// Handler serves the /api/v1/accounts endpoints.
type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

// Register mounts the account routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(basePath, h.collection)
	mux.HandleFunc(basePath+"/", h.item)
}

func (h *Handler) collection(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.list(w, r, userID)
	case http.MethodPost:
		h.create(w, r, userID)
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	if strings.HasSuffix(r.URL.Path, "/reconcile") {
		h.reconcileRoute(w, r, userID)
		return
	}
	id, ok := httpx.PathID(w, r, basePath+"/")
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.get(w, r, userID, id)
	case http.MethodPatch:
		h.update(w, r, userID, id)
	case http.MethodDelete:
		h.archive(w, r, userID, id)
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, userID string) {
	accounts, err := h.repo.List(r.Context(), userID, r.URL.Query().Get("include_archived") == "true")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list accounts")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"accounts": accounts})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, userID string) {
	var req CreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := normalizeCreate(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.repo.Create(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrUnknownCurrency) {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not create account")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"account": created})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, userID, id string) {
	found, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "account not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch account")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, found)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request, userID, id string) {
	var req UpdateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Empty() {
		httpx.WriteError(w, http.StatusBadRequest, "no fields provided")
		return
	}

	existing, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "account not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch account")
		return
	}
	if err := checkUpdate(existing, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repo.Update(r.Context(), userID, existing, req); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "account not found")
		case errors.Is(err, ErrCurrencyLocked), errors.Is(err, ErrUnknownCurrency), database.IsCheckViolation(err):
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not update account")
		}
		return
	}

	updated, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch account")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"account": updated})
}

// reconcileRoute serves POST /accounts/{id}/reconcile.
func (h *Handler) reconcileRoute(w http.ResponseWriter, r *http.Request, userID string) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, basePath+"/"), "/reconcile")
	if !validate.UUID(id) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	var req ReconcileRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	existing, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "account not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch account")
		return
	}
	if err := checkReconcile(existing, req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repo.Reconcile(r.Context(), userID, id, string(req.Balance)); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "account not found")
		case database.IsNumericOverflow(err):
			httpx.WriteError(w, http.StatusBadRequest, "balance is too large for this account")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not reconcile account")
		}
		return
	}

	updated, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch account")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"account": updated})
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request, userID, id string) {
	if err := h.repo.Archive(r.Context(), userID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "account not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete account")
		return
	}
	httpx.WriteMessage(w, http.StatusOK, "account archived")
}
