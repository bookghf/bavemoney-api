package recurring

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ledger-api/internal/database"
	"ledger-api/internal/transaction"
	"ledger-api/internal/validate"
)

// ErrNotFound is returned when the user has no such rule.
var ErrNotFound = errors.New("recurring rule not found")

// columns selects a rule joined with its account and category names.
const columns = `r.id, r.type, r.account_id, a.name, r.to_account_id, ta.name,
	r.category_id, c.name, c.icon, c.color, p.id, p.name, p.icon, p.color,
	r.amount, r.currency, r.note, r.frequency, r.day_of_month, r.weekday,
	r.start_date, r.end_date, r.next_run_on, r.last_run_on, r.time_zone,
	r.is_active, r.pause_reason, r.created_at, r.updated_at`

const from = `
	FROM recurring_rules r
	JOIN accounts a ON a.id = r.account_id
	LEFT JOIN accounts ta ON ta.id = r.to_account_id
	LEFT JOIN categories c ON c.id = r.category_id
	LEFT JOIN categories p ON p.id = c.parent_id`

// Repository reads and writes recurring rules owned by a user.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List returns the user's rules, active ones first, soonest due first.
func (r *Repository) List(ctx context.Context, userID string) ([]Rule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+columns+from+`
		WHERE r.user_id = $1
		ORDER BY r.is_active DESC, r.next_run_on, r.created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []Rule{}
	for rows.Next() {
		rule, err := scan(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// Get returns one rule, or ErrNotFound when the user does not own it.
func (r *Repository) Get(ctx context.Context, userID, id string) (Rule, error) {
	rule, err := scan(r.db.QueryRowContext(ctx, `
		SELECT `+columns+from+`
		WHERE r.id = $1 AND r.user_id = $2
	`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	return rule, err
}

// Create saves a rule validated by validateRule, first checking its accounts
// and category exactly as a new transaction would.
func (r *Repository) Create(ctx context.Context, userID string, req CreateRequest, nextRunOn time.Time) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	currency, err := transaction.CheckReferences(ctx, tx, userID, transactionRequest(req, ""))
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO recurring_rules (user_id, type, account_id, to_account_id, category_id, amount, currency, note,
			frequency, day_of_month, weekday, start_date, end_date, next_run_on, time_zone, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id
	`, userID, req.Type, req.AccountID, database.NullIfEmpty(req.ToAccountID), database.NullIfEmpty(req.CategoryID),
		string(req.Amount), currency, database.NullIfEmpty(req.Note), req.Frequency, req.DayOfMonth, req.Weekday,
		req.StartDate, database.NullIfEmpty(req.EndDate), nextRunOn.Format(time.DateOnly), req.TimeZone, req.IsActive == nil || *req.IsActive,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// Update replaces a rule's fields with req (the stored rule merged with the
// PATCH and validated again). clearPause drops the runner's pause reason, as
// when the user pauses or resumes the rule themselves.
func (r *Repository) Update(ctx context.Context, userID, id string, req CreateRequest, nextRunOn time.Time, clearPause bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	currency, err := transaction.CheckReferences(ctx, tx, userID, transactionRequest(req, ""))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE recurring_rules SET type = $3, account_id = $4, to_account_id = $5, category_id = $6, amount = $7,
			currency = $8, note = $9, frequency = $10, day_of_month = $11, weekday = $12, start_date = $13,
			end_date = $14, next_run_on = $15, time_zone = $16, is_active = $17,
			pause_reason = CASE WHEN $18 THEN NULL ELSE pause_reason END, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID, req.Type, req.AccountID, database.NullIfEmpty(req.ToAccountID), database.NullIfEmpty(req.CategoryID),
		string(req.Amount), currency, database.NullIfEmpty(req.Note), req.Frequency, req.DayOfMonth, req.Weekday,
		req.StartDate, database.NullIfEmpty(req.EndDate), nextRunOn.Format(time.DateOnly), req.TimeZone, req.IsActive == nil || *req.IsActive,
		clearPause)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// Delete removes a rule. Transactions it already created stay.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM recurring_rules WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// transactionRequest is the transaction a rule creates on occurredAt.
func transactionRequest(req CreateRequest, occurredAt string) transaction.CreateRequest {
	return transaction.CreateRequest{
		AccountID:   req.AccountID,
		ToAccountID: req.ToAccountID,
		CategoryID:  req.CategoryID,
		Type:        req.Type,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Note:        req.Note,
		OccurredAt:  occurredAt,
	}
}

// requestFrom turns a stored rule back into a full request, the base a PATCH
// is merged onto.
func requestFrom(rule Rule) CreateRequest {
	req := CreateRequest{
		Type:        rule.Type,
		AccountID:   rule.AccountID,
		ToAccountID: rule.ToAccountID,
		Amount:      validate.Decimal(rule.Amount),
		Currency:    rule.Currency,
		Note:        rule.Note,
		Frequency:   rule.Frequency,
		DayOfMonth:  rule.DayOfMonth,
		Weekday:     rule.Weekday,
		StartDate:   rule.StartDate,
		TimeZone:    rule.TimeZone,
		IsActive:    &rule.IsActive,
	}
	if rule.Category != nil {
		req.CategoryID = rule.Category.ID
	}
	if rule.EndDate != nil {
		req.EndDate = *rule.EndDate
	}
	return req
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scan(src scanner) (Rule, error) {
	var (
		rule                                                 Rule
		toAccountID, toAccountName, note, pauseReason        sql.NullString
		categoryID, categoryName, parentID, parentName       sql.NullString
		categoryIcon, categoryColor, parentIcon, parentColor sql.NullString
		dayOfMonth, weekday                                  sql.NullInt64
		startDate, nextRunOn                                 time.Time
		endDate, lastRunOn                                   sql.NullTime
	)
	if err := src.Scan(&rule.ID, &rule.Type, &rule.AccountID, &rule.AccountName, &toAccountID, &toAccountName,
		&categoryID, &categoryName, &categoryIcon, &categoryColor, &parentID, &parentName, &parentIcon, &parentColor,
		&rule.Amount, &rule.Currency, &note, &rule.Frequency, &dayOfMonth, &weekday,
		&startDate, &endDate, &nextRunOn, &lastRunOn, &rule.TimeZone,
		&rule.IsActive, &pauseReason, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return Rule{}, err
	}
	rule.ToAccountID = toAccountID.String
	rule.ToAccountName = toAccountName.String
	rule.Note = note.String
	if pauseReason.Valid {
		rule.PauseReason = &pauseReason.String
	}
	if dayOfMonth.Valid {
		day := int(dayOfMonth.Int64)
		rule.DayOfMonth = &day
	}
	if weekday.Valid {
		day := int(weekday.Int64)
		rule.Weekday = &day
	}
	if categoryID.Valid {
		rule.Category = &transaction.CategoryInfo{ID: categoryID.String, Name: categoryName.String, Icon: categoryIcon.String, Color: categoryColor.String}
		if parentID.Valid {
			rule.Category.Parent = &transaction.CategoryInfo{ID: parentID.String, Name: parentName.String, Icon: parentIcon.String, Color: parentColor.String}
		}
	}

	rule.StartDate = startDate.Format(time.DateOnly)
	rule.nextRunOn = nextRunOn
	if endDate.Valid {
		end := endDate.Time.Format(time.DateOnly)
		rule.EndDate = &end
	}
	if !endDate.Valid || !nextRunOn.After(endDate.Time) {
		next := nextRunOn.Format(time.DateOnly)
		rule.NextRunOn = &next
	}
	if lastRunOn.Valid {
		last := lastRunOn.Time.Format(time.DateOnly)
		rule.LastRunOn = &last
		rule.lastRunOn = &lastRunOn.Time
	}
	return rule, nil
}
