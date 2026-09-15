package report

import (
	"net/http"

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

	period := r.URL.Query().Get("period")
	date := r.URL.Query().Get("date")
	accountID := r.URL.Query().Get("account_id")
	currency := r.URL.Query().Get("currency")

	if period == "" || date == "" {
		httpx.WriteError(w, http.StatusBadRequest, "period and date are required")
		return
	}

	if currency == "" {
		currency = "USD"
	}

	summary, err := h.repo.GetSummary(r.Context(), userID, period, date, accountID, currency)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch report")
		return
	}

	summary.ByCategory = []CategorySummary{}
	summary.DailyBreakdown = []DailyBreakdown{}

	httpx.WriteJSON(w, http.StatusOK, summary)
}
