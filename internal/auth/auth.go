// Package auth issues and validates the API's bearer tokens.
package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ledger-api/internal/httpx"
)

// AccessTokenTTL is how long an issued access token stays valid. Clients renew
// it with a refresh token (see RefreshTokenTTL) rather than re-authenticating.
const AccessTokenTTL = 24 * time.Hour

// Claims is the JWT payload carried by an access token.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Authenticator signs and verifies tokens with a shared HMAC secret.
type Authenticator struct {
	secret []byte
}

// New returns an Authenticator backed by secret.
func New(secret string) *Authenticator {
	return &Authenticator{secret: []byte(secret)}
}

// IssueAccessToken returns a signed access token for the user.
func (a *Authenticator) IssueAccessToken(userID, email string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return token, nil
}

// UserID extracts and verifies the bearer token on the request and returns the
// authenticated user's id.
func (a *Authenticator) UserID(r *http.Request) (string, error) {
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		return "", fmt.Errorf("missing bearer token")
	}
	tokenString := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (interface{}, error) {
		return a.secret, nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	return claims.UserID, nil
}

// Require resolves the caller's user id, writing a 401 response when the
// request is not authenticated.
func (a *Authenticator) Require(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, err := a.UserID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, err.Error())
		return "", false
	}
	return userID, true
}
