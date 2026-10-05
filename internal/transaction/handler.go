package transaction

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

const basePath = "/api/v1/transactions"

// Paging bounds for GET /transactions.
const (
	defaultLimit = 20
	maxLimit     = 100
	maxPage      = 100_000
)

// Handler serves the /api/v1/transactions endpoints.
type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

// Register mounts the transaction routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(basePath, h.collection)
	mux.HandleFunc(basePath+"/", h.item)
	// More specific than basePath+"/", so these win over the {id} routes.
	mux.HandleFunc(basePath+"/import", h.importRows)
	mux.HandleFunc(basePath+"/export", h.export)
}

func (h *Handler) collection(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.list(w, r, userID)
	case http.MethodPost:
		h.create(w, r, userID)
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, basePath+"/")
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.get(w, r, userID, id)
	case http.MethodPatch:
		h.update(w, r, userID, id)
	case http.MethodDelete:
		h.delete(w, r, userID, id)
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, userID string) {
	filter, err := parseListFilter(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	transactions, total, err := h.repo.List(r.Context(), userID, filter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list transactions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"transactions": transactions,
		"pagination": map[string]interface{}{
			"page":        filter.Page,
			"limit":       filter.Limit,
			"total_items": total,
			"total_pages": (total + filter.Limit - 1) / filter.Limit,
		},
	})
}

