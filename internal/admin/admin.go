package admin

type AdminUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type AdminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AdminAuthResponse struct {
	Admin            AdminUser `json:"admin"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	ExpiresIn        int       `json:"expires_in"`
	RefreshExpiresIn int       `json:"refresh_token_expires_in"`
}

type UserInfo struct {
	ID                string `json:"id"`
	Email             string `json:"email"`
	DisplayName       string `json:"display_name"`
	DefaultCurrency   string `json:"default_currency"`
	Status            string `json:"status"`
	TransactionCount  int    `json:"transaction_count"`
	LastActiveAt      string `json:"last_active_at"`
	CreatedAt         string `json:"created_at"`
}

type UserDetailResponse struct {
	User  UserDetail `json:"user"`
	Stats UserStats  `json:"stats"`
}

type UserDetail struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name"`
	AvatarURL       string `json:"avatar_url"`
	DefaultCurrency string `json:"default_currency"`
	Status          string `json:"status"`
	GoogleID        string `json:"google_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type UserStats struct {
	TotalAccounts      int    `json:"total_accounts"`
	TotalTransactions  int    `json:"total_transactions"`
	TotalBudgets       int    `json:"total_budgets"`
	LastTransactionAt  string `json:"last_transaction_at"`
}

type SuspendRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type CategoryInfo struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	Icon            string           `json:"icon"`
	Color           string           `json:"color"`
	IsSystem        bool             `json:"is_system"`
	Subcategories   []CategoryInfo   `json:"subcategories"`
}

type CreateCategoryRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	ParentID *string `json:"parent_id"`
	Icon     string `json:"icon"`
	Color    string `json:"color"`
}

type CurrencyInfo struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	IsActive bool   `json:"is_active"`
}

type CreateCurrencyRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	IsActive bool   `json:"is_active"`
}

type UpdateCurrencyRequest struct {
	IsActive bool `json:"is_active"`
}

type ExchangeRate struct {
	ID             string `json:"id"`
	BaseCurrency   string `json:"base_currency"`
	TargetCurrency string `json:"target_currency"`
	Rate           string `json:"rate"`
	EffectiveDate  string `json:"effective_date"`
}

type CreateExchangeRateRequest struct {
	BaseCurrency   string `json:"base_currency"`
	TargetCurrency string `json:"target_currency"`
	Rate           string `json:"rate"`
	EffectiveDate  string `json:"effective_date"`
}

type AnalyticsOverview struct {
	TotalUsers        int           `json:"total_users"`
	ActiveUsers30d    int           `json:"active_users_30d"`
	NewUsers30d       int           `json:"new_users_30d"`
	TotalTransactions30d int        `json:"total_transactions_30d"`
	GrowthRatePct     float64       `json:"growth_rate_pct"`
	TopCurrencies     []CurrencyCount `json:"top_currencies"`
}

type CurrencyCount struct {
	Code      string `json:"code"`
	UserCount int    `json:"user_count"`
}

type ActivityDataPoint struct {
	Date               string `json:"date"`
	ActiveUsers        int    `json:"active_users"`
	NewUsers           int    `json:"new_users"`
	TransactionsCreated int   `json:"transactions_created"`
	TotalExpenseUSD    string `json:"total_expense_usd"`
	TotalIncomeUSD     string `json:"total_income_usd"`
}

type ActivityResponse struct {
	Period     string              `json:"period"`
	DataPoints []ActivityDataPoint `json:"data_points"`
}
