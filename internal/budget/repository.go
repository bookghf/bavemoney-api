package budget

import (
	"context"
	"database/sql"

	"ledger-api/internal/database"
)

const columns = `id, category_id, amount, currency, period, start_date, alert_threshold_pct, created_at`

// Repository reads and writes budgets owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's budgets, most recent period first.
func (r *Repository) List(ctx context.Context, userID string) ([]Budget, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+`
		FROM budgets
		WHERE user_id = $1
		ORDER BY start_date DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	budgets := []Budget{}
	for rows.Next() {
		b, err := scan(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}

// Get returns one budget, or sql.ErrNoRows when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Budget, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+columns+`
		FROM budgets
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	return scan(row)
}

// Create inserts a budget and returns it with its generated id.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest) (Budget, error) {
	b := Budget{
		Amount:            req.Amount,
		Currency:          req.Currency,
		Period:            req.Period,
		StartDate:         req.StartDate,
		AlertThresholdPct: req.AlertThresholdPct,
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO budgets (user_id, category_id, amount, currency, period, start_date, alert_threshold_pct)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, userID,
		database.NullIfEmpty(req.CategoryID),
		req.Amount,
		req.Currency,
		req.Period,
		req.StartDate,
		req.AlertThresholdPct,
	).Scan(&b.ID, &b.CreatedAt)
	if err != nil {
		return Budget{}, err
	}
	b.CurrentSpend = "0.00"
	b.Remaining = req.Amount
	b.PercentUsed = 0
	b.IsOverBudget = false
	return b, nil
}

// Update applies the non-nil fields of req.
func (r *Repository) Update(ctx context.Context, userID, id string, req UpdateRequest) error {
	update := database.NewUpdate("budgets")
	if req.Amount != nil {
		update.Set("amount", *req.Amount)
	}
	if req.Period != nil {
		update.Set("period", *req.Period)
	}
	if req.AlertThresholdPct != nil {
		update.Set("alert_threshold_pct", *req.AlertThresholdPct)
	}

	query, args := update.Build(id, userID)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Delete removes a budget owned by the user.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM budgets WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scan(src scanner) (Budget, error) {
	var (
		b          Budget
		categoryID sql.NullString
	)
	if err := src.Scan(&b.ID, &categoryID, &b.Amount, &b.Currency, &b.Period, &b.StartDate, &b.AlertThresholdPct, &b.CreatedAt); err != nil {
		return Budget{}, err
	}
	if categoryID.Valid {
		b.Category = &CategoryInfo{ID: categoryID.String}
	}
	b.CurrentSpend = "0.00"
	b.Remaining = b.Amount
	b.PercentUsed = 0
	b.IsOverBudget = false
	return b, nil
}