// parseListFilter reads the GET /transactions query parameters, applying the
// spec's defaults (page 1, limit 20, max 100).
func parseListFilter(query url.Values) (ListFilter, error) {
	filter := ListFilter{
		Page:  1,
		Limit: defaultLimit,
		// account/category, or account_id/category_id as reports name them.
		AccountID:  firstOf(query, "account", "account_id"),
		CategoryID: firstOf(query, "category", "category_id"),
		Type:       query.Get("type"),
		Search:     strings.TrimSpace(query.Get("search")),
		Sort:       query.Get("sort"),
	}

	if raw := query.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return ListFilter{}, errors.New("page must be a positive integer")
		}
		filter.Page = page
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return ListFilter{}, errors.New("limit must be a positive integer")
		}
		filter.Limit = min(limit, maxLimit)
	}
	for _, id := range []string{filter.AccountID, filter.CategoryID} {
		if id != "" && !validate.UUID(id) {
			return ListFilter{}, errors.New("account and category must be UUIDs")
		}
	}
	if filter.Type != "" && filter.Type != TypeIncome && filter.Type != TypeExpense && filter.Type != TypeTransfer {
		return ListFilter{}, errors.New("type must be income, expense, or transfer")
	}
	if filter.Page > maxPage {
		return ListFilter{}, errors.New("page is too large")
	}
	if filter.Sort != "" {
		if _, ok := sortColumns[strings.TrimPrefix(filter.Sort, "-")]; !ok {
			return ListFilter{}, errors.New("sort must be occurred_at, amount, or created_at, optionally prefixed with -")
		}
	}

	// from/to are whole days in tz (default UTC), matching how reports
	// bucket transactions into days.
	loc, ok := validate.Location(query.Get("tz"))
	if !ok {
		return ListFilter{}, errors.New("tz must be an IANA time zone such as Asia/Bangkok")
	}
	if raw := query.Get("from"); raw != "" {
		day, ok := validate.Date(raw)
		if !ok {
			return ListFilter{}, errors.New("from and to must be YYYY-MM-DD")
		}
		filter.FromTime = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	}
	if raw := query.Get("to"); raw != "" {
		day, ok := validate.Date(raw)
		if !ok {
			return ListFilter{}, errors.New("from and to must be YYYY-MM-DD")
		}
		filter.ToTime = time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, loc)
	}
	if !filter.FromTime.IsZero() && !filter.ToTime.IsZero() && !filter.FromTime.Before(filter.ToTime) {
		return ListFilter{}, errors.New("from must not be after to")
	}
	for _, tag := range strings.Split(query.Get("tags"), ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			filter.Tags = append(filter.Tags, tag)
		}
	}
	return filter, nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, userID string) {
	var req CreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := validateCreate(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.repo.Create(r.Context(), userID, req)
	if err != nil {
		if IsClientError(err) {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not create transaction")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

// maxNoteLength and maxTags bound free-text input.
const (
	maxNoteLength = 500
	maxTags       = 20
	maxTagLength  = 50
)

const amountError = "amount must be a positive decimal with at most 2 decimal places and 16 digits"

// validateCreate checks a POST /transactions payload. Currency is optional:
// income and expense take their account's currency (a different one is
// rejected), and a transfer takes the shared currency of its two accounts. A
// transfer can not have a category.
func validateCreate(req *CreateRequest) error {
	if req.AccountID == "" || req.Type == "" || req.OccurredAt == "" {
		return errors.New("account_id, type, and occurred_at are required")
	}
	if !validate.UUID(req.AccountID) {
		return errors.New("account_id must be a UUID")
	}
	if !validate.Amount(string(req.Amount)) {
		return errors.New(amountError)
	}
	if _, ok := validate.Timestamp(req.OccurredAt); !ok {
		return errors.New("occurred_at must be an RFC 3339 timestamp, e.g. 2026-09-25T10:00:00+07:00")
	}
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.Currency != "" && !validate.Currency(req.Currency) {
		return errors.New("currency must be a 3-letter code such as THB")
	}
	if req.CategoryID != "" && !validate.UUID(req.CategoryID) {
		return errors.New("category_id must be a UUID")
	}
	if err := validateText(req.Note, req.Tags); err != nil {
		return err
	}

	switch req.Type {
	case TypeIncome, TypeExpense:
		if req.ToAccountID != "" {
			return errors.New("to_account_id is only allowed on transfers")
		}
	case TypeTransfer:
		if req.ToAccountID == "" {
			return errors.New("to_account_id is required for transfers")
		}
		if !validate.UUID(req.ToAccountID) {
			return errors.New("to_account_id must be a UUID")
		}
		if req.ToAccountID == req.AccountID {
			return errors.New("can not transfer to the same account")
		}
		if req.CategoryID != "" {
			return errors.New("transfers can not have a category")
		}
	default:
		return errors.New("type must be income, expense, or transfer")
	}
	return nil
}

func validateText(note string, tags []string) error {
	if err := validate.FreeText(note, maxNoteLength, "note"); err != nil {
		return err
	}
	if len(tags) > maxTags {
		return errors.New("at most 20 tags are allowed")
	}
	for _, tag := range tags {
		if err := validate.Name(tag, maxTagLength, "tag"); err != nil {
			return err
		}
	}
	return nil
}

// validateUpdate checks a PATCH against the stored transaction: the fields
// themselves, then that the result is still a valid income, expense, or
// transfer (see nextShape). The repository repeats the shape check against the
// row it locks.
func validateUpdate(existing Transaction, req UpdateRequest) error {
	if req.Amount != nil && !validate.Amount(string(*req.Amount)) {
		return errors.New(amountError)
	}
	if req.OccurredAt != nil {
		if _, ok := validate.Timestamp(*req.OccurredAt); !ok {
			return errors.New("occurred_at must be an RFC 3339 timestamp, e.g. 2026-09-25T10:00:00+07:00")
		}
	}
	if req.CategoryID != nil && *req.CategoryID != "" && !validate.UUID(*req.CategoryID) {
		return errors.New("category_id must be a UUID")
	}
	note := ""
	if req.Note != nil {
		note = *req.Note
	}
	var tags []string
	if req.Tags != nil {
		tags = *req.Tags
	}
	if err := validateText(note, tags); err != nil {
		return err
	}
	_, err := nextShape(shapeOf(existing), req)
	return err
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, userID, id string) {
	found, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "transaction not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch transaction")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, found)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request, userID, id string) {
	var req UpdateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Empty() {
		httpx.WriteError(w, http.StatusBadRequest, "no fields provided")
		return
	}

	existing, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "transaction not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch transaction")
		return
	}
	if err := validateUpdate(existing, req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repo.Update(r.Context(), userID, existing, req); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "transaction not found")
		case IsClientError(err):
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not update transaction")
		}
		return
	}

	updated, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch transaction")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request, userID, id string) {
	if err := h.repo.Delete(r.Context(), userID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "transaction not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete transaction")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Transaction deleted",
		"id":      id,
	})
}

// firstOf returns the first non-empty query value among keys.
func firstOf(query url.Values, keys ...string) string {
	for _, key := range keys {
		if value := query.Get(key); value != "" {
			return value
		}
	}
	return ""
}
