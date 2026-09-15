package user

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
)

const defaultCurrency = "USD"

// Handler serves the /api/v1/auth endpoints.
type Handler struct {
	repo    *Repository
	auth    *auth.Authenticator
	refresh *auth.RefreshStore
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(repo *Repository, authenticator *auth.Authenticator, refresh *auth.RefreshStore) *Handler {
	return &Handler{repo: repo, auth: authenticator, refresh: refresh}
}

// Register mounts the auth routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/register", h.register)
	mux.HandleFunc("/api/v1/auth/login", h.login)
	mux.HandleFunc("/api/v1/auth/refresh", h.refreshToken)
	mux.HandleFunc("/api/v1/auth/logout", h.logout)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	creds, ok := h.credentials(w, r)
	if !ok {
		return
	}

	if creds.DefaultCurrency == "" {
		creds.DefaultCurrency = defaultCurrency
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	created, err := h.repo.Create(r.Context(), creds.Email, string(hash), creds.DisplayName, creds.DefaultCurrency)
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "user already exists")
		return
	}

	h.writeSession(w, r, created, http.StatusCreated)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	creds, ok := h.credentials(w, r)
	if !ok {
		return
	}

	stored, hash, err := h.repo.ByEmail(r.Context(), creds.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(creds.Password)); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.writeSession(w, r, stored, http.StatusOK)
}

// refreshToken exchanges a valid refresh token for a new access token, rotating
// the refresh token in the process: the presented one is consumed and cannot be
// replayed.
func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	var req RefreshRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	userID, err := h.refresh.Consume(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not refresh session")
		return
	}

	owner, err := h.repo.ByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	h.writeSession(w, r, owner, http.StatusOK)
}

// logout revokes the presented refresh token, or every token for its owner when
// `all` is set. Access tokens already handed out stay valid until they expire.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	var req LogoutRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	userID, err := h.refresh.Revoke(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			// Already logged out: report success so clients can safely retry.
			httpx.WriteMessage(w, http.StatusOK, "logged out")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not log out")
		return
	}

	if req.All {
		if err := h.refresh.RevokeAll(r.Context(), userID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not log out")
			return
		}
	}

	httpx.WriteMessage(w, http.StatusOK, "logged out")
}

// writeSession issues a fresh access/refresh token pair for the user.
func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, owner User, status int) {
	accessToken, err := h.auth.IssueAccessToken(owner.ID, owner.Email)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not issue access token")
		return
	}

	refreshToken, err := h.refresh.Issue(r.Context(), owner.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not issue refresh token")
		return
	}

	httpx.WriteJSON(w, status, map[string]interface{}{
		"user":                    owner,
		"access_token":            accessToken,
		"refresh_token":           refreshToken.Value,
		"expires_in":              int(auth.AccessTokenTTL.Seconds()),
		"refresh_token_expires_in": int(auth.RefreshTokenTTL.Seconds()),
	})
}

// credentials enforces the POST-only, well-formed-payload contract shared by
// register and login.
func (h *Handler) credentials(w http.ResponseWriter, r *http.Request) (Credentials, bool) {
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return Credentials{}, false
	}

	var creds Credentials
	if !httpx.DecodeJSON(w, r, &creds) {
		return Credentials{}, false
	}

	if strings.TrimSpace(creds.Email) == "" || strings.TrimSpace(creds.Password) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email and password are required")
		return Credentials{}, false
	}
	return creds, true
}
