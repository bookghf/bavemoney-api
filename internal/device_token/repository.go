package device_token

import (
	"context"
	"database/sql"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(ctx context.Context, userID, fcmToken, platform string) (DeviceToken, error) {
	query := `
		INSERT INTO device_tokens (user_id, fcm_token, platform)
		VALUES ($1, $2, $3)
		ON CONFLICT (fcm_token) DO UPDATE SET platform = EXCLUDED.platform, user_id = EXCLUDED.user_id
		RETURNING id, fcm_token, platform, created_at
	`
	var token DeviceToken
	err := r.db.QueryRowContext(ctx, query, userID, fcmToken, platform).
		Scan(&token.ID, &token.FCMToken, &token.Platform, &token.CreatedAt)
	if err != nil {
		return DeviceToken{}, err
	}
	return token, nil
}

func (r *Repository) Delete(ctx context.Context, userID, fcmToken string) error {
	query := `DELETE FROM device_tokens WHERE user_id = $1 AND fcm_token = $2`
	_, err := r.db.ExecContext(ctx, query, userID, fcmToken)
	return err
}
