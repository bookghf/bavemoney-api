//go:build integration

// Package integration exercises the HTTP API end to end against a real
// PostgreSQL database. Run it with the stack up:
//
//	docker compose up -d db
//	go test -tags integration ./internal/integration/
//
// DATABASE_URL comes from the environment or money-api/.env. Every test works with
// throwaway users (qa-it+…@example.com) and deletes them afterwards.
package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ledger-api/internal/config"
	"ledger-api/internal/database"
	"ledger-api/internal/migrate"
	"ledger-api/internal/server"
)

var (
	db      *sql.DB
	handler http.Handler
	counter atomic.Int64
)

func TestMain(m *testing.M) {
	// Same git-ignored .env as docker compose (tests run in this directory).
	if err := config.LoadDotEnv("../../.env"); err != nil {
		fmt.Fprintln(os.Stderr, "load .env:", err)
		os.Exit(1)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "integration tests need DATABASE_URL (set it in money-api/.env)")
		os.Exit(1)
	}
	var err error
	if db, err = database.Connect(url); err != nil {
		fmt.Fprintln(os.Stderr, "integration tests need the database:", err)
		os.Exit(1)
	}
	if err := migrate.Run(context.Background(), db); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	// A fresh signing key per run; tokens only live for the test process.
	handler = server.Router(config.Config{JWTSecret: newEmail(), CORSAllowedOrigins: "*"}, db)
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// cleanup deletes everything the tests created.
func cleanup() {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `DELETE FROM admin_audit_log WHERE admin_id IN (SELECT id FROM admin_users WHERE email LIKE 'qa-it+%')`)
	_, _ = db.ExecContext(ctx, `DELETE FROM admin_users WHERE email LIKE 'qa-it+%'`)
	_, _ = db.ExecContext(ctx, `DELETE FROM users WHERE lower(email) LIKE 'qa-it+%'`)
	_, _ = db.ExecContext(ctx, `DELETE FROM exchange_rates WHERE base_currency = 'EUR' AND target_currency = 'THB' AND effective_date = '1999-01-01'`)
}

// response is a decoded API reply.
type response struct {
	Status int
	Body   map[string]interface{}
	Raw    string
}

func (r response) str(path ...string) string {
	var v interface{} = r.Body
	for _, key := range path {
		m, ok := v.(map[string]interface{})
		if !ok {
			return ""
		}
		v = m[key]
	}
	switch value := v.(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

// call sends one request through the router. Each call gets its own client
// address so the login rate limiter does not trip across tests.
func call(t *testing.T, method, path, token string, body interface{}) response {
	t.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		raw, _ := json.Marshal(b)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = fmt.Sprintf("10.%d.%d.1:1234", counter.Add(1)%250, time.Now().Nanosecond()%250)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := response{Status: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.Body)
	return res
}

func expect(t *testing.T, res response, status int, context string) {
	t.Helper()
	if res.Status != status {
		t.Fatalf("%s: status %d, want %d: %s", context, res.Status, status, strings.TrimSpace(res.Raw))
	}
}

// user is a registered throwaway user.
type user struct {
	ID, Email, Token, Refresh string
}

func newEmail() string {
	return fmt.Sprintf("qa-it+%d-%d@example.com", time.Now().UnixNano(), counter.Add(1))
}

func register(t *testing.T) user {
	t.Helper()
	email := newEmail()
	res := call(t, "POST", "/api/v1/auth/register", "", map[string]string{
		"email": email, "password": "correct horse", "display_name": "QA", "default_currency": "THB",
	})
	expect(t, res, http.StatusCreated, "register")
	return user{ID: res.str("user", "id"), Email: email, Token: res.str("access_token"), Refresh: res.str("refresh_token")}
}

func createAccount(t *testing.T, u user, name, currency, balance string) string {
	t.Helper()
	res := call(t, "POST", "/api/v1/accounts", u.Token, map[string]string{
		"name": name, "type": "bank", "currency": currency, "initial_balance": balance,
	})
	expect(t, res, http.StatusCreated, "create account "+name)
	return res.str("account", "id")
}

func createTx(t *testing.T, u user, body map[string]interface{}) response {
	t.Helper()
	if _, ok := body["occurred_at"]; !ok {
		body["occurred_at"] = "2026-09-10T10:00:00+07:00"
	}
	return call(t, "POST", "/api/v1/transactions", u.Token, body)
}

func balance(t *testing.T, u user, accountID string) string {
	t.Helper()
	res := call(t, "GET", "/api/v1/accounts/"+accountID, u.Token, nil)
	expect(t, res, http.StatusOK, "get account")
	return res.str("current_balance")
}

// systemCategory returns a system category id of the given type.
func systemCategory(t *testing.T, categoryType string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM categories WHERE is_system AND user_id IS NULL AND parent_id IS NULL AND type = $1 LIMIT 1`, categoryType).Scan(&id); err != nil {
		t.Fatalf("no system %s category: %v", categoryType, err)
	}
	return id
}
