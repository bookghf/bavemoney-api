package admin

import (
	"context"
	"database/sql"
)

type CategoryRepository struct {
	db *sql.DB
}

func NewCategoryRepository(db *sql.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) ListSystem(ctx context.Context) ([]CategoryInfo, error) {
	query := `
		SELECT id, name, type, icon, color, is_system
		FROM categories WHERE is_system = true AND user_id IS NULL
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
		RETURNING id, name, type, icon, color, is_system
	`
	err := r.db.QueryRowContext(ctx, query, req.Name, req.Type, req.Icon, req.Color, req.ParentID).
		Scan(&c.ID, &c.Name, &c.Type, &c.Icon, &c.Color, &c.IsSystem)
	return c, err
}

func (r *CategoryRepository) Update(ctx context.Context, categoryID string, updates map[string]interface{}) error {
	query := `UPDATE categories SET `
	args := []interface{}{}
	idx := 1

	if name, ok := updates["name"].(string); ok {
		query += `name = $` + string(rune(idx))
		args = append(args, name)
		idx++
	}
	if icon, ok := updates["icon"].(string); ok {
		if len(args) > 0 {
			query += `, `
		}
		query += `icon = $` + string(rune(idx))
		args = append(args, icon)
		idx++
	}
	if color, ok := updates["color"].(string); ok {
		if len(args) > 0 {
			query += `, `
		}
		query += `color = $` + string(rune(idx))
		args = append(args, color)
		idx++
	}

	query += ` WHERE id = $` + string(rune(idx))
	args = append(args, categoryID)

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *CategoryRepository) Delete(ctx context.Context, categoryID string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM transactions WHERE category_id = $1`
	err := r.db.QueryRowContext(ctx, query, categoryID).Scan(&count)
	if err != nil || count > 0 {
		return count, err
	}

	delQuery := `DELETE FROM categories WHERE id = $1`
	_, err = r.db.ExecContext(ctx, delQuery, categoryID)
	return 0, err
}
