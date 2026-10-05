//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func reconcile(t *testing.T, u user, accountID string, balance interface{}) response {
	t.Helper()
	return call(t, "POST", "/api/v1/accounts/"+accountID+"/reconcile", u.Token, map[string]interface{}{"balance": balance})
}

func TestReconcileMovesOpeningBalanceOnly(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "Daily Life", "THB", "100.00")
	expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "type": "income", "amount": "50.10"}), http.StatusCreated, "income")
	expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "12340.35"}), http.StatusCreated, "expense")
	if got := balance(t, u, account); got != "-12190.25" {
		t.Fatalf("balance before = %s, want -12190.25", got)
	}
	summary := func() (string, string) {
		res := call(t, "GET", "/api/v1/reports/summary?period=month&date=2026-09&tz=Asia/Bangkok", u.Token, nil)
		expect(t, res, http.StatusOK, "summary")
		return res.str("total_income"), res.str("total_expense")
	}
	incomeBefore, expenseBefore := summary()

	res := reconcile(t, u, account, "1234.56")
	expect(t, res, http.StatusOK, "reconcile")
	if got := res.str("account", "current_balance"); got != "1234.56" {
		t.Errorf("current_balance = %s, want 1234.56", got)
	}
	// 1234.56 - (50.10 - 12340.35) = 13524.81
	if got := res.str("account", "initial_balance"); got != "13524.81" {
		t.Errorf("initial_balance = %s, want 13524.81", got)
	}

	// No transaction was added, so reports are unchanged.
	if income, expense := summary(); income != incomeBefore || expense != expenseBefore {
		t.Errorf("report moved: income %s -> %s, expense %s -> %s", incomeBefore, income, expenseBefore, expense)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM transactions WHERE account_id = $1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("transactions = %d, want 2", count)
	}

	// Transfers count on both sides.
	other := createAccount(t, u, "Savings", "THB", "0")
	expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "to_account_id": other, "type": "transfer", "amount": "200"}), http.StatusCreated, "transfer")
	expect(t, reconcile(t, u, other, "500"), http.StatusOK, "reconcile receiving side")
	if got := balance(t, u, other); got != "500.00" {
		t.Errorf("savings balance = %s, want 500.00", got)
	}
	if got := balance(t, u, account); got != "1034.56" {
		t.Errorf("daily life balance after transfer = %s, want 1034.56", got)
	}
}

func TestReconcileOpeningBalanceMayGoNegative(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "Wallet", "THB", "0")
	expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "type": "income", "amount": "1000"}), http.StatusCreated, "income")

	// History records more income than the wallet holds: the opening balance
	// becomes negative so the current balance matches reality.
	res := reconcile(t, u, account, "300")
	expect(t, res, http.StatusOK, "reconcile below recorded income")
	if res.str("account", "initial_balance") != "-700.00" || res.str("account", "current_balance") != "300.00" {
		t.Errorf("got initial %s current %s", res.str("account", "initial_balance"), res.str("account", "current_balance"))
	}
	// Other edits still work on that account.
	expect(t, call(t, "PATCH", "/api/v1/accounts/"+account, u.Token, map[string]string{"name": "Wallet 2", "color": "violet"}),
		http.StatusOK, "rename after negative reconcile")
}

func TestReconcileValidation(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "0")
	cardRes := call(t, "POST", "/api/v1/accounts", u.Token, map[string]string{"name": "Visa", "type": "credit_card", "currency": "THB"})
	expect(t, cardRes, http.StatusCreated, "create card")
	card := cardRes.str("account", "id")

	for name, value := range map[string]interface{}{
		"three decimals":  "1.001",
		"exponent":        "1e5",
		"not a number":    "abc",
		"negative bank":   "-10",
		"too many digits": "12345678901234567",
	} {
		expect(t, reconcile(t, u, bank, value), http.StatusBadRequest, name)
	}
	expect(t, call(t, "POST", "/api/v1/accounts/"+bank+"/reconcile", u.Token, map[string]string{}), http.StatusBadRequest, "missing balance")
	if got := balance(t, u, bank); got != "0.00" {
		t.Errorf("rejected reconciles changed the balance to %s", got)
	}

	// JSON numbers are accepted as exact decimals too.
	expect(t, reconcile(t, u, bank, 99.5), http.StatusOK, "numeric balance")
	if got := balance(t, u, bank); got != "99.50" {
		t.Errorf("bank balance = %s, want 99.50", got)
	}

	res := reconcile(t, u, card, "-1500.50")
	expect(t, res, http.StatusOK, "negative credit card")
	if got := res.str("account", "current_balance"); got != "-1500.50" {
		t.Errorf("card balance = %s", got)
	}

	other := register(t)
	expect(t, reconcile(t, other, bank, "1"), http.StatusNotFound, "someone else's account")
	expect(t, call(t, "POST", "/api/v1/accounts/not-a-uuid/reconcile", u.Token, map[string]string{"balance": "1"}), http.StatusNotFound, "bad id")
	expect(t, call(t, "GET", "/api/v1/accounts/"+bank+"/reconcile", u.Token, nil), http.StatusMethodNotAllowed, "GET reconcile")
}

