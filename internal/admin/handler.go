package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ledger-api/internal/auth"
	"ledger-api/internal/database"
	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

type Handler struct {
	adminRepo     *AdminRepository
	userRepo      *UserRepository
	categoryRepo  *CategoryRepository
	currencyRepo  *CurrencyRepository
	analyticsRepo *AnalyticsRepository
	authenticator *auth.Authenticator
}

func NewHandler(db *sql.DB, authenticator *auth.Authenticator) *Handler {
	return &Handler{
		adminRepo:     NewAdminRepository(db),
		userRepo:      NewUserRepository(db),
		categoryRepo:  NewCategoryRepository(db),
		currencyRepo:  NewCurrencyRepository(db),
		analyticsRepo: NewAnalyticsRepository(db),
		authenticator: authenticator,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	// Login is the only admin route open without an admin token.
	mux.HandleFunc("/api/v1/admin/auth/login", h.login)
	mux.HandleFunc("/api/v1/admin/users", h.requireAdmin(h.usersList))
	mux.HandleFunc("/api/v1/admin/users/", h.requireAdmin(h.usersRouter))
	mux.HandleFunc("/api/v1/admin/categories", h.requireAdmin(h.categoriesRouter))
	mux.HandleFunc("/api/v1/admin/categories/", h.requireAdmin(h.categoriesItemRouter))
	mux.HandleFunc("/api/v1/admin/currencies", h.requireAdmin(h.currenciesRouter))
	mux.HandleFunc("/api/v1/admin/currencies/", h.requireAdmin(h.currenciesItemRouter))
	mux.HandleFunc("/api/v1/admin/exchange-rates", h.requireAdmin(h.exchangeRatesRouter))
	mux.HandleFunc("/api/v1/admin/analytics/overview", h.requireAdmin(h.analyticsOverview))
	mux.HandleFunc("/api/v1/admin/analytics/activity", h.requireAdmin(h.analyticsActivity))
	// Anything else under /admin/ is unknown, but must not leak whether it
	// exists to an unauthenticated caller.
	mux.HandleFunc("/api/v1/admin/", h.requireAdmin(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
	}))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	var req AdminLoginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if req.Email == "" || req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	admin, hash, err := h.adminRepo.GetByEmail(r.Context(), validate.NormalizeEmail(req.Email))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	if err := h.adminRepo.VerifyPassword(hash, req.Password); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, err := h.authenticator.IssueAdminToken(admin.ID, admin.Email, admin.Role)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	// Admins get no refresh token: when the session expires they sign in again.
	resp := AdminAuthResponse{
		Admin:       admin,
		AccessToken: accessToken,
		ExpiresIn:   int(auth.AdminTokenTTL.Seconds()),
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) usersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "-created_at"
	}

	users, total, err := h.userRepo.List(r.Context(), page, limit, status, search, sort)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list users")
		return
	}

	totalPages := (total + limit - 1) / limit
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total_items": total,
			"total_pages": totalPages,
		},
	})
}

func (h *Handler) usersRouter(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	userID = strings.TrimSuffix(userID, "/")

	if strings.HasSuffix(r.URL.Path, "/suspend") {
		userID = strings.TrimSuffix(userID, "/suspend")
	}
	if !validate.UUID(userID) {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if strings.HasSuffix(r.URL.Path, "/suspend") {
		h.suspendUser(w, r, userID)
	} else if r.Method == http.MethodGet {
		h.getUser(w, r, userID)
	} else {
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request, userID string) {
	detail, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) suspendUser(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodPatch {
		httpx.MethodNotAllowed(w)
		return
	}

	var req SuspendRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if req.Status != "suspended" && req.Status != "active" {
		httpx.WriteError(w, http.StatusBadRequest, "status must be 'suspended' or 'active'")
		return
	}

	if err := h.userRepo.Suspend(r.Context(), userID, req.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not update user")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"status": req.Status,
		},
		"message": "User " + req.Status,
	})
}

func (h *Handler) categoriesRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.listCategories(w, r)
	} else if r.Method == http.MethodPost {
		h.createCategory(w, r)
	} else {
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) categoriesItemRouter(w http.ResponseWriter, r *http.Request) {
	categoryID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/categories/")
	categoryID = strings.TrimSuffix(categoryID, "/")

	if !validate.UUID(categoryID) {
		httpx.WriteError(w, http.StatusNotFound, "category not found")
		return
	}

	if r.Method == http.MethodPatch {
		h.updateCategory(w, r, categoryID)
	} else if r.Method == http.MethodDelete {
		h.deleteCategory(w, r, categoryID)
	} else {
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.categoryRepo.ListSystem(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list categories")
		return
	}

	if categories == nil {
		categories = []CategoryInfo{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"categories": categories})
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.Name) == "" || (req.Type != "income" && req.Type != "expense") {
		httpx.WriteError(w, http.StatusBadRequest, "name is required and type must be income or expense")
		return
	}
	if req.ParentID != nil && !validate.UUID(*req.ParentID) {
		httpx.WriteError(w, http.StatusBadRequest, "parent_id must be a UUID")
		return
	}

	c, err := h.categoryRepo.Create(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create category")
		return
	}

	c.Subcategories = []CategoryInfo{}
	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"category": c})
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request, categoryID string) {
	var req map[string]interface{}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if err := h.categoryRepo.Update(r.Context(), categoryID, req); err != nil {
		switch {
		case errors.Is(err, errNoFields):
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, sql.ErrNoRows):
			httpx.WriteError(w, http.StatusNotFound, "category not found")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not update category")
		}
		return
	}

	httpx.WriteMessage(w, http.StatusOK, "category updated")
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request, categoryID string) {
	count, err := h.categoryRepo.Delete(r.Context(), categoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "category not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete category")
		return
	}
	if count > 0 {
		httpx.WriteJSON(w, http.StatusConflict, map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "CATEGORY_IN_USE",
				"message": fmt.Sprintf("Cannot delete category: %d transactions reference it", count),
				"details": map[string]int{"transaction_count": count},
			},
		})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Category deleted",
		"id":      categoryID,
	})
}

