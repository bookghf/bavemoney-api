package report

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetSummary(ctx context.Context, userID, period, date, accountID, currency string) (SummaryResponse, error) {
	startDate, endDate := r.getPeriodRange(period, date)

	summary := SummaryResponse{
		Period:    period,
		StartDate: startDate.Format("2006-01-02"),
		EndDate:   endDate.Format("2006-01-02"),
		Currency:  currency,
	}

	query := `
		SELECT
			COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0) as total_income,
			COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0) as total_expense
		FROM transactions
		WHERE user_id = $1
			AND occurred_at::date >= $2
			AND occurred_at::date <= $3
			AND deleted_at IS NULL
	`

	args := []interface{}{userID, startDate, endDate}

	if accountID != "" {
		query += " AND account_id = $4"
		args = append(args, accountID)
	}

	var income, expense sql.NullString
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&income, &expense)
	if err != nil && err != sql.ErrNoRows {
		return SummaryResponse{}, err
	}

	summary.TotalIncome = income.String
	if summary.TotalIncome == "" {
		summary.TotalIncome = "0.00"
	}
	summary.TotalExpense = expense.String
	if summary.TotalExpense == "" {
		summary.TotalExpense = "0.00"
	}
	summary.Net = fmt.Sprintf("%.2f", 0)

	return summary, nil
}

func (r *Repository) getPeriodRange(period, dateStr string) (time.Time, time.Time) {
	t, _ := time.Parse("2006-01-02", dateStr)

	switch period {
	case "week":
		weekday := t.Weekday()
		offset := int(weekday)
		startDate := t.AddDate(0, 0, -offset)
		endDate := startDate.AddDate(0, 0, 6)
		return startDate, endDate
	case "month":
		year, month, _ := t.Date()
		startDate := time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
		endDate := startDate.AddDate(0, 1, -1)
		return startDate, endDate
	case "year":
		year := t.Year()
		startDate := time.Date(year, 1, 1, 0, 0, 0, 0, t.Location())
		endDate := time.Date(year, 12, 31, 0, 0, 0, 0, t.Location())
		return startDate, endDate
	default:
		return t, t
	}
}
