// Package transaction owns ledger entries against accounts.
package transaction

import (
	"time"

	"ledger-api/internal/validate"
)

// Transaction types.
const (
	TypeIncome   = "income"
	TypeExpense  = "expense"
	TypeTransfer = "transfer"
)

type CategoryInfo struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Icon   string        `json:"icon,omitempty"`
	Color  string        `json:"color,omitempty"`
	Parent *CategoryInfo `json:"parent,omitempty"`
}

type Attachment struct {
	ID            string `json:"id"`
	FileURL       string `json:"file_url"`
	FileSizeBytes int    `json:"file_size_bytes"`
	UploadedAt    string `json:"uploaded_at"`
}

// Transaction is the API representation of a ledger entry.
type Transaction struct {
	ID            string        `json:"id"`
	AccountID     string        `json:"account_id"`
	AccountName   string        `json:"account_name"`
	ToAccountID   string        `json:"to_account_id,omitempty"`
	ToAccountName string        `json:"to_account_name,omitempty"`
	Category      *CategoryInfo `json:"category"`
	Type          string        `json:"type"`
	Amount        string        `json:"amount"`
	Currency      string        `json:"currency"`
	// AmountInDefaultCurrency is null when no exchange rate is known.
	AmountInDefaultCurrency *string      `json:"amount_in_default_currency"`
	Note                    string       `json:"note,omitempty"`
	Tags                    []string     `json:"tags"`
	Attachments             []Attachment `json:"attachments"`
	OccurredAt              string       `json:"occurred_at"`
	CreatedAt               string       `json:"created_at"`
	UpdatedAt               string       `json:"updated_at"`
}

// ListFilter narrows and pages GET /transactions. Page is 1-indexed; Sort is
// a column name optionally prefixed with "-" for descending.
type ListFilter struct {
	Page       int
	Limit      int
	AccountID  string
	CategoryID string
	Type       string
	Tags       []string
	// FromTime (inclusive) and ToTime (exclusive) bound occurred_at; zero
	// means unbounded.
	FromTime time.Time
	ToTime   time.Time
	// Search matches the note, category, account, tags, or amount; see
	// searchCondition. SearchCategories are extra category IDs the client
	// matched by display name (e.g. the Thai name of a system category).
	Search           string
	SearchCategories []string
	Sort             string
}

// CreateRequest is the POST /transactions payload. A transfer (type
// "transfer") moves the amount from AccountID to ToAccountID.
type CreateRequest struct {
	AccountID   string `json:"account_id"`
	ToAccountID string `json:"to_account_id,omitempty"`
	CategoryID  string `json:"category_id,omitempty"`
	Type        string `json:"type"`
	// Amount accepts "12.50" or 12.5; see validate.Decimal.
	Amount     validate.Decimal `json:"amount"`
	Currency   string           `json:"currency"`
	Note       string           `json:"note,omitempty"`
	Tags       []string         `json:"tags"`
	OccurredAt string           `json:"occurred_at"`
}

// UpdateRequest is the PATCH /transactions/{id} payload; nil fields are left alone.
type UpdateRequest struct {
	CategoryID *string           `json:"category_id,omitempty"`
	Type       *string           `json:"type,omitempty"`
	Amount     *validate.Decimal `json:"amount,omitempty"`
	Note       *string           `json:"note,omitempty"`
	// Tags replaces the tag list when present; [] clears it.
	Tags       *[]string `json:"tags,omitempty"`
	OccurredAt *string   `json:"occurred_at,omitempty"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r.CategoryID == nil && r.Type == nil && r.Amount == nil &&
		r.Note == nil && r.OccurredAt == nil && r.Tags == nil
}
