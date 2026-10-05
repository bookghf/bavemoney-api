package report

import (
	"errors"
	"net/http"
	"strings"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
)

const basePath = "/api/v1/reports"

type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(basePath+"/summary", h.summary)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}

	query := r.URL.Query()
	filter := SummaryFilter{
		Period:     query.Get("period"),
		Date:       query.Get("date"),
		From:       query.Get("from"),
		To:         query.Get("to"),
		AccountID:  query.Get("account_id"),
		CategoryID: query.Get("category_id"),
		Type:       query.Get("type"),
		Currency:   strings.ToUpper(strings.TrimSpace(query.Get("currency"))),
		TimeZone:   query.Get("tz"),
	}

	summary, err := h.repo.GetSummary(r.Context(), userID, filter)
	if err != nil {
		var invalid *InvalidFilterError
		if errors.As(err, &invalid) {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch report")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, summary)
}
