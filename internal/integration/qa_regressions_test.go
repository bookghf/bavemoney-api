//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

// Regression tests for the second QA review (2026-10-01).

func TestPasswordPolicy(t *testing.T) {
	u := register(t)
	for name, pw := range map[string]string{
		"whitespace only": "        ",
		"over 72 bytes":   strings.Repeat("a", 80),
		"control char":    "abc\x00defgh",
	} {
		expect(t, call(t, "POST", "/api/v1/me/password", u.Token, map[string]string{"current_password": "correct horse", "new_password": pw}),
			http.StatusBadRequest, "change to "+name)
		expect(t, call(t, "POST", "/api/v1/auth/register", "", map[string]string{"email": newEmail(), "password": pw}),
			http.StatusBadRequest, "register with "+name)
	}
	// The account is still usable with the original password.
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"}),
		http.StatusOK, "login after rejected changes")
}

func TestSuspendedUserLosesAccessImmediately(t *testing.T) {
	u := register(t)
	if _, err := db.Exec(`UPDATE users SET status = 'suspended' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	// The access token is still unexpired, but every call is refused and no
	// new token can be minted.
	expect(t, call(t, "GET", "/api/v1/accounts", u.Token, nil), http.StatusForbidden, "read")
	expect(t, call(t, "POST", "/api/v1/accounts", u.Token, map[string]string{"name": "x", "type": "cash", "currency": "THB"}),
		http.StatusForbidden, "write")
	res := call(t, "POST", "/api/v1/me/password", u.Token, map[string]string{"current_password": "correct horse", "new_password": "battery staple"})
	expect(t, res, http.StatusForbidden, "change password")
	if res.str("access_token") != "" {
		t.Error("a suspended user was issued a token")
	}

	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, "GET", "/api/v1/accounts", u.Token, nil), http.StatusUnauthorized, "deleted user")
}

func TestControlCharactersAreRejected(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	cases := []struct {
		method, path string
		body         interface{}
	}{
		{"PATCH", "/api/v1/me", map[string]string{"display_name": "a\x00b"}},
		{"PATCH", "/api/v1/me", map[string]string{"display_name": "line\nbreak"}},
		{"POST", "/api/v1/auth/register", map[string]string{"email": newEmail(), "password": "correct horse", "display_name": "a\x00b"}},
		{"POST", "/api/v1/auth/register", map[string]string{"email": newEmail(), "password": "correct horse", "display_name": strings.Repeat("x", 61)}},
		{"POST", "/api/v1/accounts", map[string]string{"name": "a\x00b", "type": "cash", "currency": "THB"}},
		{"POST", "/api/v1/categories", map[string]string{"name": "a\x00b", "type": "expense"}},
		{"POST", "/api/v1/transactions", map[string]interface{}{"account_id": account, "type": "expense", "amount": "1", "note": "a\x00b", "occurred_at": "2026-09-01T10:00:00Z"}},
	}
	for _, c := range cases {
		expect(t, call(t, c.method, c.path, u.Token, c.body), http.StatusBadRequest, c.method+" "+c.path)
	}
	// Notes may still span lines.
	expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "1", "note": "line one\nline two"}),
		http.StatusCreated, "multi-line note")
}

func TestResetCountsOnlyVisibleTransactions(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	kept := createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "1"})
	gone := createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "2"})
	_ = kept
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+gone.str("id"), u.Token, nil), http.StatusOK, "soft delete")

	res := call(t, "POST", "/api/v1/me/reset", u.Token, map[string]string{"password": "correct horse"})
	expect(t, res, http.StatusOK, "reset")
	if got := res.str("deleted", "transactions"); got != "1" {
		t.Errorf("deleted.transactions = %s, want 1 (the visible one)", got)
	}
	var left int
	_ = db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = $1`, u.ID).Scan(&left)
	if left != 0 {
		t.Errorf("%d transactions left in the database, soft-deleted ones included", left)
	}
}

func TestInputConsistency(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")

	// Amounts may be JSON numbers as well as strings.
	res := call(t, "POST", "/api/v1/transactions", u.Token, map[string]interface{}{
		"account_id": account, "type": "expense", "amount": 12.5, "currency": "thb", "occurred_at": "2026-09-01T10:00:00+07:00",
	})
	expect(t, res, http.StatusCreated, "numeric amount, lowercase currency")
	if res.str("amount") != "12.50" || res.str("currency") != "THB" {
		t.Errorf("stored %s %s", res.str("amount"), res.str("currency"))
	}
	expect(t, call(t, "POST", "/api/v1/budgets", u.Token, map[string]interface{}{
		"amount": 1000, "currency": "THB", "period": "monthly", "start_date": "2026-09-01",
	}), http.StatusCreated, "numeric budget amount")

	// Reports accept a lowercase currency, like /me does.
	report := call(t, "GET", "/api/v1/reports/summary?period=month&date=2026-09&currency=thb&tz=Asia/Bangkok", u.Token, nil)
	expect(t, report, http.StatusOK, "lowercase report currency")
	if report.str("total_expense") != "12.50" {
		t.Errorf("total_expense = %s", report.str("total_expense"))
	}

	// Transactions take the report-style filter names too.
	list := call(t, "GET", "/api/v1/transactions?account_id="+account, u.Token, nil)
	expect(t, list, http.StatusOK, "account_id filter")
	if list.str("pagination", "total_items") != "1" {
		t.Errorf("account_id filter matched %s", list.str("pagination", "total_items"))
	}
	for _, q := range []string{"tz=Local", "from=2026-09-30&to=2026-09-01", "sort=nonsense"} {
		expect(t, call(t, "GET", "/api/v1/transactions?"+q, u.Token, nil), http.StatusBadRequest, q)
	}
}
