package category

import (
	"context"
	"database/sql"
	"errors"

	"ledger-api/internal/database"
)

const columns = `id, parent_id, name, type, icon, color`

// Parent validation failures surfaced to the caller as 400s.
var (
	ErrParentNotFound    = errors.New("parent category not found")
	ErrParentNotTopLevel = errors.New("parent category must be a top-level category")
)

// Repository reads and writes categories owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's categories as a flat slice ordered by name.
func (r *Repository) List(ctx context.Context, userID string) ([]Category, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+`
		FROM categories
		WHERE user_id = $1
		ORDER BY name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := []Category{}
	for rows.Next() {
		category, err := scan(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

// Get returns one category, or sql.ErrNoRows when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Category, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+columns+`
		FROM categories
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	return scan(row)
}

// Create inserts a category and returns it with its generated id.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest) (Category, error) {
	category := Category{
		ParentID: req.ParentID,
		Name:     req.Name,
		Type:     req.Type,
		Icon:     req.Icon,
		Color:    req.Color,
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO categories (user_id, parent_id, name, type, icon, color)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID,
		database.NullIfEmpty(req.ParentID),
		req.Name,
		req.Type,
		database.NullIfEmpty(req.Icon),
		database.NullIfEmpty(req.Color),
	).Scan(&category.ID)
	if err != nil {
		return Category{}, err
	}
	return category, nil
}

// Update applies the non-nil fields of req.
func (r *Repository) Update(ctx context.Context, userID, id string, req UpdateRequest) error {
	update := database.NewUpdate("categories")
	if req.Name != nil {
		update.Set("name", *req.Name)
	}
	if req.Type != nil {
		update.Set("type", *req.Type)
	}
	if req.ParentID != nil {
		update.Set("parent_id", database.NullIfEmpty(*req.ParentID))
	}
	if req.Icon != nil {
		update.Set("icon", database.NullIfEmpty(*req.Icon))
	}
	if req.Color != nil {
		update.Set("color", database.NullIfEmpty(*req.Color))
	}

	query, args := update.Build(id, userID)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Delete removes a category owned by the user.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// ValidateParent checks that parentID (when set) names a top-level category
// owned by the user, keeping the tree at most two levels deep.
func (r *Repository) ValidateParent(ctx context.Context, userID, parentID string) error {
	if parentID == "" {
		return nil
	}

	var grandParentID sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT parent_id FROM categories WHERE id = $1 AND user_id = $2
	`, parentID, userID).Scan(&grandParentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrParentNotFound
		}
		return err
	}
	if grandParentID.Valid {
		return ErrParentNotTopLevel
	}
	return nil
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scan(src scanner) (Category, error) {
	var (
		category              Category
		parentID, icon, color sql.NullString
	)
	if err := src.Scan(&category.ID, &parentID, &category.Name, &category.Type, &icon, &color); err != nil {
		return Category{}, err
	}
	category.ParentID = parentID.String
	category.Icon = icon.String
	category.Color = color.String
	return category, nil
}
