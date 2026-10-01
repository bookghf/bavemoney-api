package budget

import (
	"errors"
	"net/http"
	"time"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

const basePath = "/api/v1/budgets"

// Handler serves the /api/v1/budgets endpoints.
type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

// Register mounts the budget routes on mux.
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
		h.delete(w, r, userID, id)
	default:
		httpx.MethodNotAllowed(w)
	}
}

// location reads the caller's time zone (?tz=Asia/Bangkok, default UTC),
// which decides what "today" and the period's days are.
func location(w http.ResponseWriter, r *http.Request) (*time.Location, bool) {
	loc, ok := validate.Location(r.URL.Query().Get("tz"))
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "tz must be an IANA time zone such as Asia/Bangkok")
	}
	return loc, ok
}

// writeErr maps repository errors to responses.
func writeErr(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCategoryNotFound), errors.Is(err, ErrCategoryType), errors.Is(err, ErrUnknownCurrency):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "could not "+action+" budget")
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, userID string) {
	loc, ok := location(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if period != "" && !periods[period] {
		httpx.WriteError(w, http.StatusBadRequest, "period must be weekly, monthly, or yearly")
		return
	}
	budgets, err := h.repo.List(r.Context(), userID, period, loc)
	if err != nil {
		writeErr(w, err, "list")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"budgets": budgets})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, userID string) {
	loc, ok := location(w, r)
	if !ok {
		return
	}
	var req CreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := validateCreate(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.repo.Create(r.Context(), userID, req, loc)
	if err != nil {
		writeErr(w, err, "create")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"budget": created})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, userID, id string) {
	loc, ok := location(w, r)
	if !ok {
		return
	}
	found, err := h.repo.Get(r.Context(), userID, id, loc)
	if err != nil {
		writeErr(w, err, "fetch")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"budget": found})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request, userID, id string) {
	loc, ok := location(w, r)
	if !ok {
		return
	}
	var req UpdateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Empty() {
		httpx.WriteError(w, http.StatusBadRequest, "no fields provided")
		return
	}
	if err := validateUpdate(req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repo.Update(r.Context(), userID, id, req); err != nil {
		writeErr(w, err, "update")
		return
	}
	updated, err := h.repo.Get(r.Context(), userID, id, loc)
	if err != nil {
		writeErr(w, err, "fetch")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"budget": updated})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request, userID, id string) {
	if err := h.repo.Delete(r.Context(), userID, id); err != nil {
		writeErr(w, err, "delete")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Budget deleted",
		"id":      id,
	})
}
