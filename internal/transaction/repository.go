package transaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"ledger-api/internal/database"
)

// columns selects a transaction joined with its account and category names.
const columns = `t.id, t.account_id, a.name, t.to_account_id, ta.name, t.category_id, c.name, c.icon, c.color, p.id, p.name, p.icon, p.color,
	t.type, t.amount, t.currency, t.amount_in_default_currency, t.note, t.tags, t.occurred_at, t.created_at, t.updated_at`

// from is the join every read shares; the transfer target, category, and
// parent are optional.
const from = `
	FROM transactions t
	JOIN accounts a ON a.id = t.account_id
	LEFT JOIN accounts ta ON ta.id = t.to_account_id
	LEFT JOIN categories c ON c.id = t.category_id
	LEFT JOIN categories p ON p.id = c.parent_id`

// sortColumns maps the public sort keys to SQL.
var sortColumns = map[string]string{
	"occurred_at": "t.occurred_at",
	"amount":      "t.amount",
	"created_at":  "t.created_at",
}

// Repository reads and writes transactions owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns one page of the user's transactions matching filter, plus the
// total number of matches across all pages.
func (r *Repository) List(ctx context.Context, userID string, filter ListFilter) ([]Transaction, int, error) {
	where := []string{"t.user_id = $1", "t.deleted_at IS NULL"}
	args := []interface{}{userID}
	add := func(condition string, value interface{}) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}

	if filter.AccountID != "" {
		// Transfers show up on both the sending and the receiving account.
		add("(t.account_id = $%[1]d OR t.to_account_id = $%[1]d)", filter.AccountID)
	}
	if filter.CategoryID != "" {
		// A parent category also matches its subcategories' transactions.
		add("(t.category_id = $%[1]d OR c.parent_id = $%[1]d)", filter.CategoryID)
	}
	if filter.Type != "" {
		add("t.type = $%d", filter.Type)
	}
	if len(filter.Tags) > 0 {
		add("t.tags && $%d", pq.StringArray(filter.Tags))
	}
	// from/to are calendar days in the caller's time zone, resolved to UTC
	// instants by the handler so the (user_id, occurred_at) index applies.
	if !filter.FromTime.IsZero() {
		add("t.occurred_at >= $%d", filter.FromTime)
	}
	if !filter.ToTime.IsZero() {
		add("t.occurred_at < $%d", filter.ToTime)
	}
	if filter.Search != "" {
		add("t.note ILIKE '%%' || $%d || '%%'", escapeLike(filter.Search))
	}
	conditions := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*)"+from+conditions, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "t.occurred_at DESC"
	if column, ok := sortColumns[strings.TrimPrefix(filter.Sort, "-")]; ok {
		order = column + " ASC"
		if strings.HasPrefix(filter.Sort, "-") {
			order = column + " DESC"
		}
	}

	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
	query := fmt.Sprintf("SELECT %s%s%s ORDER BY %s, t.id DESC LIMIT $%d OFFSET $%d",
		columns, from, conditions, order, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	transactions := []Transaction{}
	for rows.Next() {
		tx, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, tx)
	}
	return transactions, total, rows.Err()
}

