package recurring

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
	"ledger-api/internal/transaction"
	"ledger-api/internal/validate"
)

const basePath = "/api/v1/recurring"

// Handler serves the /api/v1/recurring endpoints.
type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

// Register mounts the recurring rule routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(basePath, h.collection)
	mux.HandleFunc(basePath+"/", h.item)
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

// writeErr maps repository errors to responses.
func writeErr(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case transaction.IsClientError(err):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "could not "+action+" recurring rule")
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, userID string) {
	rules, err := h.repo.List(r.Context(), userID)
	if err != nil {
		writeErr(w, err, "list")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, userID string) {
	var req CreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	sched, loc, err := validateRule(&req)
	if err == nil {
		err = checkStartDate(req.StartDate, loc)
	}
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	start, _ := validate.Date(req.StartDate)

	id, err := h.repo.Create(r.Context(), userID, req, sched.onOrAfter(start))
	if err != nil {
		writeErr(w, err, "create")
		return
	}
	h.respond(w, r, userID, id, http.StatusCreated)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, userID, id string) {
	found, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "fetch")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"rule": found})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request, userID, id string) {
	var patch UpdateRequest
	if !httpx.DecodeJSON(w, r, &patch) {
		return
	}
	if patch.Empty() {
		httpx.WriteError(w, http.StatusBadRequest, "no fields provided")
		return
	}
	existing, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "fetch")
		return
	}

	// The merged rule is validated as a whole, exactly like a new one.
	req := requestFrom(existing)
	patch.apply(&req)
	sched, loc, err := validateRule(&req)
	if err == nil && req.StartDate != existing.StartDate {
		err = checkStartDate(req.StartDate, loc)
	}
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// A new schedule, or resuming a paused rule, restarts next_run_on: never
	// in a period that already ran, and on resume never in the past, so a
	// pause skips the periods it covered instead of back-filling them.
	resuming := !existing.IsActive && *req.IsActive
	next := existing.nextRunOn
	if resuming || sched != scheduleOf(requestFrom(existing)) || req.StartDate != existing.StartDate {
		next, _ = validate.Date(req.StartDate)
		if existing.lastRunOn != nil {
			next = later(next, sched.nextPeriod(dateOf(*existing.lastRunOn)))
		}
		if resuming {
			next = later(next, dateOf(time.Now().In(loc)))
		}
		next = sched.onOrAfter(next)
	}

	if err := h.repo.Update(r.Context(), userID, id, req, next, patch.IsActive != nil); err != nil {
		writeErr(w, err, "update")
		return
	}
	h.respond(w, r, userID, id, http.StatusOK)
}

// respond creates whatever the saved rule already has due (a start date in
// the past, or today), then replies with the rule as stored.
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, userID, id string, status int) {
	if _, err := h.repo.RunDue(r.Context(), time.Now(), userID); err != nil {
		slog.Error("recurring run after save", "error", err)
	}
	saved, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "fetch")
		return
	}
	httpx.WriteJSON(w, status, map[string]interface{}{"rule": saved})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request, userID, id string) {
	if err := h.repo.Delete(r.Context(), userID, id); err != nil {
		writeErr(w, err, "delete")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Recurring rule deleted",
		"id":      id,
	})
}

// apply copies the PATCH's fields onto req.
func (p UpdateRequest) apply(req *CreateRequest) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&req.Type, p.Type)
	set(&req.AccountID, p.AccountID)
	set(&req.ToAccountID, p.ToAccountID)
	set(&req.CategoryID, p.CategoryID)
	set(&req.Currency, p.Currency)
	set(&req.Note, p.Note)
	set(&req.Frequency, p.Frequency)
	set(&req.StartDate, p.StartDate)
	set(&req.EndDate, p.EndDate)
	set(&req.TimeZone, p.TimeZone)
	if p.Amount != nil {
		req.Amount = *p.Amount
	}
	if p.DayOfMonth != nil {
		req.DayOfMonth = p.DayOfMonth
	}
	if p.Weekday != nil {
		req.Weekday = p.Weekday
	}
	if p.IsActive != nil {
		req.IsActive = p.IsActive
	}
}

// validateRule checks a rule and normalizes it: the time zone defaults to
// UTC, the unused schedule field is dropped, and the amount, currency,
// accounts, category, and note get the exact checks of POST /transactions.
func validateRule(req *CreateRequest) (schedule, *time.Location, error) {
	req.TimeZone = strings.TrimSpace(req.TimeZone)
	if req.TimeZone == "" {
		req.TimeZone = "UTC"
	}
	loc, ok := validate.Location(req.TimeZone)
	if !ok {
		return schedule{}, nil, errors.New("time_zone must be an IANA time zone such as Asia/Bangkok")
	}
	if req.IsActive == nil {
		active := true
		req.IsActive = &active
	}

	switch req.Frequency {
	case FrequencyMonthly:
		if req.DayOfMonth == nil || *req.DayOfMonth < 1 || *req.DayOfMonth > 31 {
			return schedule{}, nil, errors.New("monthly rules need day_of_month between 1 and 31")
		}
		req.Weekday = nil
	case FrequencyWeekly:
		if req.Weekday == nil || *req.Weekday < 0 || *req.Weekday > 6 {
			return schedule{}, nil, errors.New("weekly rules need weekday between 0 (Sunday) and 6 (Saturday)")
		}
		req.DayOfMonth = nil
	default:
		return schedule{}, nil, errors.New("frequency must be monthly or weekly")
	}

	start, ok := validate.Date(req.StartDate)
	if !ok {
		return schedule{}, nil, errors.New("start_date must be YYYY-MM-DD")
	}
	if req.EndDate != "" {
		end, ok := validate.Date(req.EndDate)
		if !ok {
			return schedule{}, nil, errors.New("end_date must be YYYY-MM-DD")
		}
		if end.Before(start) {
			return schedule{}, nil, errors.New("end_date must not be before start_date")
		}
	}

	txReq := transactionRequest(*req, req.StartDate+"T00:00:00Z")
	if err := transaction.ValidateCreate(&txReq); err != nil {
		return schedule{}, nil, err
	}
	req.Currency = txReq.Currency
	return scheduleOf(*req), loc, nil
}

// checkStartDate bounds how far back a rule may start, which bounds how many
// transactions its first run back-fills.
func checkStartDate(startDate string, loc *time.Location) error {
	start, _ := validate.Date(startDate)
	if start.Before(dateOf(time.Now().In(loc)).AddDate(-1, 0, 0)) {
		return errors.New("start_date can be at most one year in the past")
	}
	return nil
}
