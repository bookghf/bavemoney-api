package admin

import (
	"context"
	"database/sql"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) List(ctx context.Context, page, limit int, status, search, sort string) ([]UserInfo, int, error) {
	offset := (page - 1) * limit

	countQuery := `SELECT COUNT(*) FROM users WHERE 1=1`
	args := []interface{}{}

	if status != "" {
		countQuery += ` AND status = $1`
		args = append(args, status)
	}
	if search != "" {
		countQuery += ` AND (email ILIKE $2 OR display_name ILIKE $2)`
		args = append(args, "%"+search+"%")
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, email, display_name, default_currency, status,
		       COALESCE((SELECT COUNT(*) FROM transactions WHERE user_id = users.id), 0) as transaction_count,
		       updated_at, created_at
		FROM users WHERE 1=1
	`

	args = []interface{}{}
	if status != "" {
		query += ` AND status = $1`
		args = append(args, status)
	}
	if search != "" {
		query += ` AND (email ILIKE $2 OR display_name ILIKE $2)`
		args = append(args, "%"+search+"%")
	}

	query += ` ORDER BY `
	switch sort {
	case "email":
		query += `email`
	case "display_name":
		query += `display_name`
	default:
		query += `created_at DESC`
	}

	query += ` LIMIT $` + "$" + `3 OFFSET $4`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []UserInfo
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.DefaultCurrency, &u.Status,
			&u.TransactionCount, &u.LastActiveAt, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}

	return users, total, nil
}

func (r *UserRepository) GetByID(ctx context.Context, userID string) (UserDetailResponse, error) {
	var detail UserDetail
	query := `
		SELECT id, email, display_name, avatar_url, default_currency, status, google_id, created_at, updated_at
		FROM users WHERE id = $1
	`
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&detail.ID, &detail.Email, &detail.DisplayName, &detail.AvatarURL,
		&detail.DefaultCurrency, &detail.Status, &detail.GoogleID,
		&detail.CreatedAt, &detail.UpdatedAt,
	)
	if err != nil {
		return UserDetailResponse{}, err
	}

	var stats UserStats
	statsQuery := `
		SELECT
			COALESCE((SELECT COUNT(*) FROM accounts WHERE user_id = $1), 0),
			COALESCE((SELECT COUNT(*) FROM transactions WHERE user_id = $1), 0),
			COALESCE((SELECT COUNT(*) FROM budgets WHERE user_id = $1), 0),
			COALESCE(MAX(occurred_at), '1970-01-01') FROM transactions WHERE user_id = $1
	`
	err = r.db.QueryRowContext(ctx, statsQuery, userID).Scan(
		&stats.TotalAccounts, &stats.TotalTransactions, &stats.TotalBudgets, &stats.LastTransactionAt,
	)
	if err != nil {
		return UserDetailResponse{}, err
	}

	return UserDetailResponse{User: detail, Stats: stats}, nil
}

func (r *UserRepository) Suspend(ctx context.Context, userID, status string) error {
	query := `UPDATE users SET status = $1, updated_at = now() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, userID)
	return err
}
