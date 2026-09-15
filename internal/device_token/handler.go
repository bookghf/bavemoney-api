package device_token

import (
	"net/http"
	"strings"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
)

const basePath = "/api/v1/device-tokens"

type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(basePath, h.create)
	mux.HandleFunc(basePath+"/", h.delete)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	var req CreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if req.FCMToken == "" || req.Platform == "" {
		httpx.WriteError(w, http.StatusBadRequest, "fcm_token and platform are required")
		return
	}

	token, err := h.repo.Upsert(r.Context(), userID, req.FCMToken, req.Platform)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not register device token")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, token)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	if r.Method != http.MethodDelete {
		httpx.MethodNotAllowed(w)
		return
	}

	token := strings.TrimPrefix(r.URL.Path, basePath+"/")
	if token == "" {
		httpx.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	err := h.repo.Delete(r.Context(), userID, token)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete device token")
		return
	}

	httpx.WriteMessage(w, http.StatusOK, "Device token removed")
}