// escapeLike makes user input match literally inside an ILIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Get returns one transaction, or sql.ErrNoRows when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Transaction, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+columns+from+`
		WHERE t.id = $1 AND t.user_id = $2 AND t.deleted_at IS NULL
	`, id, userID)
	return scan(row)
}

// Validation failures surfaced to the caller as 400s (ErrNotFound as 404).
var (
	ErrNotFound          = errors.New("transaction not found")
	ErrAccountNotFound   = errors.New("account not found")
	ErrToAccountNotFound = errors.New("to_account_id not found")
	ErrAccountArchived   = errors.New("archived accounts can not take new transactions")
	ErrCurrencyMismatch  = errors.New("transfer accounts must use the same currency")
	ErrCategoryNotFound  = errors.New("category not found")
	ErrCategoryType      = errors.New("category type must match the transaction type")
)

// IsClientError reports whether err is one of the validation failures above.
func IsClientError(err error) bool {
	var currency *AccountCurrencyError
	return errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrToAccountNotFound) ||
		errors.Is(err, ErrAccountArchived) || errors.Is(err, ErrCurrencyMismatch) ||
		errors.Is(err, ErrCategoryNotFound) || errors.Is(err, ErrCategoryType) ||
		errors.As(err, &currency)
}

// AccountCurrencyError rejects an income/expense whose currency differs from
// its account's: an account holds one currency only.
type AccountCurrencyError struct{ Account, Given string }

func (e *AccountCurrencyError) Error() string {
	return fmt.Sprintf("currency %s does not match the account's currency %s", e.Given, e.Account)
}

// convertedAmount computes amount_in_default_currency for row t: the amount
// itself in the owner's default currency, otherwise converted at the latest
// exchange rate on or before the transaction date. It is NULL when no rate is
// known, rather than a misleading unconverted figure.
const convertedAmount = `(
	SELECT CASE
		WHEN t.currency = u.default_currency THEN t.amount
		ELSE (
			SELECT round(t.amount * er.rate, 2) FROM exchange_rates er
			WHERE er.base_currency = t.currency AND er.target_currency = u.default_currency
			  AND er.effective_date <= t.occurred_at::date
			ORDER BY er.effective_date DESC
			LIMIT 1
		)
	END
	FROM users u WHERE u.id = t.user_id
)`

// Create inserts a transaction and returns it as Get would. It runs in one
// database transaction that share-locks the accounts involved, so an account
// can not be archived between the checks and the insert.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest) (Transaction, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, err
	}
	defer func() { _ = tx.Rollback() }()

	id, err := CreateInTx(ctx, tx, userID, req)
	if err != nil {
		return Transaction{}, err
	}
	if err := tx.Commit(); err != nil {
		return Transaction{}, err
	}
	return r.Get(ctx, userID, id)
}

// CreateInTx inserts a transaction validated by ValidateCreate inside the
// caller's database transaction and returns its id. Other packages (such as
// recurring rules) use it to add transactions atomically with their own rows.
func CreateInTx(ctx context.Context, tx *sql.Tx, userID string, req CreateRequest) (string, error) {
	currency, err := CheckReferences(ctx, tx, userID, req)
	if err != nil {
		return "", err
	}

	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO transactions (user_id, account_id, to_account_id, category_id, type, amount, currency, note, tags, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, userID,
		req.AccountID,
		database.NullIfEmpty(req.ToAccountID),
		database.NullIfEmpty(req.CategoryID),
		req.Type,
		string(req.Amount),
		currency,
		database.NullIfEmpty(req.Note),
		pq.StringArray(req.Tags),
		req.OccurredAt,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	if err := convert(ctx, tx, id); err != nil {
		return "", err
	}
	return id, nil
}

// CheckReferences verifies, inside tx, that the accounts and category of req
// are the user's and usable: accounts open (and share-locked), a transfer's
// accounts in one currency, an income/expense in its account's currency, and
// the category matching the type. It returns the transaction's currency.
func CheckReferences(ctx context.Context, tx *sql.Tx, userID string, req CreateRequest) (string, error) {
	currency, err := lockAccount(ctx, tx, userID, req.AccountID, ErrAccountNotFound)
	if err != nil {
		return "", err
	}
	switch req.Type {
	case TypeTransfer:
		toCurrency, err := lockAccount(ctx, tx, userID, req.ToAccountID, ErrToAccountNotFound)
		if err != nil {
			return "", err
		}
		if toCurrency != currency {
			return "", ErrCurrencyMismatch
		}
	default:
		if req.Currency != "" && req.Currency != currency {
			return "", &AccountCurrencyError{Account: currency, Given: req.Currency}
		}
	}
	if req.CategoryID != "" {
		if err := checkCategory(ctx, tx, userID, req.CategoryID, req.Type); err != nil {
			return "", err
		}
	}
	return currency, nil
}

// lockAccount share-locks an account the user owns and returns its currency.
// notFound is returned when the user has no such account.
func lockAccount(ctx context.Context, tx *sql.Tx, userID, accountID string, notFound error) (string, error) {
	var (
		currency string
		archived bool
	)
	err := tx.QueryRowContext(ctx, `
		SELECT currency, is_archived FROM accounts WHERE id = $1 AND user_id = $2 FOR SHARE
	`, accountID, userID).Scan(&currency, &archived)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", notFound
	case err != nil:
		return "", err
	case archived:
		return "", ErrAccountArchived
	}
	return currency, nil
}

// checkCategory verifies that the user may use the category (their own or a
// system one) and that its type matches the transaction's.
func checkCategory(ctx context.Context, tx *sql.Tx, userID, categoryID, txType string) error {
	var categoryType string
	err := tx.QueryRowContext(ctx, `
		SELECT type FROM categories
		WHERE id = $1 AND (user_id = $2 OR (user_id IS NULL AND is_system))
	`, categoryID, userID).Scan(&categoryType)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrCategoryNotFound
	case err != nil:
		return err
	case categoryType != txType:
		return ErrCategoryType
	}
	return nil
}

// convert (re)computes amount_in_default_currency for one transaction.
func convert(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, `UPDATE transactions t SET amount_in_default_currency = `+convertedAmount+` WHERE t.id = $1`, id)
	return err
}

// Update applies the non-nil fields of req to the transaction existing (as
// loaded by Get), returning ErrNotFound when it disappeared meanwhile.
func (r *Repository) Update(ctx context.Context, userID string, existing Transaction, req UpdateRequest) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	newType := existing.Type
	if req.Type != nil {
		newType = *req.Type
	}
	// A new category, or a type change that keeps the old category, must
	// still pair a category with a transaction of the same type.
	categoryID := ""
	if existing.Category != nil {
		categoryID = existing.Category.ID
	}
	if req.CategoryID != nil {
		categoryID = *req.CategoryID
	}
	if categoryID != "" && (req.CategoryID != nil || req.Type != nil) {
		if err := checkCategory(ctx, tx, userID, categoryID, newType); err != nil {
			return err
		}
	}

	update := database.NewUpdate("transactions")
	if req.CategoryID != nil {
		update.Set("category_id", database.NullIfEmpty(*req.CategoryID))
	}
	if req.Type != nil {
		update.Set("type", *req.Type)
	}
	if req.Amount != nil {
		update.Set("amount", string(*req.Amount))
	}
	if req.Note != nil {
		update.Set("note", database.NullIfEmpty(*req.Note))
	}
	if req.OccurredAt != nil {
		update.Set("occurred_at", *req.OccurredAt)
	}
	if req.Tags != nil {
		update.Set("tags", pq.StringArray(*req.Tags))
	}

	query, args := update.Build(existing.ID, userID)
	result, err := tx.ExecContext(ctx, query+" AND deleted_at IS NULL", args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := convert(ctx, tx, existing.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete soft-deletes a transaction owned by the user, returning ErrNotFound
// when there is no such live transaction.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE transactions SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scan(src scanner) (Transaction, error) {
	var (
		tx                                                   Transaction
		toAccountID, toAccountName                           sql.NullString
		categoryID, categoryName, parentID, parentName, note sql.NullString
		categoryIcon, categoryColor, parentIcon, parentColor sql.NullString
		converted                                            sql.NullString
		tags                                                 pq.StringArray
	)
	if err := src.Scan(&tx.ID, &tx.AccountID, &tx.AccountName, &toAccountID, &toAccountName, &categoryID, &categoryName, &categoryIcon, &categoryColor, &parentID, &parentName, &parentIcon, &parentColor,
		&tx.Type, &tx.Amount, &tx.Currency, &converted, &note, &tags, &tx.OccurredAt, &tx.CreatedAt, &tx.UpdatedAt); err != nil {
		return Transaction{}, err
	}
	tx.ToAccountID = toAccountID.String
	tx.ToAccountName = toAccountName.String
	tx.Note = note.String
	tx.Tags = tags
	if tx.Tags == nil {
		tx.Tags = []string{}
	}
	tx.Attachments = []Attachment{}
	if converted.Valid {
		tx.AmountInDefaultCurrency = &converted.String
	}

	if categoryID.Valid {
		tx.Category = &CategoryInfo{ID: categoryID.String, Name: categoryName.String, Icon: categoryIcon.String, Color: categoryColor.String}
		if parentID.Valid {
			tx.Category.Parent = &CategoryInfo{ID: parentID.String, Name: parentName.String, Icon: parentIcon.String, Color: parentColor.String}
		}
	}
	return tx, nil
}
