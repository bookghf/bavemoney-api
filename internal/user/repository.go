package user

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Repository reads and writes user records.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts an active user and returns it with its generated id.
func (r *Repository) Create(ctx context.Context, email, passwordHash, displayName, defaultCurrency string) (User, error) {
	user := User{Email: email, DisplayName: displayName, DefaultCurrency: defaultCurrency, MonthStartDay: 1, Status: "active"}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, display_name, default_currency, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, created_at
	`, email, passwordHash, displayName, defaultCurrency).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// ByEmail returns the user with the given email along with its password hash.
// It returns sql.ErrNoRows when no such user exists.
func (r *Repository) ByEmail(ctx context.Context, email string) (User, string, error) {
	var (
		user        = User{Email: email}
		hash        string
		displayName sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, password_hash, display_name, default_currency, month_start_day, status, created_at
		FROM users
		WHERE lower(email) = $1
		ORDER BY created_at
		LIMIT 1
	`, email).Scan(&user.ID, &hash, &displayName, &user.DefaultCurrency, &user.MonthStartDay, &user.Status, &user.CreatedAt)
	if err != nil {
		return User{}, "", err
	}
	user.DisplayName = displayName.String
	return user, hash, nil
}

// ByID returns the user with the given id, or sql.ErrNoRows when it does not
// exist.
func (r *Repository) ByID(ctx context.Context, id string) (User, error) {
	var (
		user        = User{ID: id}
		displayName sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT email, display_name, default_currency, month_start_day, status, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&user.Email, &displayName, &user.DefaultCurrency, &user.MonthStartDay, &user.Status, &user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	user.DisplayName = displayName.String
	return user, nil
}

// CurrencyExists reports whether code is an active currency.
func (r *Repository) CurrencyExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM currencies WHERE code = $1 AND is_active)`, code).Scan(&exists)
	return exists, err
}

// ErrNotFound is returned when the user row no longer exists.
var ErrNotFound = errors.New("user not found")

// UpdateProfile applies the non-nil fields and returns the updated user.
func (r *Repository) UpdateProfile(ctx context.Context, id string, req UpdateProfileRequest) (User, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users SET
			display_name = COALESCE($2, display_name),
			default_currency = COALESCE($3, default_currency),
			month_start_day = COALESCE($4, month_start_day),
			updated_at = now()
		WHERE id = $1
	`, id, req.DisplayName, req.DefaultCurrency, req.MonthStartDay)
	if err != nil {
		return User{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return User{}, ErrNotFound
	}
	return r.ByID(ctx, id)
}

// PasswordHash returns the user's bcrypt hash.
func (r *Repository) PasswordHash(ctx context.Context, id string) (string, error) {
	var hash sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, id).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return hash.String, err
}

// SetPassword stores a new bcrypt hash.
func (r *Repository) SetPassword(ctx context.Context, id, hash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	return err
}

// ResetData deletes all of the user's ledger data in one database transaction,
// keeping the user row and sessions. Children go first: transactions reference
// accounts and categories without ON DELETE CASCADE; attachments cascade with
// their transactions.
func (r *Repository) ResetData(ctx context.Context, id string) (ResetCounts, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ResetCounts{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var counts ResetCounts
	// Report what the user could see: soft-deleted rows are purged too, but
	// they were already gone from the app.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE user_id = $1 AND deleted_at IS NULL`, id).
		Scan(&counts.Transactions); err != nil {
		return ResetCounts{}, err
	}
	steps := []struct {
		query string
		count *int64
	}{
		{`DELETE FROM transactions WHERE user_id = $1`, nil},
		{`DELETE FROM budgets WHERE user_id = $1`, &counts.Budgets},
		{`DELETE FROM accounts WHERE user_id = $1`, &counts.Accounts},
		{`DELETE FROM categories WHERE user_id = $1 AND parent_id IS NOT NULL`, &counts.Categories},
		{`DELETE FROM categories WHERE user_id = $1`, nil},
	}
	for _, step := range steps {
		result, err := tx.ExecContext(ctx, step.query, id)
		if err != nil {
			return ResetCounts{}, err
		}
		n, _ := result.RowsAffected()
		switch {
		case step.count != nil:
			*step.count = n
		case strings.Contains(step.query, "categories"):
			counts.Categories += n
		}
	}
	return counts, tx.Commit()
}

// Delete removes the user. Every table that belongs to a user cascades from
// the user row, so this also deletes their ledger data and sessions.
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

// SaveResetCode stores a new reset code for the user, replacing any earlier
// one, unless the last code is younger than cooldown. It reports whether the
// code was saved (and so should be sent).
func (r *Repository) SaveResetCode(ctx context.Context, userID, codeHash string, ttl, cooldown time.Duration) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO password_reset_codes (user_id, code_hash, expires_at)
		VALUES ($1, $2, now() + $3 * interval '1 second')
		ON CONFLICT (user_id) DO UPDATE
		SET code_hash = EXCLUDED.code_hash, expires_at = EXCLUDED.expires_at, attempts = 0, created_at = now()
		WHERE password_reset_codes.created_at < now() - $4 * interval '1 second'
	`, userID, codeHash, ttl.Seconds(), cooldown.Seconds())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

// UseResetCodeAttempt counts one try at the user's reset code and returns its
// hash. It returns sql.ErrNoRows when there is no code, it has expired, or its
// tries are used up.
func (r *Repository) UseResetCodeAttempt(ctx context.Context, userID string, maxAttempts int) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, `
		UPDATE password_reset_codes SET attempts = attempts + 1
		WHERE user_id = $1 AND expires_at > now() AND attempts < $2
		RETURNING code_hash
	`, userID, maxAttempts).Scan(&hash)
	return hash, err
}

// ResetPassword stores the new password hash and deletes the used reset code
// in one database transaction.
func (r *Repository) ResetPassword(ctx context.Context, userID, hash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, hash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}
