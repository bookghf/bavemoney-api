// Package budget owns per-period spending limits.
package budget

import "ledger-api/internal/validate"

// Budget periods.
var periods = map[string]bool{"weekly": true, "monthly": true, "yearly": true}

type CategoryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Budget is the API representation of a budget, with its progress through the
// period that contains today.
type Budget struct {
	ID                string        `json:"id"`
	Category          *CategoryInfo `json:"category"`
	Amount            string        `json:"amount"`
	Currency          string        `json:"currency"`
	Period            string        `json:"period"`
	StartDate         string        `json:"start_date"`
	AlertThresholdPct int           `json:"alert_threshold_pct"`
	// PeriodStart and PeriodEnd (inclusive) bound the current period.
	PeriodStart  string  `json:"period_start"`
	PeriodEnd    string  `json:"period_end"`
	CurrentSpend string  `json:"current_spend"`
	Remaining    string  `json:"remaining"`
	PercentUsed  float64 `json:"percent_used"`
	IsOverBudget bool    `json:"is_over_budget"`
	CreatedAt    string  `json:"created_at"`
}

// CreateRequest is the POST /budgets payload.
type CreateRequest struct {
	CategoryID        string           `json:"category_id,omitempty"`
	Amount            validate.Decimal `json:"amount"`
	Currency          string           `json:"currency"`
	Period            string           `json:"period"`
	StartDate         string           `json:"start_date"`
	AlertThresholdPct int              `json:"alert_threshold_pct,omitempty"`
}

// UpdateRequest is the PATCH /budgets/{id} payload; nil fields are left alone.
type UpdateRequest struct {
	Amount            *validate.Decimal `json:"amount"`
	Period            *string           `json:"period"`
	AlertThresholdPct *int              `json:"alert_threshold_pct,omitempty"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r.Amount == nil && r.Period == nil && r.AlertThresholdPct == nil
}
