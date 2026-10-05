//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// balances reads several accounts' current balances in order.
func balances(t *testing.T, u user, ids ...string) []string {
	t.Helper()
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = balance(t, u, id)
	}
	return out
}

func expectBalances(t *testing.T, u user, context string, ids []string, want ...string) {
	t.Helper()
	got := balances(t, u, ids...)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: balances = %v, want %v", context, got, want)
		}
	}
}

func patchTx(t *testing.T, u user, id string, body map[string]interface{}) response {
	t.Helper()
	return call(t, "PATCH", "/api/v1/transactions/"+id, u.Token, body)
}

func TestEditMovesATransactionBetweenAccountsAndTypes(t *testing.T) {
	u := register(t)
	cash := createAccount(t, u, "Cash", "THB", "1000.00")
	bank := createAccount(t, u, "Bank", "THB", "500.00")
	wallet := createAccount(t, u, "Wallet", "THB", "0.00")
	ids := []string{cash, bank, wallet}

	res := createTx(t, u, map[string]interface{}{"account_id": cash, "category_id": systemCategory(t, "expense"), "type": "expense", "amount": "120.50"})
	expect(t, res, http.StatusCreated, "expense on cash")
	id := res.str("id")
	expectBalances(t, u, "after create", ids, "879.50", "500.00", "0.00")

	// Move it to another account.
	res = patchTx(t, u, id, map[string]interface{}{"account_id": bank})
	expect(t, res, http.StatusOK, "move to bank")
	if res.str("account_name") != "Bank" {
		t.Errorf("account_name = %q, want Bank", res.str("account_name"))
	}
	expectBalances(t, u, "after move", ids, "1000.00", "379.50", "0.00")

	// Expense -> income needs a matching (or no) category.
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "income"}), http.StatusBadRequest, "income keeping an expense category")
	res = patchTx(t, u, id, map[string]interface{}{"type": "income", "category_id": systemCategory(t, "income")})
	expect(t, res, http.StatusOK, "expense -> income")
	expectBalances(t, u, "after income", ids, "1000.00", "620.50", "0.00")

	// Income -> transfer: needs a target, drops the category.
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "transfer"}), http.StatusBadRequest, "transfer without a target")
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "transfer", "to_account_id": bank}), http.StatusBadRequest, "transfer to itself")
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "transfer", "to_account_id": wallet, "category_id": systemCategory(t, "income")}),
		http.StatusBadRequest, "transfer with a category")
	res = patchTx(t, u, id, map[string]interface{}{"type": "transfer", "to_account_id": wallet})
	expect(t, res, http.StatusOK, "income -> transfer")
	if res.Body["category"] != nil || res.str("to_account_id") != wallet {
		t.Errorf("transfer = %s, want no category and to_account_id %s", res.Raw, wallet)
	}
	expectBalances(t, u, "after transfer", ids, "1000.00", "379.50", "120.50")

	// Change both sides of the transfer.
	res = patchTx(t, u, id, map[string]interface{}{"account_id": wallet, "to_account_id": cash, "amount": "20.00"})
	expect(t, res, http.StatusOK, "re-route the transfer")
	expectBalances(t, u, "after re-route", ids, "1020.00", "500.00", "-20.00")

	// Transfer -> expense drops the receiving account.
	res = patchTx(t, u, id, map[string]interface{}{"type": "expense", "account_id": cash})
	expect(t, res, http.StatusOK, "transfer -> expense")
	if res.str("to_account_id") != "" {
		t.Errorf("to_account_id = %q, want none", res.str("to_account_id"))
	}
	expectBalances(t, u, "after expense", ids, "980.00", "500.00", "0.00")

	// Deleting it restores every opening balance.
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+id, u.Token, nil), http.StatusOK, "delete")
	expectBalances(t, u, "after delete", ids, "1000.00", "500.00", "0.00")
}

func TestEditRejectsForeignArchivedAndMismatchedAccounts(t *testing.T) {
	u, other := register(t), register(t)
	cash := createAccount(t, u, "Cash", "THB", "100.00")
	bank := createAccount(t, u, "Bank", "THB", "0.00")
	usd := createAccount(t, u, "Dollars", "USD", "0.00")
	archived := createAccount(t, u, "Old", "THB", "0.00")
	theirs := createAccount(t, other, "Theirs", "THB", "0.00")

	res := createTx(t, u, map[string]interface{}{"account_id": cash, "type": "expense", "amount": "10"})
	expect(t, res, http.StatusCreated, "expense")
	id := res.str("id")
	res = createTx(t, u, map[string]interface{}{"account_id": archived, "type": "income", "amount": "5"})
	expect(t, res, http.StatusCreated, "income on the soon-archived account")
	onArchived := res.str("id")
	expect(t, call(t, "DELETE", "/api/v1/accounts/"+archived, u.Token, nil), http.StatusOK, "archive")

	expect(t, patchTx(t, u, id, map[string]interface{}{"account_id": theirs}), http.StatusBadRequest, "move to another user's account")
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "transfer", "to_account_id": theirs}), http.StatusBadRequest, "transfer to another user's account")
	expect(t, patchTx(t, u, id, map[string]interface{}{"account_id": archived}), http.StatusBadRequest, "move to an archived account")
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "transfer", "to_account_id": archived}), http.StatusBadRequest, "transfer to an archived account")
	expect(t, patchTx(t, u, id, map[string]interface{}{"type": "transfer", "to_account_id": usd}), http.StatusBadRequest, "transfer across currencies")
	// The other user can not edit it at all.
	expect(t, call(t, "PATCH", "/api/v1/transactions/"+id, other.Token, map[string]interface{}{"account_id": theirs}), http.StatusNotFound, "foreign edit")

	// Nothing above changed a balance.
	expectBalances(t, u, "after rejected edits", []string{cash, bank}, "90.00", "0.00")
	if got := balance(t, other, theirs); got != "0.00" {
		t.Errorf("other user's balance = %s, want 0.00", got)
	}

	// An expense moved to a USD account takes that account's currency.
	res = patchTx(t, u, id, map[string]interface{}{"account_id": usd})
	expect(t, res, http.StatusOK, "move to the USD account")
	if res.str("currency") != "USD" {
		t.Errorf("currency = %s, want USD", res.str("currency"))
	}
	expectBalances(t, u, "after USD move", []string{cash, usd}, "100.00", "-10.00")

	// A transaction already on an archived account can still be edited, and
	// moved off it, but not turned into a transfer into another archived one.
	expect(t, patchTx(t, u, onArchived, map[string]interface{}{"amount": "6"}), http.StatusOK, "edit on an archived account")
	expect(t, patchTx(t, u, onArchived, map[string]interface{}{"account_id": bank}), http.StatusOK, "move off the archived account")
	expectBalances(t, u, "after moving off archived", []string{bank, archived}, "6.00", "0.00")
}
