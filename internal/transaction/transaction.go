// Package transaction owns ledger entries against accounts.
package transaction

type CategoryInfo struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Parent *CategoryInfo  `json:"parent,omitempty"`
}

type Attachment struct {
	ID            string `json:"id"`
	FileURL       string `json:"file_url"`
	FileSizeBytes int    `json:"file_size_bytes"`
	UploadedAt    string `json:"uploaded_at"`
}

// Transaction is the API representation of a ledger entry.
type Transaction struct {
	ID                       string        `json:"id"`
	AccountID                string        `json:"account_id"`
	AccountName              string        `json:"account_name"`
	Category                 *CategoryInfo `json:"category"`
	Type                     string        `json:"type"`
	Amount                   string        `json:"amount"`
	Currency                 string        `json:"currency"`
	AmountInDefaultCurrency  string        `json:"amount_in_default_currency"`
	Note                     string        `json:"note,omitempty"`
	Tags                     []string      `json:"tags"`
	Attachments              []Attachment  `json:"attachments"`
	OccurredAt               string        `json:"occurred_at"`
	CreatedAt                string        `json:"created_at"`
	UpdatedAt                string        `json:"updated_at"`
}

// CreateRequest is the POST /transactions payload.
type CreateRequest struct {
	AccountID  string   `json:"account_id"`
	CategoryID string   `json:"category_id,omitempty"`
	Type       string   `json:"type"`
	Amount     string   `json:"amount"`
	Currency   string   `json:"currency"`
	Note       string   `json:"note,omitempty"`
	Tags       []string `json:"tags"`
	OccurredAt string   `json:"occurred_at"`
}

// UpdateRequest is the PATCH /transactions/{id} payload; nil fields are left alone.
type UpdateRequest struct {
	CategoryID *string  `json:"category_id,omitempty"`
	Type       *string  `json:"type,omitempty"`
	Amount     *string  `json:"amount,omitempty"`
	Note       *string  `json:"note,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	OccurredAt *string  `json:"occurred_at,omitempty"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r.CategoryID == nil && r.Type == nil && r.Amount == nil &&
		r.Note == nil && r.OccurredAt == nil && len(r.Tags) == 0
}
