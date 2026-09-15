package user

import (
	"context"
	"database/sql"
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
	user := User{Email: email, DisplayName: displayName, DefaultCurrency: defaultCurrency, Status: "active"}
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
		SELECT id, password_hash, display_name, default_currency, status, created_at
		FROM users
		WHERE email = $1
	`, email).Scan(&user.ID, &hash, &displayName, &user.DefaultCurrency, &user.Status, &user.CreatedAt)
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
		SELECT email, display_name, default_currency, status, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&user.Email, &displayName, &user.DefaultCurrency, &user.Status, &user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	user.DisplayName = displayName.String
	return user, nil
}
