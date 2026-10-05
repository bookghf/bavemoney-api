package transaction

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"

	"ledger-api/internal/database"
	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

// maxImportRows bounds one import request; larger files are sent in chunks.
const maxImportRows = 2000

// ImportRow is one income or expense to import. AccountID overrides the
// request's default account for this row.
type ImportRow struct {
	AccountID  string           `json:"account_id,omitempty"`
	CategoryID string           `json:"category_id,omitempty"`
	Type       string           `json:"type"`
	Amount     validate.Decimal `json:"amount"`
	Note       string           `json:"note,omitempty"`
	OccurredAt string           `json:"occurred_at"`
}

// ImportRequest is the POST /transactions/import payload. Rows are parsed
// from a CSV by the app; Tag (e.g. "import:spending.csv") is added to every
// row so an import can be found, filtered, or checked for duplicates later.
type ImportRequest struct {
	AccountID string      `json:"account_id"`
	Tag       string      `json:"tag,omitempty"`
	Rows      []ImportRow `json:"rows"`
}

// RowError explains why one row (0-based index) was rejected.
type RowError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// maxReportedRowErrors keeps a bad file's error response readable.
const maxReportedRowErrors = 20

func (h *Handler) importRows(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}
	// Imports are larger than other bodies: 2000 rows fit in 2 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var req ImportRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if len(req.Rows) == 0 || len(req.Rows) > maxImportRows {
		httpx.WriteError(w, http.StatusBadRequest, fmt.Sprintf("send between 1 and %d rows", maxImportRows))
		return
	}
	if req.Tag != "" {
		if err := validate.Name(req.Tag, maxTagLength, "tag"); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	var rowErrors []RowError
	for i := range req.Rows {
		row := &req.Rows[i]
		if row.AccountID == "" {
			row.AccountID = req.AccountID
		}
		if err := validateImportRow(*row); err != nil {
			rowErrors = append(rowErrors, RowError{Row: i, Error: err.Error()})
		}
	}
	if len(rowErrors) > 0 {
		writeRowErrors(w, rowErrors)
		return
	}

	imported, rowErrors, err := h.repo.Import(r.Context(), userID, req)
	switch {
	case len(rowErrors) > 0:
		writeRowErrors(w, rowErrors)
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "could not import transactions")
	default:
		httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"imported": imported})
	}
}

func validateImportRow(row ImportRow) error {
	if row.Type != TypeIncome && row.Type != TypeExpense {
		return errors.New("type must be income or expense")
	}
	if !validate.UUID(row.AccountID) {
		return errors.New("account_id must be a UUID")
	}
	if row.CategoryID != "" && !validate.UUID(row.CategoryID) {
		return errors.New("category_id must be a UUID")
	}
	if !validate.Amount(string(row.Amount)) {
		return errors.New(amountError)
	}
	if _, ok := validate.Timestamp(row.OccurredAt); !ok {
		return errors.New("occurred_at must be an RFC 3339 timestamp")
	}
	return validate.FreeText(row.Note, maxNoteLength, "note")
}

func writeRowErrors(w http.ResponseWriter, rowErrors []RowError) {
	total := len(rowErrors)
	if total > maxReportedRowErrors {
		rowErrors = rowErrors[:maxReportedRowErrors]
	}
	httpx.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
		"error":       fmt.Sprintf("%d rows can not be imported; nothing was saved", total),
		"row_errors":  rowErrors,
		"error_count": total,
	})
}

