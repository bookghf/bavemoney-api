package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var errNoFields = errors.New("no fields provided")

type CategoryRepository struct {
	db *sql.DB
}

func NewCategoryRepository(db *sql.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) ListSystem(ctx context.Context) ([]CategoryInfo, error) {
	query := `
		SELECT id, name, type, COALESCE(icon, ''), COALESCE(color, ''), is_system
		FROM categories WHERE is_system = true AND user_id IS NULL AND parent_id IS NULL
		ORDER BY name
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make(map[string]*CategoryInfo)
	var roots []*CategoryInfo

	for rows.Next() {
		var c CategoryInfo
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Icon, &c.Color, &c.IsSystem); err != nil {
			return nil, err
		}
		c.Subcategories = []CategoryInfo{}
		categories[c.ID] = &c
		roots = append(roots, &c)
	}

	var result []CategoryInfo
	for _, r := range roots {
		result = append(result, *r)
	}
	return result, nil
}

func (r *CategoryRepository) Create(ctx context.Context, req CreateCategoryRequest) (CategoryInfo, error) {
	var c CategoryInfo
	query := `
		INSERT INTO categories (name, type, icon, color, is_system, parent_id)
		VALUES ($1, $2, $3, $4, true, $5)
		RETURNING id, name, type, COALESCE(icon, ''), COALESCE(color, ''), is_system
	`
	err := r.db.QueryRowContext(ctx, query, req.Name, req.Type, req.Icon, req.Color, req.ParentID).
		Scan(&c.ID, &c.Name, &c.Type, &c.Icon, &c.Color, &c.IsSystem)
	return c, err
}

// Update applies name, icon, and color changes to a system category. It
// returns sql.ErrNoRows when no system category has that id.
func (r *CategoryRepository) Update(ctx context.Context, categoryID string, updates map[string]interface{}) error {
	var sets []string
	var args []interface{}
	for _, column := range []string{"name", "icon", "color"} {
		if value, ok := updates[column].(string); ok {
			args = append(args, value)
			sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
		}
	}
	if len(sets) == 0 {
		return errNoFields
	}

	args = append(args, categoryID)
	query := fmt.Sprintf(`UPDATE categories SET updated_at = now(), %s WHERE id = $%d AND is_system AND user_id IS NULL`,
		strings.Join(sets, ", "), len(args))
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes an unused system category, returning how many transactions
// still reference it (and deleting nothing) when it is in use.
func (r *CategoryRepository) Delete(ctx context.Context, categoryID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transactions t
		JOIN categories c ON c.id = t.category_id
		WHERE (c.id = $1 OR c.parent_id = $1) AND t.deleted_at IS NULL
	`, categoryID).Scan(&count)
	if err != nil || count > 0 {
		return count, err
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = $1 AND is_system AND user_id IS NULL`, categoryID)
	if err != nil {
		return 0, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return 0, sql.ErrNoRows
	}
	return 0, nil
}
