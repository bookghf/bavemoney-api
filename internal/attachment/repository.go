package attachment

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

func (r *Repository) Create(ctx context.Context, transactionID string) (string, error) {
	var id string
	query := `INSERT INTO attachments (transaction_id, file_url) VALUES ($1, '') RETURNING id`
	err := r.db.QueryRowContext(ctx, query, transactionID).Scan(&id)
	return id, err
}

func (r *Repository) UpdateURL(ctx context.Context, attachmentID, fileURL string, fileSizeBytes int) error {
	query := `UPDATE attachments SET file_url = $1, file_size_bytes = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, fileURL, fileSizeBytes, attachmentID)
	return err
}

func (r *Repository) Get(ctx context.Context, attachmentID string) (Attachment, error) {
	var att Attachment
	query := `SELECT id, transaction_id, file_url, file_size_bytes, uploaded_at FROM attachments WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, attachmentID).
		Scan(&att.ID, &att.TransactionID, &att.FileURL, &att.FileSizeBytes, &att.UploadedAt)
	return att, err
}

func (r *Repository) Delete(ctx context.Context, attachmentID string) error {
	query := `DELETE FROM attachments WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, attachmentID)
	return err
}

func (r *Repository) ListByTransaction(ctx context.Context, transactionID string) ([]Attachment, error) {
	query := `SELECT id, transaction_id, file_url, file_size_bytes, uploaded_at FROM attachments WHERE transaction_id = $1 ORDER BY uploaded_at DESC`
	rows, err := r.db.QueryContext(ctx, query, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var att Attachment
		if err := rows.Scan(&att.ID, &att.TransactionID, &att.FileURL, &att.FileSizeBytes, &att.UploadedAt); err != nil {
			return nil, err
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func (r *Repository) VerifyOwnership(ctx context.Context, userID, transactionID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM transactions WHERE id = $1 AND user_id = $2)`
	err := r.db.QueryRowContext(ctx, query, transactionID, userID).Scan(&exists)
	return exists, err
}