// Import saves every row in one database transaction: either all rows are
// imported or none are. Accounts must be the user's and open; categories must
// be visible to the user and match the row's type.
func (r *Repository) Import(ctx context.Context, userID string, req ImportRequest) (int, []RowError, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	accounts := map[string]string{}   // id -> currency
	categories := map[string]string{} // id -> type
	rows, err := tx.QueryContext(ctx, `
		SELECT id::text, type FROM categories WHERE user_id = $1 OR (user_id IS NULL AND is_system)
	`, userID)
	if err != nil {
		return 0, nil, err
	}
	for rows.Next() {
		var id, categoryType string
		if err := rows.Scan(&id, &categoryType); err != nil {
			rows.Close()
			return 0, nil, err
		}
		categories[id] = categoryType
	}
	rows.Close()

	var rowErrors []RowError
	for i, row := range req.Rows {
		currency, seen := accounts[row.AccountID]
		if !seen {
			currency, err = lockAccount(ctx, tx, userID, row.AccountID, ErrAccountNotFound)
			if err != nil && !IsClientError(err) {
				return 0, nil, err
			}
			if err != nil {
				rowErrors = append(rowErrors, RowError{Row: i, Error: err.Error()})
				continue
			}
			accounts[row.AccountID] = currency
		}
		if row.CategoryID != "" {
			categoryType, ok := categories[row.CategoryID]
			switch {
			case !ok:
				rowErrors = append(rowErrors, RowError{Row: i, Error: ErrCategoryNotFound.Error()})
			case categoryType != row.Type:
				rowErrors = append(rowErrors, RowError{Row: i, Error: ErrCategoryType.Error()})
			}
		}
	}
	if len(rowErrors) > 0 {
		return 0, rowErrors, nil
	}

	var tags pq.StringArray
	if req.Tag != "" {
		tags = pq.StringArray{req.Tag}
	} else {
		tags = pq.StringArray{}
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO transactions (user_id, account_id, category_id, type, amount, currency, note, tags, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`)
	if err != nil {
		return 0, nil, err
	}
	defer stmt.Close()
	ids := make(pq.StringArray, 0, len(req.Rows))
	for _, row := range req.Rows {
		var id string
		if err := stmt.QueryRowContext(ctx, userID, row.AccountID, database.NullIfEmpty(row.CategoryID), row.Type,
			string(row.Amount), accounts[row.AccountID], database.NullIfEmpty(strings.TrimSpace(row.Note)), tags,
			row.OccurredAt).Scan(&id); err != nil {
			return 0, nil, err
		}
		ids = append(ids, id)
	}
	// Convert the whole batch to the default currency in one statement.
	if _, err := tx.ExecContext(ctx, `
		UPDATE transactions t SET amount_in_default_currency = `+convertedAmount+`
		WHERE t.id = ANY($1::uuid[])
	`, ids); err != nil {
		return 0, nil, err
	}
	return len(req.Rows), nil, tx.Commit()
}

// exportColumns is the CSV header. The app's importer reads this format back.
var exportColumns = []string{"date", "time", "type", "amount", "currency", "account", "to_account", "category", "subcategory", "note", "tags"}

// export writes every live transaction as CSV, oldest first, with dates in the
// caller's time zone (?tz=Asia/Bangkok). A UTF-8 byte order mark makes Excel
// and Google Sheets read Thai text correctly.
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}
	loc, ok := validate.Location(r.URL.Query().Get("tz"))
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "tz must be an IANA time zone such as Asia/Bangkok")
		return
	}

	rows, err := h.repo.db.QueryContext(r.Context(), `
		SELECT t.occurred_at, t.type, t.amount::text, t.currency, a.name, COALESCE(ta.name, ''),
			COALESCE(p.name, c.name, ''), CASE WHEN p.id IS NULL THEN '' ELSE c.name END,
			COALESCE(t.note, ''), COALESCE(array_to_string(t.tags, ';'), '')
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		LEFT JOIN accounts ta ON ta.id = t.to_account_id
		LEFT JOIN categories c ON c.id = t.category_id
		LEFT JOIN categories p ON p.id = c.parent_id
		WHERE t.user_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.occurred_at, t.created_at
	`, userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not export transactions")
		return
	}
	defer rows.Close()

	filename := "bavemoney-" + time.Now().In(loc).Format("20060102") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte("\xef\xbb\xbf")) // UTF-8 byte order mark
	out := csv.NewWriter(w)
	_ = out.Write(exportColumns)
	for rows.Next() {
		var (
			occurredAt                                                 time.Time
			kind, amount, currency, account, toAccount, top, sub, note string
			tags                                                       string
		)
		if err := rows.Scan(&occurredAt, &kind, &amount, &currency, &account, &toAccount, &top, &sub, &note, &tags); err != nil {
			return // headers are sent; the client sees a truncated file
		}
		local := occurredAt.In(loc)
		_ = out.Write([]string{local.Format("2006-01-02"), local.Format("15:04"), kind, amount, currency,
			account, toAccount, top, sub, note, tags})
	}
	out.Flush()
}
