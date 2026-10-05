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
	mux.HandleFunc("/api/v1/me", h.me)
	mux.HandleFunc("/api/v1/me/password", h.changePassword)
	mux.HandleFunc("/api/v1/me/reset", h.resetAccount)
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
	if err := validate.Password(creds.Password); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	creds.DisplayName = strings.TrimSpace(creds.DisplayName)
	if err := validate.Name(creds.DisplayName, maxDisplayName, "display name"); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
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

// maxDisplayName bounds display names.
const maxDisplayName = 60

// me serves GET /me (the signed-in user), PATCH /me (edit profile), and
// DELETE /me (delete the account).
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		found, err := h.repo.ByID(r.Context(), userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.WriteError(w, http.StatusNotFound, "user not found")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "could not fetch profile")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"user": found})
	case http.MethodPatch:
		h.updateProfile(w, r, userID)
	case http.MethodDelete:
		h.deleteAccount(w, r, userID)
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request, userID string) {
	var req UpdateProfileRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.DisplayName == nil && req.DefaultCurrency == nil {
		httpx.WriteError(w, http.StatusBadRequest, "no fields provided")
		return
	}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if err := validate.Name(name, maxDisplayName, "display name"); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.DisplayName = &name
	}
	if req.DefaultCurrency != nil {
		code := strings.ToUpper(strings.TrimSpace(*req.DefaultCurrency))
		known, err := h.repo.CurrencyExists(r.Context(), code)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not check currency")
			return
		}
		if !validate.Currency(code) || !known {
			httpx.WriteError(w, http.StatusBadRequest, "default_currency is not a supported currency")
			return
		}
		req.DefaultCurrency = &code
	}

	updated, err := h.repo.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not update profile")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"user": updated})
}

// changePassword checks the current password, stores the new one, and signs
// out every other session: all refresh tokens are revoked and a fresh session
// is returned for this device.
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}
	var req ChangePasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := validate.Password(req.NewPassword); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := h.repo.PasswordHash(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not change password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}
	if req.CurrentPassword == req.NewPassword {
		httpx.WriteError(w, http.StatusBadRequest, "new password must be different")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := h.repo.SetPassword(r.Context(), userID, string(newHash)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not change password")
		return
	}
	if err := h.refresh.RevokeAll(r.Context(), userID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not sign out other devices")
		return
	}

	owner, err := h.repo.ByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch profile")
		return
	}
	h.writeSession(w, r, owner, http.StatusOK)
}

// resetAccount erases the user's ledger data (transactions, accounts, budgets,
// custom categories) after re-checking their password. The login and sessions
// stay, so the user can start over right away.
func (h *Handler) resetAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}
	if !h.confirmPassword(w, r, userID, "could not reset account") {
		return
	}

	counts, err := h.repo.ResetData(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not reset account")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"message": "account reset", "deleted": counts})
}

// deleteAccount permanently deletes the user and everything they own, after
// the password confirms intent. Sessions go with the user row, and the
// access token stops working because the user no longer exists.
func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request, userID string) {
	if !h.confirmPassword(w, r, userID, "could not delete account") {
		return
	}
	if err := h.repo.Delete(r.Context(), userID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// confirmPassword reads a PasswordConfirmation body and checks it against the
// user's password, writing the error response when it does not match.
func (h *Handler) confirmPassword(w http.ResponseWriter, r *http.Request, userID, failure string) bool {
	var req PasswordConfirmation
	if !httpx.DecodeJSON(w, r, &req) {
		return false
	}
	if req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "password is required")
		return false
	}
	hash, err := h.repo.PasswordHash(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, failure)
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "password is incorrect")
		return false
	}
	return true
}