// A transaction insert that is in flight when the reconcile starts must be
// counted: the reconcile waits for it instead of reading a stale sum.
func TestReconcileWaitsForConcurrentInsert(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "Bank", "THB", "0")

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	// The same share lock the transaction repository takes before inserting.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM accounts WHERE id = $1 FOR SHARE`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transactions (user_id, account_id, type, amount, currency, occurred_at)
		VALUES ($1, $2, 'expense', 40, 'THB', now())
	`, u.ID, account); err != nil {
		t.Fatal(err)
	}

	done := make(chan response, 1)
	go func() { done <- reconcile(t, u, account, "1000") }()
	select {
	case res := <-done:
		t.Fatalf("reconcile finished while an insert was in flight: %d %s", res.Status, res.Raw)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-done:
		expect(t, res, http.StatusOK, "reconcile")
	case <-time.After(10 * time.Second):
		t.Fatal("reconcile never finished")
	}
	if got := balance(t, u, account); got != "1000.00" {
		t.Errorf("balance = %s, want 1000.00 (the concurrent expense was missed)", got)
	}
}

func TestAccountColor(t *testing.T) {
	u := register(t)
	res := call(t, "POST", "/api/v1/accounts", u.Token, map[string]string{"name": "Cash", "type": "cash", "currency": "THB", "color": "lime"})
	expect(t, res, http.StatusCreated, "create with color")
	id := res.str("account", "id")
	if res.str("account", "color") != "lime" {
		t.Errorf("color = %q, want lime", res.str("account", "color"))
	}

	plain := createAccount(t, u, "Plain", "THB", "0")
	res = call(t, "GET", "/api/v1/accounts/"+plain, u.Token, nil)
	expect(t, res, http.StatusOK, "get plain")
	if _, ok := res.Body["color"]; ok {
		t.Errorf("account without a color returned one: %s", res.Raw)
	}

	expect(t, call(t, "POST", "/api/v1/accounts", u.Token, map[string]string{"name": "x", "type": "cash", "currency": "THB", "color": "teal"}),
		http.StatusBadRequest, "unknown color on create")
	expect(t, call(t, "PATCH", "/api/v1/accounts/"+id, u.Token, map[string]string{"color": "#ff0000"}),
		http.StatusBadRequest, "unknown color on update")

	res = call(t, "PATCH", "/api/v1/accounts/"+id, u.Token, map[string]string{"color": "fuchsia"})
	expect(t, res, http.StatusOK, "set color")
	if res.str("account", "color") != "fuchsia" {
		t.Errorf("color after update = %q", res.str("account", "color"))
	}

	res = call(t, "GET", "/api/v1/accounts", u.Token, nil)
	expect(t, res, http.StatusOK, "list")
	found := false
	for _, raw := range res.Body["accounts"].([]interface{}) {
		a := raw.(map[string]interface{})
		if a["id"] == id {
			found = a["color"] == "fuchsia"
		}
	}
	if !found {
		t.Errorf("list does not carry the color: %s", res.Raw)
	}

	// An empty string clears it back to the type's default.
	res = call(t, "PATCH", "/api/v1/accounts/"+id, u.Token, map[string]string{"color": ""})
	expect(t, res, http.StatusOK, "clear color")
	if res.str("account", "color") != "" {
		t.Errorf("color not cleared: %s", res.Raw)
	}
}
