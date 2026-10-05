package report

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"ledger-api/internal/month"
	"ledger-api/internal/validate"

	// Embedded zone data so tz validation works on images without tzdata.
	_ "time/tzdata"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// InvalidFilterError reports a query parameter the caller must fix.
type InvalidFilterError struct{ msg string }

func (e *InvalidFilterError) Error() string { return e.msg }

func invalid(format string, args ...interface{}) error {
	return &InvalidFilterError{msg: fmt.Sprintf(format, args...)}
}

const dateLayout = "2006-01-02"

// GetSummary totals the user's income and expense over the filter's date
// range, breaks the selected type down by category and subcategory, and
// returns one daily_breakdown row per day in the range (zero days included).
// Transfers move money between the user's own accounts and are left out.
func (r *Repository) GetSummary(ctx context.Context, userID string, filter SummaryFilter) (SummaryResponse, error) {
	// The user's own currency is the default, and their month_start_day
	// decides what a month covers.
	var defaultCurrency string
	if err := r.db.QueryRowContext(ctx, `SELECT default_currency, month_start_day FROM users WHERE id = $1`, userID).
		Scan(&defaultCurrency, &filter.MonthStartDay); err != nil {
		return SummaryResponse{}, err
	}
	filter, start, end, err := normalize(filter)
	if err != nil {
		return SummaryResponse{}, err
	}

	summary := SummaryResponse{
		Period:         filter.Period,
		StartDate:      start.Format(dateLayout),
		EndDate:        end.Format(dateLayout),
		Currency:       filter.Currency,
		TimeZone:       filter.TimeZone,
		Type:           filter.Type,
		ByCategory:     []CategorySummary{},
		DailyBreakdown: []DailyBreakdown{},
	}

	if summary.Currency == "" {
		summary.Currency = defaultCurrency
	}

	// tx is the filtered set every query below aggregates. $2 is the time
	// zone that decides each transaction's calendar day. The range bounds are
	// written against occurred_at itself so the (user_id, occurred_at) index
	// applies. Amounts are only added up within one currency: a report covers
	// the transactions recorded in $5, never a sum of mixed currencies.
	// Cents are NUMERIC, not bigint, so even absurd totals can not overflow.
	args := []interface{}{userID, filter.TimeZone, summary.StartDate, summary.EndDate, summary.Currency}
	tx := `
		WITH tx AS (
			SELECT t.type, t.category_id, c.parent_id,
				t.amount * 100 AS cents,
				(t.occurred_at AT TIME ZONE $2)::date AS day
			FROM transactions t
			LEFT JOIN categories c ON c.id = t.category_id
			WHERE t.user_id = $1
				AND t.deleted_at IS NULL
				AND t.type IN ('income', 'expense')
				AND t.currency = $5
				AND t.occurred_at >= $3::date::timestamp AT TIME ZONE $2
				AND t.occurred_at < ($4::date + 1)::timestamp AT TIME ZONE $2`
	if filter.AccountID != "" {
		args = append(args, filter.AccountID)
		tx += fmt.Sprintf(" AND t.account_id = $%d", len(args))
	}
	if filter.CategoryID != "" {
		// A parent category also matches its subcategories' transactions.
		args = append(args, filter.CategoryID)
		tx += fmt.Sprintf(" AND (t.category_id = $%[1]d OR c.parent_id = $%[1]d)", len(args))
	}
	tx += `
		)`

	var income, expense centsValue
	err = r.db.QueryRowContext(ctx, tx+`
		SELECT
			COALESCE(SUM(cents) FILTER (WHERE type = 'income'), 0)::text,
			COALESCE(SUM(cents) FILTER (WHERE type = 'expense'), 0)::text,
			COUNT(*)
		FROM tx
	`, args...).Scan(&income, &expense, &summary.TransactionCount)
	if err != nil {
		return SummaryResponse{}, err
	}
	summary.TotalIncome = formatCents(income.Int)
	summary.TotalExpense = formatCents(expense.Int)
	summary.Net = formatCents(new(big.Int).Sub(income.Int, expense.Int))

	typeTotal := expense.Int
	if filter.Type == "income" {
		typeTotal = income.Int
	}
	if summary.ByCategory, err = r.byCategory(ctx, tx, args, filter.Type, typeTotal); err != nil {
		return SummaryResponse{}, err
	}
	if summary.DailyBreakdown, err = r.daily(ctx, tx, args); err != nil {
		return SummaryResponse{}, err
	}
	return summary, nil
}

// categoryRow is one (top-level category, subcategory) group of tx.
type categoryRow struct {
	topID, topName, topIcon, topColor sql.NullString
	subID, subName                    sql.NullString
	cents                             *big.Int
	count                             int
}

