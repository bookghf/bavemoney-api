package account

import (
	"context"
	"database/sql"

	"ledger-api/internal/database"
)

const columns = `id, name, type, currency, initial_balance, is_archived, created_at`

// Repository reads and writes accounts owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's accounts, newest first.
func (r *Repository) List(ctx context.Context, userID string) ([]Account, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+`
		FROM accounts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []Account{}
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.Name, &account.Type, &account.Currency, &account.InitialBalance, &account.IsArchived, &account.CreatedAt); err != nil {
			return nil, err
		}
		account.CurrentBalance = account.InitialBalance
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// Get returns one account, or sql.ErrNoRows when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Account, error) {
	var account Account
	err := r.db.QueryRowContext(ctx, `
		SELECT `+columns+`
		FROM accounts
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&account.ID, &account.Name, &account.Type, &account.Currency, &account.InitialBalance, &account.IsArchived, &account.CreatedAt)
	if err != nil {
		return Account{}, err
	}
	account.CurrentBalance = account.InitialBalance
	return account, nil
}

// Create inserts an account and returns it with its generated id.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest) (Account, error) {
	account := Account{
		Name:           req.Name,
		Type:           req.Type,
		Currency:       req.Currency,
		InitialBalance: req.InitialBalance,
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO accounts (user_id, name, type, currency, initial_balance)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, userID, req.Name, req.Type, req.Currency, req.InitialBalance).Scan(&account.ID, &account.CreatedAt)
	if err != nil {
		return Account{}, err
	}
	account.CurrentBalance = account.InitialBalance
	return account, nil
}

// Update applies the non-nil fields of req.
func (r *Repository) Update(ctx context.Context, userID, id string, req UpdateRequest) error {
	update := database.NewUpdate("accounts")
	if req.Name != nil {
		update.Set("name", *req.Name)
	}
	if req.Type != nil {
		update.Set("type", *req.Type)
	}
	if req.Currency != nil {
		update.Set("currency", *req.Currency)
	}
	if req.InitialBalance != nil {
		update.Set("initial_balance", *req.InitialBalance)
	}
	if req.IsArchived != nil {
		update.Set("is_archived", *req.IsArchived)
	}

	query, args := update.Build(id, userID)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Archive soft-deletes an account by flagging it archived.
func (r *Repository) Archive(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE accounts SET is_archived = TRUE, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	return err
}
