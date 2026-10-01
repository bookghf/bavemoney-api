//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

func TestEditProfile(t *testing.T) {
	u := register(t)

	res := call(t, "GET", "/api/v1/me", u.Token, nil)
	expect(t, res, http.StatusOK, "get profile")
	if res.str("user", "email") != u.Email {
		t.Errorf("email = %q", res.str("user", "email"))
	}

	res = call(t, "PATCH", "/api/v1/me", u.Token, map[string]string{"display_name": "  Somchai  ", "default_currency": "usd"})
	expect(t, res, http.StatusOK, "update profile")
	if res.str("user", "display_name") != "Somchai" || res.str("user", "default_currency") != "USD" {
		t.Errorf("profile = %v", res.Body["user"])
	}

	// Only the given fields change.
	res = call(t, "PATCH", "/api/v1/me", u.Token, map[string]string{"display_name": "Som"})
	expect(t, res, http.StatusOK, "update name only")
	if res.str("user", "default_currency") != "USD" {
		t.Errorf("currency changed to %q", res.str("user", "default_currency"))
	}

	for name, body := range map[string]interface{}{
		"no fields":        map[string]string{},
		"unknown currency": map[string]string{"default_currency": "XYZ"},
		"long name":        map[string]string{"display_name": strings.Repeat("x", 61)},
	} {
		expect(t, call(t, "PATCH", "/api/v1/me", u.Token, body), http.StatusBadRequest, name)
	}
	expect(t, call(t, "GET", "/api/v1/me", "", nil), http.StatusUnauthorized, "no token")
}

func TestChangePassword(t *testing.T) {
	u := register(t)
	other := call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"})
	expect(t, other, http.StatusOK, "second device logs in")

	expect(t, call(t, "POST", "/api/v1/me/password", u.Token, map[string]string{"current_password": "wrong one", "new_password": "battery staple"}),
		http.StatusBadRequest, "wrong current password")
	expect(t, call(t, "POST", "/api/v1/me/password", u.Token, map[string]string{"current_password": "correct horse", "new_password": "short"}),
		http.StatusBadRequest, "short new password")

	res := call(t, "POST", "/api/v1/me/password", u.Token, map[string]string{"current_password": "correct horse", "new_password": "battery staple"})
	expect(t, res, http.StatusOK, "change password")
	if res.str("access_token") == "" || res.str("refresh_token") == "" {
		t.Fatal("this device must get a fresh session")
	}

	expect(t, call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": other.str("refresh_token")}),
		http.StatusUnauthorized, "other device was signed out")
	expect(t, call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": res.str("refresh_token")}),
		http.StatusOK, "new session works")
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"}),
		http.StatusUnauthorized, "old password")
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "battery staple"}),
		http.StatusOK, "new password")
}

func TestResetAccount(t *testing.T) {
	u, other := register(t), register(t)
	account := createAccount(t, u, "Wallet", "THB", "100")
	otherAccount := createAccount(t, other, "Other", "THB", "0")
	createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "17"})
	createTx(t, other, map[string]interface{}{"account_id": otherAccount, "type": "expense", "amount": "5"})
	parent := call(t, "POST", "/api/v1/categories", u.Token, map[string]string{"name": "Pets", "type": "expense"}).str("id")
	expect(t, call(t, "POST", "/api/v1/categories", u.Token, map[string]string{"name": "Cat food", "type": "expense", "parent_id": parent}),
		http.StatusCreated, "subcategory")
	expect(t, call(t, "POST", "/api/v1/budgets", u.Token, map[string]interface{}{
		"amount": "1000", "currency": "THB", "period": "monthly", "start_date": "2026-10-01",
	}), http.StatusCreated, "budget")

	expect(t, call(t, "POST", "/api/v1/me/reset", u.Token, map[string]string{"password": "wrong password"}),
		http.StatusBadRequest, "wrong password")
	expect(t, call(t, "POST", "/api/v1/me/reset", u.Token, map[string]string{}), http.StatusBadRequest, "no password")
	expect(t, call(t, "POST", "/api/v1/me/reset", "", map[string]string{"password": "correct horse"}),
		http.StatusUnauthorized, "no token")
	if n := len(call(t, "GET", "/api/v1/accounts", u.Token, nil).Body["accounts"].([]interface{})); n != 1 {
		t.Fatalf("a failed reset deleted data: %d accounts left", n)
	}

	res := call(t, "POST", "/api/v1/me/reset", u.Token, map[string]string{"password": "correct horse"})
	expect(t, res, http.StatusOK, "reset")
	for field, want := range map[string]string{"transactions": "1", "accounts": "1", "budgets": "1", "categories": "2"} {
		if got := res.str("deleted", field); got != want {
			t.Errorf("deleted %s = %s, want %s", field, got, want)
		}
	}

	if n := len(call(t, "GET", "/api/v1/accounts?include_archived=true", u.Token, nil).Body["accounts"].([]interface{})); n != 0 {
		t.Errorf("%d accounts left after reset", n)
	}
	if got := call(t, "GET", "/api/v1/transactions", u.Token, nil).str("pagination", "total_items"); got != "0" {
		t.Errorf("%s transactions left after reset", got)
	}
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"}),
		http.StatusOK, "login survives the reset")
	if got := call(t, "GET", "/api/v1/transactions", other.Token, nil).str("pagination", "total_items"); got != "1" {
		t.Errorf("another user's data was touched: %s transactions", got)
	}
}
