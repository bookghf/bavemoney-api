// Package recurring owns recurring rules (rent, salary, monthly transfers)
// and the runner that turns due rules into transactions.
package recurring

import (
	"time"

	"ledger-api/internal/transaction"
	"ledger-api/internal/validate"
)

// Rule frequencies.
const (
	FrequencyMonthly = "monthly"
	FrequencyWeekly  = "weekly"
)

// Pause reasons the runner records when it can not create a rule's
// transaction; the rule stays paused until the user fixes and resumes it.
const (
	ReasonAccountArchived = "account_archived"
	ReasonInvalid         = "invalid"
)

// Rule is the API representation of a recurring rule.
type Rule struct {
	ID            string                    `json:"id"`
	Type          string                    `json:"type"`
	AccountID     string                    `json:"account_id"`
	AccountName   string                    `json:"account_name"`
	ToAccountID   string                    `json:"to_account_id,omitempty"`
	ToAccountName string                    `json:"to_account_name,omitempty"`
	Category      *transaction.CategoryInfo `json:"category"`
	Amount        string                    `json:"amount"`
	Currency      string                    `json:"currency"`
	Note          string                    `json:"note,omitempty"`
	Frequency     string                    `json:"frequency"`
	DayOfMonth    *int                      `json:"day_of_month"`
	Weekday       *int                      `json:"weekday"`
	StartDate     string                    `json:"start_date"`
	EndDate       *string                   `json:"end_date"`
	// NextRunOn is the next local day a transaction is created; null once
	// the rule has passed its end date.
	NextRunOn *string `json:"next_run_on"`
	LastRunOn *string `json:"last_run_on"`
	TimeZone  string  `json:"time_zone"`
	IsActive  bool    `json:"is_active"`
	// PauseReason says why the runner paused the rule (account_archived,
	// invalid); null when the user paused it or it is active.
	PauseReason *string `json:"pause_reason"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`

	// nextRunOn and lastRunOn are the stored dates the handler schedules from.
	nextRunOn time.Time
	lastRunOn *time.Time
}

// CreateRequest is the POST /recurring payload. Amount, currency, accounts,
// category, and note follow the POST /transactions rules. Monthly rules take
// day_of_month (1-31; past the month's end means its last day), weekly rules
// take weekday (0 = Sunday ... 6 = Saturday). Dates are YYYY-MM-DD in
// time_zone (IANA, default UTC).
type CreateRequest struct {
	Type        string           `json:"type"`
	AccountID   string           `json:"account_id"`
	ToAccountID string           `json:"to_account_id,omitempty"`
	CategoryID  string           `json:"category_id,omitempty"`
	Amount      validate.Decimal `json:"amount"`
	Currency    string           `json:"currency,omitempty"`
	Note        string           `json:"note,omitempty"`
	Frequency   string           `json:"frequency"`
	DayOfMonth  *int             `json:"day_of_month,omitempty"`
	Weekday     *int             `json:"weekday,omitempty"`
	StartDate   string           `json:"start_date"`
	EndDate     string           `json:"end_date,omitempty"`
	TimeZone    string           `json:"time_zone,omitempty"`
	IsActive    *bool            `json:"is_active,omitempty"`
}

// UpdateRequest is the PATCH /recurring/{id} payload; nil fields are left
// alone. An empty to_account_id, category_id, note, or end_date clears it.
// is_active pauses or resumes the rule.
type UpdateRequest struct {
	Type        *string           `json:"type,omitempty"`
	AccountID   *string           `json:"account_id,omitempty"`
	ToAccountID *string           `json:"to_account_id,omitempty"`
	CategoryID  *string           `json:"category_id,omitempty"`
	Amount      *validate.Decimal `json:"amount,omitempty"`
	Currency    *string           `json:"currency,omitempty"`
	Note        *string           `json:"note,omitempty"`
	Frequency   *string           `json:"frequency,omitempty"`
	DayOfMonth  *int              `json:"day_of_month,omitempty"`
	Weekday     *int              `json:"weekday,omitempty"`
	StartDate   *string           `json:"start_date,omitempty"`
	EndDate     *string           `json:"end_date,omitempty"`
	TimeZone    *string           `json:"time_zone,omitempty"`
	IsActive    *bool             `json:"is_active,omitempty"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r == UpdateRequest{}
}

// schedule is when a rule falls due.
type schedule struct {
	frequency  string
	dayOfMonth int
	weekday    int
}

// onOrAfter returns the first due day on or after day (a UTC midnight).
func (s schedule) onOrAfter(day time.Time) time.Time {
	if s.frequency == FrequencyWeekly {
		return day.AddDate(0, 0, (s.weekday-int(day.Weekday())+7)%7)
	}
	if due := monthDay(day.Year(), day.Month(), s.dayOfMonth); !due.Before(day) {
		return due
	}
	return monthDay(day.Year(), day.Month()+1, s.dayOfMonth)
}

// after returns the due day following a run on day.
func (s schedule) after(day time.Time) time.Time {
	return s.onOrAfter(day.AddDate(0, 0, 1))
}

// nextPeriod returns the first day of the period after the one containing
// day: the next month, or the next Monday-started week. A rescheduled rule
// starts there so that a period never gets two runs.
func (s schedule) nextPeriod(day time.Time) time.Time {
	if s.frequency == FrequencyWeekly {
		isoWeekday := (int(day.Weekday())+6)%7 + 1 // Monday = 1 ... Sunday = 7
		return day.AddDate(0, 0, 8-isoWeekday)
	}
	return time.Date(day.Year(), day.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// monthDay is the given day of a month, clamped to the month's last day: the
// 31st is February 28 (or 29), April 30, and so on.
func monthDay(year int, month time.Month, day int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(day, last)-1)
}

// dateOf is the calendar day of t in its own location, as a UTC midnight.
func dateOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// later returns the later of two days.
func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
