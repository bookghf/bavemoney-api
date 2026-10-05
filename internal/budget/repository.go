package budget

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ledger-api/internal/database"
)

const columns = `b.id, b.category_id, c.name, c.icon, c.color, b.amount::text, b.currency, b.period, b.start_date::text, b.alert_threshold_pct, b.created_at, u.month_start_day`

const from = `
	FROM budgets b
	JOIN users u ON u.id = b.user_id
	LEFT JOIN categories c ON c.id = b.category_id`

// Errors surfaced to the caller.
var (
	ErrNotFound         = errors.New("budget not found")
	ErrCategoryNotFound = errors.New("category not found")
	ErrCategoryType     = errors.New("budgets can only track expense categories")
	ErrUnknownCurrency  = errors.New("currency is not supported")
	ErrDuplicate        = errors.New("a budget for this category and period already exists")
)

// Repository reads and writes budgets owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's budgets with progress as of today in loc, optionally
// limited to one period.
func (r *Repository) List(ctx context.Context, userID, period string, loc *time.Location) ([]Budget, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+from+`
		WHERE b.user_id = $1 AND ($2 = '' OR b.period = $2)
		ORDER BY b.category_id IS NOT NULL, c.name, b.created_at
	`, userID, period)
	if err != nil {
		return nil, err
	}
	budgets := []Budget{}
	for rows.Next() {
		b, err := scan(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		budgets = append(budgets, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range budgets {
		if err := r.fillProgress(ctx, userID, &budgets[i], loc); err != nil {
			return nil, err
		}
	}
	return budgets, nil
}

// Get returns one budget with its progress, or ErrNotFound.
func (r *Repository) Get(ctx context.Context, userID, id string, loc *time.Location) (Budget, error) {
	b, err := scan(r.db.QueryRowContext(ctx, `SELECT `+columns+from+` WHERE b.id = $1 AND b.user_id = $2`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Budget{}, ErrNotFound
	}
	if err != nil {
		return Budget{}, err
	}
	return b, r.fillProgress(ctx, userID, &b, loc)
}

// fillProgress computes spend in the current period: expenses in the budget's
// currency, in its category (subcategories included) or in every category for
// an overall budget. Money stays NUMERIC until it is rendered.
func (r *Repository) fillProgress(ctx context.Context, userID string, b *Budget, loc *time.Location) error {
	start, err := time.Parse(time.DateOnly, b.StartDate)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	windowFrom, windowTo := currentWindow(start, b.Period, today, b.monthStartDay)
	b.PeriodStart = windowFrom.Format(time.DateOnly)
	b.PeriodEnd = windowTo.AddDate(0, 0, -1).Format(time.DateOnly)

	var categoryID interface{}
	if b.Category != nil {
		categoryID = b.Category.ID
	}
	// Window days are calendar days in loc.
	fromInstant := time.Date(windowFrom.Year(), windowFrom.Month(), windowFrom.Day(), 0, 0, 0, 0, loc)
	toInstant := time.Date(windowTo.Year(), windowTo.Month(), windowTo.Day(), 0, 0, 0, 0, loc)

	return r.db.QueryRowContext(ctx, `
		WITH spend AS (
			SELECT COALESCE(SUM(t.amount), 0) AS total
			FROM transactions t
			LEFT JOIN categories c ON c.id = t.category_id
			WHERE t.user_id = $1 AND t.deleted_at IS NULL AND t.type = 'expense'
			  AND t.currency = $2
			  AND t.occurred_at >= $3 AND t.occurred_at < $4
			  AND ($5::uuid IS NULL OR t.category_id = $5::uuid OR c.parent_id = $5::uuid)
		)
		SELECT total::text, ($6::numeric - total)::text,
			COALESCE(round(total / NULLIF($6::numeric, 0) * 100, 2), 0)::float8, total > $6::numeric
		FROM spend
	`, userID, b.Currency, fromInstant, toInstant, categoryID, b.Amount).
		Scan(&b.CurrentSpend, &b.Remaining, &b.PercentUsed, &b.IsOverBudget)
}

// Create inserts a budget after checking its category and currency.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest, loc *time.Location) (Budget, error) {
	var known bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM currencies WHERE code = $1 AND is_active)`, req.Currency).Scan(&known); err != nil {
		return Budget{}, err
	}
	if !known {
		return Budget{}, ErrUnknownCurrency
	}
	if req.CategoryID != "" {
		var categoryType string
		err := r.db.QueryRowContext(ctx, `
			SELECT type FROM categories WHERE id = $1 AND (user_id = $2 OR (user_id IS NULL AND is_system))
		`, req.CategoryID, userID).Scan(&categoryType)
		if errors.Is(err, sql.ErrNoRows) {
			return Budget{}, ErrCategoryNotFound
		}
		if err != nil {
			return Budget{}, err
		}
		if categoryType != "expense" {
			return Budget{}, ErrCategoryType
		}
	}

	var duplicate bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM budgets WHERE user_id = $1 AND category_id IS NOT DISTINCT FROM $2::uuid AND period = $3)
	`, userID, database.NullIfEmpty(req.CategoryID), req.Period).Scan(&duplicate); err != nil {
		return Budget{}, err
	}
	if duplicate {
		return Budget{}, ErrDuplicate
	}

	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO budgets (user_id, category_id, amount, currency, period, start_date, alert_threshold_pct)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, userID, database.NullIfEmpty(req.CategoryID), string(req.Amount), req.Currency, req.Period, req.StartDate, req.AlertThresholdPct).Scan(&id)
	if err != nil {
		return Budget{}, err
	}
	return r.Get(ctx, userID, id, loc)
}

// Update applies the non-nil fields of req, returning ErrNotFound when the
// user has no such budget.
func (r *Repository) Update(ctx context.Context, userID, id string, req UpdateRequest) error {
	update := database.NewUpdate("budgets")
	if req.Amount != nil {
		update.Set("amount", string(*req.Amount))
	}
	if req.Period != nil {
		update.Set("period", *req.Period)
	}
	if req.AlertThresholdPct != nil {
		update.Set("alert_threshold_pct", *req.AlertThresholdPct)
	}

	query, args := update.Build(id, userID)
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a budget owned by the user, returning ErrNotFound when there
// is none.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM budgets WHERE id = $1 AND user_id = $2`, id, userID)
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

func scan(src scanner) (Budget, error) {
	var (
		b                                     Budget
		categoryID, categoryName, icon, color sql.NullString
	)
	if err := src.Scan(&b.ID, &categoryID, &categoryName, &icon, &color, &b.Amount, &b.Currency, &b.Period, &b.StartDate, &b.AlertThresholdPct, &b.CreatedAt, &b.monthStartDay); err != nil {
		return Budget{}, err
	}
	if categoryID.Valid {
		b.Category = &CategoryInfo{ID: categoryID.String, Name: categoryName.String, Icon: icon.String, Color: color.String}
	}
	return b, nil
}
