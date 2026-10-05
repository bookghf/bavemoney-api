// Package user owns registration, login, and the user record itself.
package user

// User is the API representation of an account holder.
type User struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name"`
	DefaultCurrency string `json:"default_currency"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
}

// Credentials is the payload accepted by both register and login.
type Credentials struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	DisplayName     string `json:"display_name"`
	DefaultCurrency string `json:"default_currency"`
}

// RefreshRequest is the POST /auth/refresh payload.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest is the POST /auth/logout payload. All revokes every session for
// the user instead of just the presented one.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
	All          bool   `json:"all,omitempty"`
}

// AuthResponse is returned after a successful register, login, or refresh.
// ExpiresIn is the access token's lifetime in seconds.
type AuthResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	User                  User   `json:"user"`
}

// UpdateProfileRequest is the PATCH /me payload; nil fields are left alone.
// Email is not editable here: changing it needs a verification step.
type UpdateProfileRequest struct {
	DisplayName     *string `json:"display_name"`
	DefaultCurrency *string `json:"default_currency"`
}

// ChangePasswordRequest is the POST /me/password payload.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// PasswordConfirmation is the POST /me/reset and DELETE /me payload: the
// password confirms intent.
type PasswordConfirmation struct {
	Password string `json:"password"`
}

// ResetCounts reports how much ledger data a reset removed.
type ResetCounts struct {
	Transactions int64 `json:"transactions"`
	Accounts     int64 `json:"accounts"`
	Budgets      int64 `json:"budgets"`
	Categories   int64 `json:"categories"`
}
