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
	Search   string
	Sort     string
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

// UpdateRequest is the PATCH /transactions/{id} payload; nil fields are left
// alone. AccountID moves the transaction to another account (a transfer's
// sending side). ToAccountID is a transfer's receiving account: it is required
// when turning an income or expense into a transfer, and is dropped when a
// transfer becomes an income or expense.
type UpdateRequest struct {
	AccountID   *string           `json:"account_id,omitempty"`
	ToAccountID *string           `json:"to_account_id,omitempty"`
	CategoryID  *string           `json:"category_id,omitempty"`
	Type        *string           `json:"type,omitempty"`
	Amount      *validate.Decimal `json:"amount,omitempty"`
	Note        *string           `json:"note,omitempty"`
	// Tags replaces the tag list when present; [] clears it.
	Tags       *[]string `json:"tags,omitempty"`
	OccurredAt *string   `json:"occurred_at,omitempty"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r.AccountID == nil && r.ToAccountID == nil && r.CategoryID == nil && r.Type == nil &&
		r.Amount == nil && r.Note == nil && r.OccurredAt == nil && r.Tags == nil
}

// shape is the part of a transaction that decides which accounts it moves
// money between: its type, accounts, and category ("" when unset).
type shape struct {
	Type, AccountID, ToAccountID, CategoryID string
}

func shapeOf(tx Transaction) shape {
	s := shape{Type: tx.Type, AccountID: tx.AccountID, ToAccountID: tx.ToAccountID}
	if tx.Category != nil {
		s.CategoryID = tx.Category.ID
	}
	return s
}

// InvalidUpdateError rejects a PATCH whose result would not be a valid
// income, expense, or transfer.
type InvalidUpdateError struct{ Message string }

func (e *InvalidUpdateError) Error() string { return e.Message }

func invalid(message string) error { return &InvalidUpdateError{Message: message} }

// nextShape applies req to current and checks the result. An income or
// expense has no receiving account; a transfer has one that differs from the
// sending account, and no category. Converting into a transfer therefore needs
// to_account_id and drops the category; converting out of one drops the
// receiving account.
func nextShape(current shape, req UpdateRequest) (shape, error) {
	next := current
	if req.Type != nil {
		next.Type = *req.Type
	}
	if req.AccountID != nil {
		if !validate.UUID(*req.AccountID) {
			return shape{}, invalid("account_id must be a UUID")
		}
		next.AccountID = *req.AccountID
	}
	if req.CategoryID != nil {
		next.CategoryID = *req.CategoryID
	}

	switch next.Type {
	case TypeIncome, TypeExpense:
		if req.ToAccountID != nil && *req.ToAccountID != "" {
			return shape{}, invalid("to_account_id is only allowed on transfers")
		}
		next.ToAccountID = ""
	case TypeTransfer:
		if req.ToAccountID != nil {
			next.ToAccountID = *req.ToAccountID
		}
		if next.ToAccountID == "" {
			return shape{}, invalid("to_account_id is required for transfers")
		}
		if !validate.UUID(next.ToAccountID) {
			return shape{}, invalid("to_account_id must be a UUID")
		}
		if next.ToAccountID == next.AccountID {
			return shape{}, invalid("can not transfer to the same account")
		}
		if req.CategoryID != nil && *req.CategoryID != "" {
			return shape{}, invalid("transfers can not have a category")
		}
		next.CategoryID = ""
	default:
		return shape{}, invalid("type must be income, expense, or transfer")
	}
	return next, nil
}
