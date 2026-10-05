//go:build integration

package integration

import (
	"encoding/csv"
	"net/http"
	"strings"
	"testing"
)

func categoryID(t *testing.T, u user, name, categoryType string) string {
	t.Helper()
	res := call(t, "GET", "/api/v1/categories", u.Token, nil)
	for _, raw := range res.Body["categories"].([]interface{}) {
		c := raw.(map[string]interface{})
		if c["name"] == name && c["type"] == categoryType {
			return c["id"].(string)
		}
	}
	t.Fatalf("category %s/%s not found", name, categoryType)
	return ""
}

func TestBuiltInCategoriesHaveIconsAndColors(t *testing.T) {
	u := register(t)
	res := call(t, "GET", "/api/v1/categories", u.Token, nil)
	names := map[string]bool{}
	for _, raw := range res.Body["categories"].([]interface{}) {
		c := raw.(map[string]interface{})
		if c["is_system"] == true {
			names[c["name"].(string)] = true
			if c["icon"] == "" || c["color"] == "" {
				t.Errorf("built-in %v has no icon or color", c["name"])
			}
		}
	}
	for _, want := range []string{"Shopping", "Household", "Clothing", "Health", "Other", "Other Income"} {
		if !names[want] {
			t.Errorf("missing built-in category %q", want)
		}
	}
}

func TestCustomCategoryLook(t *testing.T) {
	u := register(t)
	res := call(t, "POST", "/api/v1/categories", u.Token, map[string]string{"name": "Pets", "type": "expense", "icon": "paw", "color": "brown"})
	expect(t, res, http.StatusCreated, "create with icon and color")
	id := res.str("id")
	for name, body := range map[string]map[string]string{
		"bad icon":  {"name": "X", "type": "expense", "icon": "Not An Icon!"},
		"bad color": {"name": "X", "type": "expense", "color": "#ff0000"},
	} {
		expect(t, call(t, "POST", "/api/v1/categories", u.Token, body), http.StatusBadRequest, name)
	}
	expect(t, call(t, "PATCH", "/api/v1/categories/"+id, u.Token, map[string]string{"type": "income"}),
		http.StatusBadRequest, "type change")
	expect(t, call(t, "PATCH", "/api/v1/categories/"+id, u.Token, map[string]string{"name": "Pet care", "color": "pink"}),
		http.StatusOK, "rename and recolor")

	account := createAccount(t, u, "A", "THB", "0")
	tx := createTx(t, u, map[string]interface{}{"account_id": account, "category_id": id, "type": "expense", "amount": "5"})
	if tx.str("category", "color") != "pink" || tx.str("category", "icon") != "paw" {
		t.Errorf("transaction category look = %v", tx.Body["category"])
	}
}

func TestImportIsAllOrNothing(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "Cash", "THB", "1000")
	food := categoryID(t, u, "Food", "expense")
	salary := categoryID(t, u, "Salary", "income")

	bad := map[string]interface{}{
		"account_id": account,
		"rows": []map[string]interface{}{
			{"type": "expense", "amount": "57", "note": "ข้าวเที่ยง", "category_id": food, "occurred_at": "2026-07-30T12:00:00+07:00"},
			{"type": "expense", "amount": "-5", "occurred_at": "2026-07-30T12:01:00+07:00"},
			{"type": "expense", "amount": "5", "category_id": salary, "occurred_at": "2026-07-30T12:02:00+07:00"},
		},
	}
	res := call(t, "POST", "/api/v1/transactions/import", u.Token, bad)
	expect(t, res, http.StatusBadRequest, "bad rows")
	if res.str("error_count") != "1" {
		t.Errorf("validation pass should flag the negative amount only, got %s", res.Raw)
	}
	bad["rows"] = bad["rows"].([]map[string]interface{})[:1]
	bad["rows"] = append(bad["rows"].([]map[string]interface{}),
		map[string]interface{}{"type": "expense", "amount": "5", "category_id": salary, "occurred_at": "2026-07-30T12:02:00+07:00"})
	expect(t, call(t, "POST", "/api/v1/transactions/import", u.Token, bad), http.StatusBadRequest, "category type mismatch")
	if got := call(t, "GET", "/api/v1/transactions", u.Token, nil).str("pagination", "total_items"); got != "0" {
		t.Fatalf("a rejected import saved %s rows", got)
	}

	good := map[string]interface{}{
		"account_id": account,
		"tag":        "import:spending.csv",
		"rows": []map[string]interface{}{
			{"type": "expense", "amount": "57", "note": "ข้าวเที่ยง", "category_id": food, "occurred_at": "2026-07-30T12:00:00+07:00"},
			{"type": "expense", "amount": 17, "note": "mrt", "occurred_at": "2026-07-30T12:01:00+07:00"},
			{"type": "income", "amount": "30000", "category_id": salary, "occurred_at": "2026-07-31T09:00:00+07:00"},
		},
	}
	res = call(t, "POST", "/api/v1/transactions/import", u.Token, good)
	expect(t, res, http.StatusCreated, "good import")
	if res.str("imported") != "3" {
		t.Errorf("imported = %s", res.str("imported"))
	}
	if got := balance(t, u, account); got != "30926.00" {
		t.Errorf("balance = %s, want 30926.00", got)
	}
	tagged := call(t, "GET", "/api/v1/transactions?tags=import:spending.csv", u.Token, nil)
	if tagged.str("pagination", "total_items") != "3" {
		t.Errorf("tagged rows = %s", tagged.str("pagination", "total_items"))
	}

	other := register(t)
	expect(t, call(t, "POST", "/api/v1/transactions/import", other.Token, good), http.StatusBadRequest, "someone else's account")
}

func TestExportCSV(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "Cash", "THB", "0")
	food := categoryID(t, u, "Food", "expense")
	createTx(t, u, map[string]interface{}{"account_id": account, "category_id": food, "type": "expense", "amount": "57",
		"note": "ข้าว, เที่ยง", "occurred_at": "2026-07-30T05:30:00Z"})

	res := call(t, "GET", "/api/v1/transactions/export?tz=Asia/Bangkok", u.Token, nil)
	expect(t, res, http.StatusOK, "export")
	if !strings.HasPrefix(res.Raw, "\xef\xbb\xbf") {
		t.Error("missing UTF-8 byte order mark")
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(res.Raw, "\xef\xbb\xbf"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %v", records)
	}
	row := strings.Join(records[1], "|")
	if want := "2026-07-30|12:30|expense|57.00|THB|Cash||Food||ข้าว, เที่ยง|"; row != want {
		t.Errorf("row = %q, want %q", row, want)
	}
	expect(t, call(t, "GET", "/api/v1/transactions/export?tz=Local", u.Token, nil), http.StatusBadRequest, "bad tz")
}