// centsValue scans a NUMERIC cent amount (as text) into a big.Int.
type centsValue struct{ *big.Int }

func (c *centsValue) Scan(src interface{}) error {
	var text string
	switch v := src.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return fmt.Errorf("cents: unexpected %T", src)
	}
	// NUMERIC renders amount*100 with a trailing ".00"; drop the fraction.
	value, ok := new(big.Float).SetPrec(256).SetString(text)
	if !ok {
		return fmt.Errorf("cents: invalid number %q", text)
	}
	c.Int, _ = value.Int(nil)
	return nil
}

func (r *Repository) byCategory(ctx context.Context, tx string, args []interface{}, txType string, total *big.Int) ([]CategorySummary, error) {
	args = append(append([]interface{}{}, args...), txType)
	rows, err := r.db.QueryContext(ctx, tx+fmt.Sprintf(`
		SELECT top.id, top.name, top.icon, top.color, sub.id, sub.name, SUM(tx.cents)::text, COUNT(*)
		FROM tx
		LEFT JOIN categories top ON top.id = COALESCE(tx.parent_id, tx.category_id)
		LEFT JOIN categories sub ON sub.id = tx.category_id AND tx.parent_id IS NOT NULL
		WHERE tx.type = $%d
		GROUP BY top.id, top.name, top.icon, top.color, sub.id, sub.name
	`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grouped []categoryRow
	for rows.Next() {
		var (
			row   categoryRow
			cents centsValue
		)
		if err := rows.Scan(&row.topID, &row.topName, &row.topIcon, &row.topColor, &row.subID, &row.subName, &cents, &row.count); err != nil {
			return nil, err
		}
		row.cents = cents.Int
		grouped = append(grouped, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildCategories(grouped, txType, total), nil
}

// buildCategories folds (category, subcategory) groups into the by_category
// tree, largest first. Transactions with no category land in an
// "Uncategorized" entry; those filed on a parent with no subcategory land in
// its "Other" subcategory when the parent also has real subcategories.
func buildCategories(grouped []categoryRow, txType string, total *big.Int) []CategorySummary {
	type acc struct {
		info  CategoryInfo
		cents *big.Int
		count int
		subs  []categoryRow
	}
	byID := map[string]*acc{}
	var order []*acc
	for _, row := range grouped {
		key := row.topID.String
		a, ok := byID[key]
		if !ok {
			info := CategoryInfo{ID: row.topID.String, Name: row.topName.String, Type: txType, Icon: row.topIcon.String, Color: row.topColor.String}
			if !row.topID.Valid {
				info.Name = "Uncategorized"
			}
			a = &acc{info: info, cents: new(big.Int)}
			byID[key] = a
			order = append(order, a)
		}
		a.cents.Add(a.cents, row.cents)
		a.count += row.count
		a.subs = append(a.subs, row)
	}

	categories := make([]CategorySummary, 0, len(order))
	for _, a := range order {
		summary := CategorySummary{
			Category:         a.info,
			Total:            formatCents(a.cents),
			Percentage:       percentage(a.cents, total),
			TransactionCount: a.count,
			Subcategories:    []SubcategorySummary{},
		}
		hasSubs := false
		for _, sub := range a.subs {
			hasSubs = hasSubs || sub.subID.Valid
		}
		if hasSubs {
			sort.SliceStable(a.subs, func(i, j int) bool { return a.subs[i].cents.Cmp(a.subs[j].cents) > 0 })
			for _, sub := range a.subs {
				name := sub.subName.String
				if !sub.subID.Valid {
					name = "Other"
				}
				summary.Subcategories = append(summary.Subcategories, SubcategorySummary{
					ID:               sub.subID.String,
					Name:             name,
					Total:            formatCents(sub.cents),
					Percentage:       percentage(sub.cents, total),
					TransactionCount: sub.count,
				})
			}
		}
		categories = append(categories, summary)
	}
	sort.SliceStable(categories, func(i, j int) bool {
		return byID[categories[i].Category.ID].cents.Cmp(byID[categories[j].Category.ID].cents) > 0
	})
	return categories
}

func (r *Repository) daily(ctx context.Context, tx string, args []interface{}) ([]DailyBreakdown, error) {
	rows, err := r.db.QueryContext(ctx, tx+`
		SELECT to_char(d, 'YYYY-MM-DD'),
			COALESCE(SUM(tx.cents) FILTER (WHERE tx.type = 'income'), 0)::text,
			COALESCE(SUM(tx.cents) FILTER (WHERE tx.type = 'expense'), 0)::text
		FROM generate_series($3::date, $4::date, interval '1 day') AS d
		LEFT JOIN tx ON tx.day = d::date
		GROUP BY d
		ORDER BY d
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	days := []DailyBreakdown{}
	for rows.Next() {
		var (
			day             DailyBreakdown
			income, expense centsValue
		)
		if err := rows.Scan(&day.Date, &income, &expense); err != nil {
			return nil, err
		}
		day.Income = formatCents(income.Int)
		day.Expense = formatCents(expense.Int)
		days = append(days, day)
	}
	return days, rows.Err()
}

// normalize validates filter, fills its defaults, and returns the first and
// last day of the range it covers.
func normalize(filter SummaryFilter) (SummaryFilter, time.Time, time.Time, error) {
	if filter.Type == "" {
		filter.Type = "expense"
	}
	if filter.Type != "expense" && filter.Type != "income" {
		return filter, time.Time{}, time.Time{}, invalid("type must be expense or income")
	}

	if filter.TimeZone == "" {
		filter.TimeZone = "UTC"
	}
	// "Local" is Go-only; Postgres would reject it.
	if _, err := time.LoadLocation(filter.TimeZone); err != nil || filter.TimeZone == "Local" {
		return filter, time.Time{}, time.Time{}, invalid("tz must be an IANA time zone such as Asia/Bangkok")
	}

	for _, id := range []string{filter.AccountID, filter.CategoryID} {
		if id != "" && !validate.UUID(id) {
			return filter, time.Time{}, time.Time{}, invalid("account_id and category_id must be UUIDs")
		}
	}
	if filter.Currency != "" && !validate.Currency(filter.Currency) {
		return filter, time.Time{}, time.Time{}, invalid("currency must be a 3-letter code such as THB")
	}

	start, end, err := periodRange(filter)
	return filter, start, end, err
}

// periodRange returns the first and last day of the filter's period. For
// day/week/month/year, date may be YYYY-MM-DD; month also takes YYYY-MM and
// year YYYY. Weeks start on Sunday. Months start on MonthStartDay: a date
// picks the month containing it, YYYY-MM the month that starts in it. A
// custom period uses from and to.
func periodRange(filter SummaryFilter) (time.Time, time.Time, error) {
	if filter.Period == PeriodCustom {
		start, errFrom := time.Parse(dateLayout, filter.From)
		end, errTo := time.Parse(dateLayout, filter.To)
		switch {
		case errFrom != nil || errTo != nil:
			return time.Time{}, time.Time{}, invalid("custom period requires from and to as YYYY-MM-DD")
		case end.Before(start):
			return time.Time{}, time.Time{}, invalid("to must not be before from")
		case end.Sub(start).Hours()/24 >= maxCustomDays:
			return time.Time{}, time.Time{}, invalid("custom period can span at most %d days", maxCustomDays)
		}
		return start, end, nil
	}

	var layouts []string
	switch filter.Period {
	case PeriodDay, PeriodWeek:
		layouts = []string{dateLayout}
	case PeriodMonth:
		layouts = []string{dateLayout, "2006-01"}
	case PeriodYear:
		layouts = []string{dateLayout, "2006"}
	default:
		return time.Time{}, time.Time{}, invalid("period must be day, week, month, year, or custom")
	}

	var (
		t      time.Time
		err    error
		layout string
	)
	for _, layout = range layouts {
		if t, err = time.Parse(layout, filter.Date); err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, time.Time{}, invalid("date is required and must match the %s period", filter.Period)
	}

	switch filter.Period {
	case PeriodDay:
		return t, t, nil
	case PeriodWeek:
		start := t.AddDate(0, 0, -int(t.Weekday()))
		return start, start.AddDate(0, 0, 6), nil
	case PeriodMonth:
		if layout != dateLayout && month.ValidStartDay(filter.MonthStartDay) {
			// YYYY-MM parses to the 1st; name the month starting in it.
			t = t.AddDate(0, 0, filter.MonthStartDay-1)
		}
		start, end := month.Range(t, filter.MonthStartDay)
		return start, end, nil
	default:
		start := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return start, time.Date(t.Year(), 12, 31, 0, 0, 0, 0, time.UTC), nil
	}
}

// formatCents renders an integer cent amount as a 2-decimal string.
func formatCents(cents *big.Int) string {
	sign := ""
	abs := new(big.Int).Set(cents)
	if abs.Sign() < 0 {
		sign = "-"
		abs.Neg(abs)
	}
	whole, frac := new(big.Int).QuoRem(abs, big.NewInt(100), new(big.Int))
	return fmt.Sprintf("%s%s.%02d", sign, whole.String(), frac.Int64())
}

// percentage returns part/total as a percentage rounded to 2 decimals.
func percentage(part, total *big.Int) float64 {
	if total.Sign() == 0 {
		return 0
	}
	ratio, _ := new(big.Rat).SetFrac(part, total).Float64()
	return math.Round(ratio*10000) / 100
}
