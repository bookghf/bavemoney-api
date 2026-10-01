package attachment

import (
	"context"
	"database/sql"
)

// owned limits a query to attachments of a live transaction owned by the
// user. Every attachment query goes through it, so an attachment id from
// another transaction or another user never matches.
const owned = `
	a.transaction_id = $2
	AND EXISTS (SELECT 1 FROM transactions t WHERE t.id = a.transaction_id AND t.user_id = $1 AND t.deleted_at IS NULL)`

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Create adds a pending attachment to a transaction the user owns, returning
// sql.ErrNoRows when they do not own it.
func (r *Repository) Create(ctx context.Context, userID, transactionID string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO attachments (transaction_id, file_url)
		SELECT t.id, '' FROM transactions t
		WHERE t.id = $2 AND t.user_id = $1 AND t.deleted_at IS NULL
		RETURNING id
	`, userID, transactionID).Scan(&id)
	return id, err
}

// Confirm records the uploaded file's URL, returning sql.ErrNoRows when the
// attachment is not on that transaction or the user does not own it.
func (r *Repository) Confirm(ctx context.Context, userID, transactionID, attachmentID, fileURL string) (Attachment, error) {
	var att Attachment
	err := r.db.QueryRowContext(ctx, `
		UPDATE attachments a SET file_url = $4
		WHERE a.id = $3 AND`+owned+`
		RETURNING a.id, a.transaction_id, a.file_url, COALESCE(a.file_size_bytes, 0), a.uploaded_at
	`, userID, transactionID, attachmentID, fileURL).
		Scan(&att.ID, &att.TransactionID, &att.FileURL, &att.FileSizeBytes, &att.UploadedAt)
	return att, err
}

// Delete removes an attachment, returning sql.ErrNoRows when it is not on that
// transaction or the user does not own it.
func (r *Repository) Delete(ctx context.Context, userID, transactionID, attachmentID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM attachments a WHERE a.id = $3 AND`+owned,
		userID, transactionID, attachmentID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// VerifyOwnership reports whether the user owns the live transaction.
func (r *Repository) VerifyOwnership(ctx context.Context, userID, transactionID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM transactions WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)
	`, transactionID, userID).Scan(&exists)
	return exists, err
}
