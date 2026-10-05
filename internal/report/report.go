package report

// Report periods. A custom period covers From..To; the others cover the
// day, week (Sunday to Saturday), month, or year containing Date. Months
// begin on the user's month_start_day (calendar months when it is 1).
const (
	PeriodDay    = "day"
	PeriodWeek   = "week"
	PeriodMonth  = "month"
	PeriodYear   = "year"
	PeriodCustom = "custom"
)

// maxCustomDays bounds a custom range so the daily breakdown stays small.
const maxCustomDays = 731

// SummaryFilter is the parsed GET /reports/summary query.
type SummaryFilter struct {
	Period    string
	Date      string
	From      string
	To        string
	AccountID string
	// CategoryID narrows the report to one category. A top-level category
	// also matches its subcategories' transactions.
	CategoryID string
	// Type selects which transactions by_category breaks down: expense
	// (default) or income.
	Type     string
	Currency string
	// TimeZone is the IANA zone used to decide which calendar day a
	// transaction falls on. Defaults to UTC.
	TimeZone string
	// MonthStartDay is the day (1-28) the user's month begins on. It comes
	// from the user's profile, not the query.
	MonthStartDay int
}

type CategorySummary struct {
	Category         CategoryInfo         `json:"category"`
	Total            string               `json:"total"`
	Percentage       float64              `json:"percentage"`
	TransactionCount int                  `json:"transaction_count"`
	Subcategories    []SubcategorySummary `json:"subcategories"`
}

// CategoryInfo identifies a top-level category. ID is empty for the
// "Uncategorized" bucket.
type CategoryInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

// SubcategorySummary is one subcategory's share. ID is empty for the "Other"
// bucket holding transactions filed directly under the parent.
type SubcategorySummary struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Total            string  `json:"total"`
	Percentage       float64 `json:"percentage"`
	TransactionCount int     `json:"transaction_count"`
}

type DailyBreakdown struct {
	Date    string `json:"date"`
	Income  string `json:"income"`
	Expense string `json:"expense"`
}

type SummaryResponse struct {
	Period           string            `json:"period"`
	StartDate        string            `json:"start_date"`
	EndDate          string            `json:"end_date"`
	Currency         string            `json:"currency"`
	TimeZone         string            `json:"time_zone"`
	Type             string            `json:"type"`
	TotalIncome      string            `json:"total_income"`
	TotalExpense     string            `json:"total_expense"`
	Net              string            `json:"net"`
	TransactionCount int               `json:"transaction_count"`
	ByCategory       []CategorySummary `json:"by_category"`
	DailyBreakdown   []DailyBreakdown  `json:"daily_breakdown"`
}
