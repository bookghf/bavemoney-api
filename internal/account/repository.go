package account

import (
	"context"
	"database/sql"
	"errors"

	"ledger-api/internal/database"
)

// columns includes current_balance: initial balance plus income, minus
// expense, minus transfers out, plus transfers in, across the account's live
// transactions. Money is read as text so it reaches clients exactly.
const columns = `id, name, type, currency, initial_balance::text,
	(initial_balance + COALESCE((
		SELECT SUM(CASE
			WHEN t.to_account_id = accounts.id THEN t.amount
			WHEN t.type = 'income' THEN t.amount
			WHEN t.type IN ('expense', 'transfer') THEN -t.amount
			ELSE 0
		END)
		FROM transactions t
		WHERE (t.account_id = accounts.id OR t.to_account_id = accounts.id) AND t.deleted_at IS NULL
	), 0))::text AS current_balance,
	is_archived, created_at`

// Errors surfaced to the caller.
var (
	ErrNotFound        = errors.New("account not found")
	ErrUnknownCurrency = errors.New("currency is not supported")
	ErrCurrencyLocked  = errors.New("currency can not change once the account has transactions")
)

// Repository reads and writes accounts owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's accounts, newest first. Archived accounts are only
// included when includeArchived is set.
func (r *Repository) List(ctx context.Context, userID string, includeArchived bool) ([]Account, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+`
		FROM accounts
		WHERE user_id = $1 AND ($2 OR NOT is_archived)
		ORDER BY created_at DESC
	`, userID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []Account{}
	for rows.Next() {
		account, err := scan(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// Get returns one account, or ErrNotFound when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Account, error) {
	account, err := scan(r.db.QueryRowContext(ctx, `
		SELECT `+columns+`
		FROM accounts
		WHERE id = $1 AND user_id = $2
	`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return account, err
}

// Create inserts an account and returns it.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest) (Account, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO accounts (user_id, name, type, currency, initial_balance)
		SELECT $1, $2, $3, c.code, $5::numeric
		FROM currencies c WHERE c.code = $4 AND c.is_active
		RETURNING id
	`, userID, req.Name, req.Type, req.Currency, string(req.InitialBalance)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrUnknownCurrency
	}
	if err != nil {
		return Account{}, err
	}
	return r.Get(ctx, userID, id)
}

// Update applies the non-nil fields of req. Changing the currency is only
// allowed while the account has no transactions, since existing amounts were
// recorded in the old currency.
func (r *Repository) Update(ctx context.Context, userID string, existing Account, req UpdateRequest) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	update := database.NewUpdate("accounts")
	if req.Name != nil {
		update.Set("name", *req.Name)
	}
	if req.Type != nil {
		update.Set("type", *req.Type)
	}
	if req.Currency != nil && *req.Currency != existing.Currency {
		var used, known bool
		if err := tx.QueryRowContext(ctx, `
			SELECT
				EXISTS (SELECT 1 FROM transactions WHERE (account_id = $1 OR to_account_id = $1) AND deleted_at IS NULL),
				EXISTS (SELECT 1 FROM currencies WHERE code = $2 AND is_active)
		`, existing.ID, *req.Currency).Scan(&used, &known); err != nil {
			return err
		}
		if used {
			return ErrCurrencyLocked
		}
		if !known {
			return ErrUnknownCurrency
		}
		update.Set("currency", *req.Currency)
	}
	if req.InitialBalance != nil {
		update.Set("initial_balance", string(*req.InitialBalance))
	}
	if req.IsArchived != nil {
		update.Set("is_archived", *req.IsArchived)
	}
	if update.Empty() {
		return nil
	}

	query, args := update.Build(existing.ID, userID)
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// Archive soft-deletes an account by flagging it archived, returning
// ErrNotFound when the user does not own it.
func (r *Repository) Archive(ctx context.Context, userID, id string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE accounts SET is_archived = TRUE, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scan(src scanner) (Account, error) {
	var a Account
	err := src.Scan(&a.ID, &a.Name, &a.Type, &a.Currency, &a.InitialBalance, &a.CurrentBalance, &a.IsArchived, &a.CreatedAt)
	return a, err
}
