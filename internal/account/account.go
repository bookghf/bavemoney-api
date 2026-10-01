// Package account owns the user's cash, bank, and card accounts.
package account

import "ledger-api/internal/validate"

// Account types.
var types = map[string]bool{"cash": true, "bank": true, "credit_card": true, "e_wallet": true}

// maxNameLength bounds account names, per the API spec.
const maxNameLength = 100

// Account is the API representation of an account. Money is a decimal string
// so balances never pick up float rounding.
type Account struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Currency       string `json:"currency"`
	InitialBalance string `json:"initial_balance"`
	CurrentBalance string `json:"current_balance"`
	IsArchived     bool   `json:"is_archived"`
	CreatedAt      string `json:"created_at"`
}

// CreateRequest is the POST /accounts payload.
type CreateRequest struct {
	Name           string           `json:"name"`
	Type           string           `json:"type"`
	Currency       string           `json:"currency"`
	InitialBalance validate.Decimal `json:"initial_balance"`
}

// UpdateRequest is the PATCH /accounts/{id} payload; nil fields are left alone.
type UpdateRequest struct {
	Name           *string           `json:"name"`
	Type           *string           `json:"type"`
	Currency       *string           `json:"currency"`
	InitialBalance *validate.Decimal `json:"initial_balance"`
	IsArchived     *bool             `json:"is_archived"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r.Name == nil && r.Type == nil && r.Currency == nil && r.InitialBalance == nil && r.IsArchived == nil
}
