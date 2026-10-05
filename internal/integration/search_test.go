//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// systemCategoryNamed returns the id of the system category called name.
func systemCategoryNamed(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM categories WHERE is_system AND user_id IS NULL AND name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("no system category %q: %v", name, err)
	}
	return id
}

// searchNotes lists the notes of the user's transactions matching query
// (already URL-encoded), sorted so tests can compare them as sets.
func searchNotes(t *testing.T, u user, query string) []string {
	t.Helper()
	res := call(t, "GET", "/api/v1/transactions?limit=100&"+query, u.Token, nil)
	expect(t, res, http.StatusOK, "search "+query)
	notes := []string{}
	for _, item := range res.Body["transactions"].([]interface{}) {
		notes = append(notes, item.(map[string]interface{})["note"].(string))
	}
	sort.Strings(notes)
	return notes
}

func TestSearchCoversNoteCategoryAccountTagsAndAmount(t *testing.T) {
	u := register(t)
	wallet := createAccount(t, u, "Wallet", "THB", "0")
	savings := createAccount(t, u, "Kasikorn Savings", "THB", "0")
	food := systemCategoryNamed(t, "Food")
	dining := systemCategoryNamed(t, "Dining Out") // a subcategory of Food
	pets := call(t, "POST", "/api/v1/categories", u.Token, map[string]string{"name": "สัตว์เลี้ยง", "type": "expense"})
	expect(t, pets, http.StatusCreated, "create Thai category")

	for _, tx := range []map[string]interface{}{
		{"note": "groceries", "account_id": wallet, "category_id": food, "type": "expense", "amount": "257"},
		{"note": "ramen", "account_id": wallet, "category_id": dining, "type": "expense", "amount": "1500"},
		{"note": "cat food", "account_id": wallet, "category_id": pets.str("id"), "type": "expense", "amount": "2570.50", "tags": []string{"Trip_2026"}},
		{"note": "50% off", "account_id": savings, "type": "income", "amount": "12.34"},
		{"note": "move", "account_id": wallet, "to_account_id": savings, "type": "transfer", "amount": "99"},
	} {
		expect(t, createTx(t, u, tx), http.StatusCreated, "create "+tx["note"].(string))
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"search=ramen", []string{"ramen"}},
		// q is an alias; matching ignores case.
		{"q=RAMEN", []string{"ramen"}},
		// Category name, including a parent matching its subcategories, and
		// a user category typed in Thai.
		{"search=food", []string{"cat food", "groceries", "ramen"}},
		{"search=dining", []string{"ramen"}},
		{"search=" + url.QueryEscape("สัตว์"), []string{"cat food"}},
		// Account name, on either side of a transfer.
		{"search=kasikorn", []string{"50% off", "move"}},
		// Tags.
		{"search=trip_2026", []string{"cat food"}},
		// Amounts by number prefix, with thousands separators and decimals.
		{"search=257", []string{"cat food", "groceries"}},
		{"search=257.00", []string{"groceries"}},
		{"search=" + url.QueryEscape("1,500"), []string{"ramen"}},
		{"search=12.3", []string{"50% off"}},
		// LIKE wildcards are literal.
		{"search=" + url.QueryEscape("%"), []string{"50% off"}},
		{"search=" + url.QueryEscape("_"), []string{"cat food"}},
		// Category IDs resolved by the app (e.g. "อาหาร" -> Food) widen the
		// search, a parent bringing its subcategories along.
		{"search=" + url.QueryEscape("อาหาร") + "&search_categories=" + food, []string{"groceries", "ramen"}},
		// Search combines with the other filters.
		{"search=food&type=expense&account=" + wallet, []string{"cat food", "groceries", "ramen"}},
		{"search=nothing-like-this", []string{}},
	}
	for _, tt := range tests {
		if got := searchNotes(t, u, tt.query); strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: got %q, want %q", tt.query, got, tt.want)
		}
	}

	expect(t, call(t, "GET", "/api/v1/transactions?search=x&search_categories=nope", u.Token, nil),
		http.StatusBadRequest, "bad search_categories")
}

func TestSearchNeverReachesAnotherUsersTransactions(t *testing.T) {
	owner, other := register(t), register(t)
	ownerAccount := createAccount(t, owner, "Secret Stash", "THB", "0")
	hobby := call(t, "POST", "/api/v1/categories", owner.Token, map[string]string{"name": "Hidden Hobby", "type": "expense"})
	expect(t, hobby, http.StatusCreated, "owner category")
	expect(t, createTx(t, owner, map[string]interface{}{
		"note": "private", "account_id": ownerAccount, "category_id": hobby.str("id"),
		"type": "expense", "amount": "4242", "tags": []string{"confidential"},
	}), http.StatusCreated, "owner transaction")

	otherAccount := createAccount(t, other, "Mine", "THB", "0")
	expect(t, createTx(t, other, map[string]interface{}{"note": "own", "account_id": otherAccount, "type": "expense", "amount": "1"}),
		http.StatusCreated, "other transaction")

	for _, query := range []string{
		"search=private", "search=secret", "search=hidden", "search=confidential", "search=4242",
		// Passing the owner's category id does not reach the owner's rows.
		"search=x&search_categories=" + hobby.str("id"),
	} {
		if got := searchNotes(t, other, query); len(got) != 0 {
			t.Errorf("other user %s: got %q, want nothing", query, got)
		}
	}
	if got := searchNotes(t, owner, "search=4242"); len(got) != 1 {
		t.Errorf("owner search=4242: got %q, want the one transaction", got)
	}
}