func (h *Handler) currenciesRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.listCurrencies(w, r)
	} else if r.Method == http.MethodPost {
		h.createCurrency(w, r)
	} else {
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) currenciesItemRouter(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/currencies/")
	code = strings.TrimSuffix(code, "/")

	if code == "" || strings.Contains(code, "/") {
		httpx.WriteError(w, http.StatusNotFound, "currency not found")
		return
	}

	if r.Method == http.MethodPatch {
		h.updateCurrency(w, r, code)
	} else {
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) listCurrencies(w http.ResponseWriter, r *http.Request) {
	var isActive *bool
	if val := r.URL.Query().Get("is_active"); val != "" {
		b := val == "true"
		isActive = &b
	}

	currencies, err := h.currencyRepo.List(r.Context(), isActive)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list currencies")
		return
	}

	if currencies == nil {
		currencies = []CurrencyInfo{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"currencies": currencies})
}

func (h *Handler) createCurrency(w http.ResponseWriter, r *http.Request) {
	var req CreateCurrencyRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if !validate.Currency(req.Code) || req.Name == "" || req.Symbol == "" {
		httpx.WriteError(w, http.StatusBadRequest, "code (3 letters), name, and symbol are required")
		return
	}

	c, err := h.currencyRepo.Create(r.Context(), req)
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpx.WriteError(w, http.StatusConflict, "currency already exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not create currency")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"currency": c})
}

func (h *Handler) updateCurrency(w http.ResponseWriter, r *http.Request, code string) {
	var req UpdateCurrencyRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if err := h.currencyRepo.Update(r.Context(), code, req); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "currency not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not update currency")
		return
	}

	httpx.WriteMessage(w, http.StatusOK, "currency updated")
}

func (h *Handler) exchangeRatesRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.listExchangeRates(w, r)
	} else if r.Method == http.MethodPost {
		h.createExchangeRate(w, r)
	} else {
		httpx.MethodNotAllowed(w)
	}
}

func (h *Handler) listExchangeRates(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	base := r.URL.Query().Get("base")
	target := r.URL.Query().Get("target")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	rates, total, err := h.currencyRepo.ListExchangeRates(r.Context(), base, target, from, to, page, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list exchange rates")
		return
	}

	if rates == nil {
		rates = []ExchangeRate{}
	}

	totalPages := (total + limit - 1) / limit
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"rates": rates,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total_items": total,
			"total_pages": totalPages,
		},
	})
}

func (h *Handler) createExchangeRate(w http.ResponseWriter, r *http.Request) {
	var req CreateExchangeRateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if !validate.Currency(req.BaseCurrency) || !validate.Currency(req.TargetCurrency) || req.BaseCurrency == req.TargetCurrency {
		httpx.WriteError(w, http.StatusBadRequest, "base_currency and target_currency must be two different currency codes")
		return
	}
	if rate, err := strconv.ParseFloat(req.Rate, 64); err != nil || rate <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "rate must be a positive decimal")
		return
	}
	if _, ok := validate.Date(req.EffectiveDate); !ok {
		httpx.WriteError(w, http.StatusBadRequest, "effective_date must be YYYY-MM-DD")
		return
	}

	rate, err := h.currencyRepo.CreateExchangeRate(r.Context(), req)
	if database.IsForeignKeyViolation(err) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown currency")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create exchange rate")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"rate": rate})
}

func (h *Handler) analyticsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}

	overview, err := h.analyticsRepo.GetOverview(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch analytics")
		return
	}

	if overview.TopCurrencies == nil {
		overview.TopCurrencies = []CurrencyCount{}
	}

	httpx.WriteJSON(w, http.StatusOK, overview)
}

func (h *Handler) analyticsActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "daily"
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		}
	}

	dataPoints, err := h.analyticsRepo.GetActivity(r.Context(), period, from, to)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch activity data")
		return
	}

	if dataPoints == nil {
		dataPoints = []ActivityDataPoint{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"period":      period,
		"data_points": dataPoints,
	})
}
