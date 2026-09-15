package report

type CategorySummary struct {
	Category        CategoryInfo         `json:"category"`
	Total           string               `json:"total"`
	Percentage      float64              `json:"percentage"`
	Subcategories   []SubcategorySummary `json:"subcategories"`
}

type CategoryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type SubcategorySummary struct {
	Name       string  `json:"name"`
	Total      string  `json:"total"`
	Percentage float64 `json:"percentage"`
}

type DailyBreakdown struct {
	Date    string `json:"date"`
	Income  string `json:"income"`
	Expense string `json:"expense"`
}

type SummaryResponse struct {
	Period        string              `json:"period"`
	StartDate     string              `json:"start_date"`
	EndDate       string              `json:"end_date"`
	Currency      string              `json:"currency"`
	TotalIncome   string              `json:"total_income"`
	TotalExpense  string              `json:"total_expense"`
	Net           string              `json:"net"`
	ByCategory    []CategorySummary   `json:"by_category"`
	DailyBreakdown []DailyBreakdown   `json:"daily_breakdown"`
}
