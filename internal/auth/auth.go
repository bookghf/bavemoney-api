// Package auth issues and validates the API's bearer tokens.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ledger-api/internal/httpx"
)

// AccessTokenTTL is how long an issued access token stays valid. It is short
// because access tokens can not be revoked: clients renew them with a refresh
// token (see RefreshTokenTTL), which re-checks the account's status.
const AccessTokenTTL = 15 * time.Minute

// AdminTokenTTL is how long an admin session lasts. Admins have no refresh
// token and simply sign in again.
const AdminTokenTTL = 8 * time.Hour

const issuer = "ledger-api"

// Token audiences. A token is only accepted by the side it was issued for, and
// the two sides also sign with different keys.
const (
	audienceUser  = "app"
	audienceAdmin = "admin"
)

// Claims is the JWT payload carried by an access token.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	// Role is only set on admin tokens.
	Role string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

// AccountStatus looks up whether a user still exists and is active.
type AccountStatus func(ctx context.Context, userID string) (exists, active bool, err error)

// Authenticator signs and verifies tokens with HMAC secrets.
type Authenticator struct {
	userKey  []byte
	adminKey []byte
	// status, when set, is checked on every authenticated request, so a
	// suspended or deleted user loses access at once instead of keeping it
	// until their access token expires.
	status AccountStatus
}

// CheckAccountStatus makes Require reject suspended and deleted users.
func (a *Authenticator) CheckAccountStatus(status AccountStatus) {
	a.status = status
}

// New returns an Authenticator backed by secret. The admin signing key is
// derived from it, so a user token can never verify as an admin token.
func New(secret string) *Authenticator {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("ledger-api admin tokens"))
	return &Authenticator{userKey: []byte(secret), adminKey: mac.Sum(nil)}
}

// IssueAccessToken returns a signed access token for an app user.
func (a *Authenticator) IssueAccessToken(userID, email string) (string, error) {
	return a.sign(a.userKey, audienceUser, AccessTokenTTL, Claims{UserID: userID, Email: email})
}

// IssueAdminToken returns a signed token for a backoffice admin.
func (a *Authenticator) IssueAdminToken(adminID, email, role string) (string, error) {
	return a.sign(a.adminKey, audienceAdmin, AdminTokenTTL, Claims{UserID: adminID, Email: email, Role: role})
}

func (a *Authenticator) sign(key []byte, audience string, ttl time.Duration, claims Claims) (string, error) {
	now := time.Now()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		IssuedAt:  jwt.NewNumericDate(now),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

func (a *Authenticator) verify(r *http.Request, key []byte, audience string) (*Claims, error) {
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		return nil, fmt.Errorf("missing bearer token")
	}
	tokenString := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(*jwt.Token) (interface{}, error) { return key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid || claims.UserID == "" {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// UserID extracts and verifies the bearer token on the request and returns the
// authenticated user's id.
func (a *Authenticator) UserID(r *http.Request) (string, error) {
	claims, err := a.verify(r, a.userKey, audienceUser)
	if err != nil {
		return "", err
	}
	return claims.UserID, nil
}

// Admin verifies an admin bearer token and returns its claims.
func (a *Authenticator) Admin(r *http.Request) (*Claims, error) {
	return a.verify(r, a.adminKey, audienceAdmin)
}

// Require resolves the caller's user id, writing a 401 response when the
// request is not authenticated and a 403 when the account is suspended.
func (a *Authenticator) Require(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, err := a.UserID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, err.Error())
		return "", false
	}
	if a.status != nil {
		exists, active, err := a.status(r.Context(), userID)
		switch {
		case err != nil:
			httpx.WriteError(w, http.StatusInternalServerError, "could not check account")
			return "", false
		case !exists:
			httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
			return "", false
		case !active:
			httpx.WriteError(w, http.StatusForbidden, "this account is suspended")
			return "", false
		}
	}
	return userID, true
}
