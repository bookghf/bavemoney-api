package category

import (
	"database/sql"
	"errors"
	"net/http"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
)

const basePath = "/api/v1/categories"

// Handler serves the /api/v1/categories endpoints.
type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

// Register mounts the category routes on mux.
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

func (h *Handler) list(w http.ResponseWriter, r *http.Request, userID string) {
	categories, err := h.repo.List(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list categories")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"categories": BuildTree(categories)})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, userID string) {
	var req CreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.Type == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	if !h.validParent(w, r, userID, req.ParentID) {
		return
	}

	created, err := h.repo.Create(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create category")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, userID, id string) {
	found, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "category not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch category")
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
	if req.ParentID != nil && !h.validParent(w, r, userID, *req.ParentID) {
		return
	}

	if err := h.repo.Update(r.Context(), userID, id, req); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not update category")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "category updated"})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request, userID, id string) {
	if err := h.repo.Delete(r.Context(), userID, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete category")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "category deleted"})
}

// validParent reports whether parentID is usable, writing the 400 response when
// it is not.
func (h *Handler) validParent(w http.ResponseWriter, r *http.Request, userID, parentID string) bool {
	if err := h.repo.ValidateParent(r.Context(), userID, parentID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
