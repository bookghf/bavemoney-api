package admin

import (
	"context"
	"database/sql"
	"fmt"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) List(ctx context.Context, page, limit int, status, search, sort string) ([]UserInfo, int, error) {
	where := " WHERE 1=1"
	args := []interface{}{}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		where += fmt.Sprintf(" AND (email ILIKE $%[1]d OR display_name ILIKE $%[1]d)", len(args))
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "created_at DESC"
	switch sort {
	case "email":
		order = "email"
	case "display_name":
		order = "display_name"
	case "created_at":
		order = "created_at"
	}

	args = append(args, limit, (page-1)*limit)
	query := fmt.Sprintf(`
		SELECT id, email, COALESCE(display_name, ''), default_currency, status,
		       (SELECT COUNT(*) FROM transactions WHERE user_id = users.id AND deleted_at IS NULL),
		       updated_at, created_at
		FROM users%s
		ORDER BY %s, id
		LIMIT $%d OFFSET $%d`, where, order, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := []UserInfo{}
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.DefaultCurrency, &u.Status,
			&u.TransactionCount, &u.LastActiveAt, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *UserRepository) GetByID(ctx context.Context, userID string) (UserDetailResponse, error) {
	var detail UserDetail
	query := `
		SELECT id, email, COALESCE(display_name, ''), COALESCE(avatar_url, ''), default_currency, status,
		       COALESCE(google_id, ''), created_at, updated_at
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
			COALESCE((SELECT COUNT(*) FROM transactions WHERE user_id = $1 AND deleted_at IS NULL), 0),
			COALESCE((SELECT COUNT(*) FROM budgets WHERE user_id = $1), 0),
			COALESCE((SELECT MAX(occurred_at)::text FROM transactions WHERE user_id = $1 AND deleted_at IS NULL), '')
	`
	err = r.db.QueryRowContext(ctx, statsQuery, userID).Scan(
		&stats.TotalAccounts, &stats.TotalTransactions, &stats.TotalBudgets, &stats.LastTransactionAt,
	)
	if err != nil {
		return UserDetailResponse{}, err
	}

	return UserDetailResponse{User: detail, Stats: stats}, nil
}

// Suspend sets the user's status. Suspending also revokes every refresh token,
// so the user is signed out once their short-lived access token expires.
// It returns sql.ErrNoRows when the user does not exist.
func (r *UserRepository) Suspend(ctx context.Context, userID, status string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE users SET status = $1, updated_at = now() WHERE id = $2`, status, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if status != "active" {
		if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
