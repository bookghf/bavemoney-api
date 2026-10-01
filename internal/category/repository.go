package category

import (
	"context"
	"database/sql"
	"errors"

	"ledger-api/internal/database"
	"ledger-api/internal/validate"
)

const columns = `id, parent_id, name, type, icon, color, is_system`

// visible matches the categories a user can see: their own plus the global
// system categories. $1 is the user id.
const visible = `(user_id = $1 OR (user_id IS NULL AND is_system))`

// Parent validation failures surfaced to the caller as 400s.
var (
	ErrParentNotFound    = errors.New("parent category not found")
	ErrParentNotTopLevel = errors.New("parent category must be a top-level category")
	ErrParentType        = errors.New("a subcategory must have the same type as its parent")
)

// Repository reads categories visible to a user and writes the ones they own.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's categories and the system categories as a flat
// slice ordered by name.
func (r *Repository) List(ctx context.Context, userID string) ([]Category, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+`
		FROM categories
		WHERE `+visible+`
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

// Get returns one category, or sql.ErrNoRows when the user can not see it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Category, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+columns+`
		FROM categories
		WHERE `+visible+` AND id = $2
	`, userID, id)
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
// visible to the user, keeping the tree at most two levels deep. Users may add
// their own subcategories under a system category.
// childType, when set, must equal the parent's type.
func (r *Repository) ValidateParent(ctx context.Context, userID, parentID, childType string) error {
	if parentID == "" {
		return nil
	}
	if !validate.UUID(parentID) {
		return ErrParentNotFound
	}

	var (
		grandParentID sql.NullString
		parentType    string
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT parent_id, type FROM categories WHERE `+visible+` AND id = $2
	`, userID, parentID).Scan(&grandParentID, &parentType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrParentNotFound
		}
		return err
	}
	if grandParentID.Valid {
		return ErrParentNotTopLevel
	}
	if childType != "" && childType != parentType {
		return ErrParentType
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
	if err := src.Scan(&category.ID, &parentID, &category.Name, &category.Type, &icon, &color, &category.IsSystem); err != nil {
		return Category{}, err
	}
	category.ParentID = parentID.String
	category.Icon = icon.String
	category.Color = color.String
	return category, nil
}
