package transaction

import (
	"context"
	"database/sql"

	"github.com/lib/pq"

	"ledger-api/internal/database"
)

const (
	columns = `id, account_id, category_id, type, amount, currency, note, tags, occurred_at, created_at, updated_at`

	// listLimit caps the un-paginated list endpoint.
	listLimit = 50
)

// Repository reads and writes transactions owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's most recent transactions, newest first.
func (r *Repository) List(ctx context.Context, userID string) ([]Transaction, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+`
		FROM transactions
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY occurred_at DESC
		LIMIT $2
	`, userID, listLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := []Transaction{}
	for rows.Next() {
		tx, err := scan(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}
	return transactions, rows.Err()
}

// Get returns one transaction, or sql.ErrNoRows when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Transaction, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+columns+`
		FROM transactions
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID)
	return scan(row)
}

// Create inserts a transaction and returns it with its generated id.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest) (Transaction, error) {
	tx := Transaction{
		AccountID:  req.AccountID,
		Type:       req.Type,
		Amount:     req.Amount,
		Currency:   req.Currency,
		Note:       req.Note,
		Tags:       req.Tags,
		OccurredAt: req.OccurredAt,
		AmountInDefaultCurrency: req.Amount,
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO transactions (user_id, account_id, category_id, type, amount, currency, note, tags, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, userID,
		req.AccountID,
		database.NullIfEmpty(req.CategoryID),
		req.Type,
		req.Amount,
		req.Currency,
		database.NullIfEmpty(req.Note),
		req.Tags,
		req.OccurredAt,
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

// Update applies the non-nil fields of req.
func (r *Repository) Update(ctx context.Context, userID, id string, req UpdateRequest) error {
	update := database.NewUpdate("transactions")
	if req.CategoryID != nil {
		update.Set("category_id", database.NullIfEmpty(*req.CategoryID))
	}
	if req.Type != nil {
		update.Set("type", *req.Type)
	}
	if req.Amount != nil {
		update.Set("amount", *req.Amount)
	}
	if req.Note != nil {
		update.Set("note", database.NullIfEmpty(*req.Note))
	}
	if req.OccurredAt != nil {
		update.Set("occurred_at", *req.OccurredAt)
	}
	if len(req.Tags) > 0 {
		update.Set("tags", req.Tags)
	}

	query, args := update.Build(id, userID)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Delete soft-deletes a transaction owned by the user.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE transactions SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	return err
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scan(src scanner) (Transaction, error) {
	var (
		tx               Transaction
		categoryID, note sql.NullString
		tags             pq.StringArray
	)
	if err := src.Scan(&tx.ID, &tx.AccountID, &categoryID, &tx.Type, &tx.Amount, &tx.Currency, &note, &tags, &tx.OccurredAt, &tx.CreatedAt, &tx.UpdatedAt); err != nil {
		return Transaction{}, err
	}
	tx.Note = note.String
	tx.Tags = tags
	if tx.Tags == nil {
		tx.Tags = []string{}
	}
	tx.Attachments = []Attachment{}
	tx.AmountInDefaultCurrency = tx.Amount

	if categoryID.Valid {
		tx.Category = &CategoryInfo{ID: categoryID.String}
	}
	return tx, nil
}
