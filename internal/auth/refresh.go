package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// RefreshTokenTTL is how long an issued refresh token stays valid.
const RefreshTokenTTL = 30 * 24 * time.Hour

// refreshTokenBytes is the entropy of an opaque refresh token.
const refreshTokenBytes = 32

// reuseGrace tolerates a client racing two refreshes with the same token: a
// token presented again within this window after rotation is only rejected,
// while a later replay is treated as theft and revokes the whole family.
const reuseGrace = 10 * time.Second

// ErrInvalidRefreshToken is returned when a refresh token is unknown, expired,
// or already used/revoked.
var ErrInvalidRefreshToken = errors.New("invalid refresh token")

// RefreshToken is a freshly issued refresh token. Value is only ever available
// here — the store keeps just its hash.
type RefreshToken struct {
	Value     string
	ExpiresAt time.Time
}

// RefreshStore issues, rotates, and revokes refresh tokens. Tokens are opaque
// random strings rather than JWTs so that logout can revoke them server-side.
type RefreshStore struct {
	db *sql.DB
}

// NewRefreshStore returns a RefreshStore backed by db.
func NewRefreshStore(db *sql.DB) *RefreshStore {
	return &RefreshStore{db: db}
}

// Issue creates a new refresh token for the user. familyID ties a rotated
// token to the login it descends from; pass "" to start a new family.
func (s *RefreshStore) Issue(ctx context.Context, userID, familyID string) (RefreshToken, error) {
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return RefreshToken{}, fmt.Errorf("generate refresh token: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := time.Now().Add(RefreshTokenTTL)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, family_id)
		VALUES ($1, $2, $3, COALESCE($4::uuid, gen_random_uuid()))
	`, userID, hashToken(value), expiresAt, nullIfEmpty(familyID))
	if err != nil {
		return RefreshToken{}, fmt.Errorf("store refresh token: %w", err)
	}

	return RefreshToken{Value: value, ExpiresAt: expiresAt}, nil
}

// Consume atomically revokes the token and returns the user it belonged to and
// its family. A token can therefore only be redeemed once: unknown, expired,
// and already-used tokens are rejected with ErrInvalidRefreshToken. Replaying
// a token that was rotated earlier also revokes every token in its family, so
// a stolen token stops working for the thief and the victim alike.
func (s *RefreshStore) Consume(ctx context.Context, value string) (userID, familyID string, err error) {
	err = s.db.QueryRowContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id, family_id
	`, hashToken(value)).Scan(&userID, &familyID)
	if err == nil {
		return userID, familyID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("consume refresh token: %w", err)
	}

	// Not redeemable. If it was rotated a while ago, this is a replay.
	if _, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE revoked_at IS NULL AND family_id = (
			SELECT family_id FROM refresh_tokens
			WHERE token_hash = $1 AND revoked_at < now() - $2::interval
		)
	`, hashToken(value), fmt.Sprintf("%d seconds", int(reuseGrace.Seconds()))); err != nil {
		return "", "", fmt.Errorf("revoke reused refresh token family: %w", err)
	}
	return "", "", ErrInvalidRefreshToken
}

// Revoke invalidates a single refresh token, returning the user it belonged to.
// Unlike Consume it tolerates an already-expired token, so logging out with a
// stale token still succeeds.
func (s *RefreshStore) Revoke(ctx context.Context, value string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
		RETURNING user_id
	`, hashToken(value)).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidRefreshToken
		}
		return "", fmt.Errorf("revoke refresh token: %w", err)
	}
	return userID, nil
}

// RevokeAll invalidates every active refresh token for a user, logging them out
// of all devices.
func (s *RefreshStore) RevokeAll(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}

func nullIfEmpty(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
