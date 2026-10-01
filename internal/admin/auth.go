package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

type AdminRepository struct {
	db *sql.DB
}

func NewAdminRepository(db *sql.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

func (r *AdminRepository) GetByEmail(ctx context.Context, email string) (AdminUser, string, error) {
	var admin AdminUser
	var hash string
	query := `SELECT id, email, role, password_hash, created_at FROM admin_users WHERE lower(email) = $1`
	err := r.db.QueryRowContext(ctx, query, email).Scan(&admin.ID, &admin.Email, &admin.Role, &hash, &admin.CreatedAt)
	return admin, hash, err
}

func (r *AdminRepository) GetByID(ctx context.Context, adminID string) (AdminUser, error) {
	var admin AdminUser
	query := `SELECT id, email, role, created_at FROM admin_users WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, adminID).Scan(&admin.ID, &admin.Email, &admin.Role, &admin.CreatedAt)
	return admin, err
}

func (r *AdminRepository) LogAction(ctx context.Context, adminID string, action, targetTable string, targetID, detail interface{}) error {
	query := `INSERT INTO admin_audit_log (admin_id, action, target_table, target_id, detail) VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query, adminID, action, targetTable, targetID, detail)
	return err
}

func (r *AdminRepository) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (r *AdminRepository) VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
