package user

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"ledger-api/internal/auth"
	"ledger-api/internal/database"
	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

// defaultCurrency is used when register omits default_currency; the app's
// primary market is Thailand.
const defaultCurrency = "THB"

// minPasswordLength is the shortest password register accepts.
const minPasswordLength = 8

// statusActive is the only account status allowed to sign in.
const statusActive = "active"

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

	if !validate.Email(creds.Email) {
		httpx.WriteError(w, http.StatusBadRequest, "email is not valid")
		return
	}
	if len(creds.Password) < minPasswordLength {
		httpx.WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	creds.DisplayName = strings.TrimSpace(creds.DisplayName)
	creds.DefaultCurrency = strings.ToUpper(strings.TrimSpace(creds.DefaultCurrency))
	if creds.DefaultCurrency == "" {
		creds.DefaultCurrency = defaultCurrency
	}
	if known, err := h.repo.CurrencyExists(r.Context(), creds.DefaultCurrency); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not check currency")
		return
	} else if !validate.Currency(creds.DefaultCurrency) || !known {
		httpx.WriteError(w, http.StatusBadRequest, "default_currency is not a supported currency")
		return
	}

	// Older rows may differ from the lowercased email only by case.
	if _, _, err := h.repo.ByEmail(r.Context(), creds.Email); err == nil {
		httpx.WriteError(w, http.StatusConflict, "an account with this email already exists")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	created, err := h.repo.Create(r.Context(), creds.Email, string(hash), creds.DisplayName, creds.DefaultCurrency)
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpx.WriteError(w, http.StatusConflict, "an account with this email already exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not create account")
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
	if stored.Status != statusActive {
		httpx.WriteError(w, http.StatusForbidden, "this account is suspended")
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

	userID, familyID, err := h.refresh.Consume(r.Context(), req.RefreshToken)
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
	if owner.Status != statusActive {
		httpx.WriteError(w, http.StatusForbidden, "this account is suspended")
		return
	}

	h.writeSessionInFamily(w, r, owner, familyID, http.StatusOK)
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

// writeSession issues a fresh access/refresh token pair for a new login.
func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, owner User, status int) {
	h.writeSessionInFamily(w, r, owner, "", status)
}

// writeSessionInFamily issues a fresh access/refresh token pair; a non-empty
// familyID continues the rotation chain of an existing login.
func (h *Handler) writeSessionInFamily(w http.ResponseWriter, r *http.Request, owner User, familyID string, status int) {
	accessToken, err := h.auth.IssueAccessToken(owner.ID, owner.Email)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not issue access token")
		return
	}

	refreshToken, err := h.refresh.Issue(r.Context(), owner.ID, familyID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not issue refresh token")
		return
	}

	httpx.WriteJSON(w, status, AuthResponse{
		AccessToken:           accessToken,
		ExpiresIn:             int(auth.AccessTokenTTL.Seconds()),
		RefreshToken:          refreshToken.Value,
		RefreshTokenExpiresIn: int(auth.RefreshTokenTTL.Seconds()),
		User:                  owner,
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

	creds.Email = validate.NormalizeEmail(creds.Email)
	if creds.Email == "" || strings.TrimSpace(creds.Password) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email and password are required")
		return Credentials{}, false
	}
	return creds, true
}
