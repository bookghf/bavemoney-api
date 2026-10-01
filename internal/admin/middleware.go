package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"ledger-api/internal/httpx"
	"ledger-api/internal/validate"
)

// Admin roles. support may only read; the other roles may also change data.
const (
	RoleSupport    = "support"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"
)

type contextKey struct{}

// canWrite reports whether role may call mutating admin endpoints.
func canWrite(role string) bool {
	return role == RoleAdmin || role == RoleSuperAdmin
}

// requireAdmin guards an admin endpoint: the caller needs a valid admin token
// for an admin account that still exists, and a writing role for anything
// other than GET. Every successful mutation is recorded in admin_audit_log.
func (h *Handler) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := h.authenticator.Admin(r)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "admin authentication required")
			return
		}
		// Re-read the role so a removed or demoted admin loses access at once.
		admin, err := h.adminRepo.GetByID(r.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.WriteError(w, http.StatusUnauthorized, "admin authentication required")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "database error")
			return
		}

		readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
		if !readOnly && !canWrite(admin.Role) {
			httpx.WriteError(w, http.StatusForbidden, "your admin role can not change data")
			return
		}

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(recorder, r.WithContext(context.WithValue(r.Context(), contextKey{}, admin)))

		if !readOnly && recorder.status < 300 {
			h.audit(r, admin)
		}
	}
}

// audit records a successful mutation. A failure to log is reported but does
// not undo the change that already happened.
func (h *Handler) audit(r *http.Request, admin AdminUser) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	table := parts[0]
	var targetID interface{}
	if len(parts) > 1 && validate.UUID(parts[1]) {
		targetID = parts[1]
	}
	detail, _ := json.Marshal(map[string]string{"method": r.Method, "path": r.URL.Path})
	if err := h.adminRepo.LogAction(r.Context(), admin.ID, r.Method+" "+path, table, targetID, string(detail)); err != nil {
		log.Printf("admin audit log failed: %v", err)
	}
}

// statusRecorder captures the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}
