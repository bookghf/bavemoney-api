//go:build integration

package integration

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCurrencyRules(t *testing.T) {
	u := register(t)
	thb := createAccount(t, u, "THB", "THB", "0")
	usd := createAccount(t, u, "USD", "USD", "0")

	expect(t, createTx(t, u, map[string]interface{}{"account_id": thb, "type": "expense", "amount": "100", "currency": "USD"}),
		http.StatusBadRequest, "USD expense on a THB account")
	res := createTx(t, u, map[string]interface{}{"account_id": thb, "type": "expense", "amount": "20.50"})
	expect(t, res, http.StatusCreated, "expense without currency")
	if res.str("currency") != "THB" || res.str("amount_in_default_currency") != "20.50" {
		t.Errorf("expense took currency %q, converted %q", res.str("currency"), res.str("amount_in_default_currency"))
	}
	expect(t, createTx(t, u, map[string]interface{}{"account_id": usd, "type": "expense", "amount": "100"}), http.StatusCreated, "USD expense")

	// Reports never add currencies together.
	for currency, want := range map[string]string{"THB": "20.50", "USD": "100.00", "": "20.50"} {
		path := "/api/v1/reports/summary?period=month&date=2026-09&tz=Asia/Bangkok"
		if currency != "" {
			path += "&currency=" + currency
		}
		res := call(t, "GET", path, u.Token, nil)
		expect(t, res, http.StatusOK, "report "+currency)
		if got := res.str("total_expense"); got != want {
			t.Errorf("report currency %q: total_expense = %s, want %s", currency, got, want)
		}
	}

	// Account currency is locked once used, free while empty.
	expect(t, call(t, "PATCH", "/api/v1/accounts/"+thb, u.Token, map[string]string{"currency": "USD"}),
		http.StatusBadRequest, "change currency of a used account")
	empty := createAccount(t, u, "Empty", "THB", "0")
	expect(t, call(t, "PATCH", "/api/v1/accounts/"+empty, u.Token, map[string]string{"currency": "EUR"}),
		http.StatusOK, "change currency of an empty account")
}

func TestForeignAmountIsConvertedWhenARateExists(t *testing.T) {
	u := register(t)
	eur := createAccount(t, u, "EUR", "EUR", "0")
	if _, err := db.Exec(`INSERT INTO exchange_rates (base_currency, target_currency, rate, effective_date)
		VALUES ('EUR', 'THB', 38.5, '1999-01-01') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	res := createTx(t, u, map[string]interface{}{"account_id": eur, "type": "expense", "amount": "10", "occurred_at": "1999-06-01T10:00:00Z"})
	expect(t, res, http.StatusCreated, "EUR expense")
	if got := res.str("amount_in_default_currency"); got != "385.00" {
		t.Errorf("amount_in_default_currency = %q, want 385.00", got)
	}
}

func TestArchivedAccountsRejectNewTransactions(t *testing.T) {
	u := register(t)
	open := createAccount(t, u, "Open", "THB", "100")
	archived := createAccount(t, u, "Archived", "THB", "0")
	expect(t, call(t, "DELETE", "/api/v1/accounts/"+archived, u.Token, nil), http.StatusOK, "archive")

	expect(t, createTx(t, u, map[string]interface{}{"account_id": archived, "type": "expense", "amount": "1"}),
		http.StatusBadRequest, "expense on an archived account")
	expect(t, createTx(t, u, map[string]interface{}{"account_id": open, "to_account_id": archived, "type": "transfer", "amount": "1"}),
		http.StatusBadRequest, "transfer into an archived account")

	res := call(t, "GET", "/api/v1/accounts", u.Token, nil)
	if n := len(res.Body["accounts"].([]interface{})); n != 1 {
		t.Errorf("default list has %d accounts, want 1 (archived hidden)", n)
	}
	res = call(t, "GET", "/api/v1/accounts?include_archived=true", u.Token, nil)
	if n := len(res.Body["accounts"].([]interface{})); n != 2 {
		t.Errorf("include_archived list has %d accounts, want 2", n)
	}
}

func TestBalancesAreExactDecimals(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "Big", "THB", "0")
	expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "9999999999999999.99"}),
		http.StatusCreated, "huge expense")
	if got := balance(t, u, account); got != "-9999999999999999.99" {
		t.Errorf("balance = %s, want -9999999999999999.99", got)
	}

	// Many huge incomes in one month must not overflow the report.
	for i := 0; i < 10; i++ {
		createTx(t, u, map[string]interface{}{"account_id": account, "type": "income", "amount": "9999999999999999.99", "occurred_at": "2025-01-15T10:00:00Z"})
	}
	res := call(t, "GET", "/api/v1/reports/summary?period=month&date=2025-01", u.Token, nil)
	expect(t, res, http.StatusOK, "report with huge totals")
	if got := res.str("total_income"); got != "99999999999999999.90" {
		t.Errorf("total_income = %s", got)
	}

	// Accounts take decimal strings (and numbers) for initial_balance.
	res = call(t, "POST", "/api/v1/accounts", u.Token, map[string]interface{}{"name": "Num", "type": "cash", "currency": "THB", "initial_balance": 1000.5})
	expect(t, res, http.StatusCreated, "numeric initial_balance")
	if got := res.str("account", "initial_balance"); got != "1000.50" {
		t.Errorf("initial_balance = %s, want 1000.50", got)
	}
}

func TestBadInputIsAClientErrorNotA500(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	other := register(t)
	otherAccount := createAccount(t, other, "B", "THB", "0")

	bad := []struct {
		method, path string
		body         interface{}
		want         int
	}{
		{"POST", "/api/v1/transactions", map[string]interface{}{"account_id": account, "type": "expense", "amount": "1", "occurred_at": "not-a-date"}, 400},
		{"POST", "/api/v1/transactions", map[string]interface{}{"account_id": account, "type": "expense", "amount": "1", "occurred_at": "2026-02-30T10:00:00Z"}, 400},
		{"POST", "/api/v1/transactions", map[string]interface{}{"account_id": account, "type": "expense", "amount": "1", "currency": "XYZ", "occurred_at": "2026-09-01T10:00:00Z"}, 400},
		{"POST", "/api/v1/transactions", map[string]interface{}{"account_id": account, "type": "expense", "amount": "1", "category_id": "nope", "occurred_at": "2026-09-01T10:00:00Z"}, 400},
		{"POST", "/api/v1/transactions", map[string]interface{}{"account_id": account, "type": "expense", "amount": "99999999999999999999", "occurred_at": "2026-09-01T10:00:00Z"}, 400},
		{"GET", "/api/v1/transactions/not-a-uuid", nil, 404},
		{"DELETE", "/api/v1/transactions/not-a-uuid", nil, 404},
		{"GET", "/api/v1/accounts/not-a-uuid", nil, 404},
		{"POST", "/api/v1/accounts", map[string]interface{}{"name": "x", "type": "cash", "currency": "XXX"}, 400},
		{"POST", "/api/v1/accounts", map[string]interface{}{"name": "x", "type": "cash", "currency": "thb"}, 201},
		{"POST", "/api/v1/accounts", map[string]interface{}{"name": "x", "type": "cash", "currency": "THB", "initial_balance": 1e20}, 400},
		{"POST", "/api/v1/accounts", map[string]interface{}{"name": "x", "type": "spaceship", "currency": "THB"}, 400},
		{"POST", "/api/v1/accounts", map[string]interface{}{"name": "   ", "type": "cash", "currency": "THB"}, 400},
		{"POST", "/api/v1/accounts", map[string]interface{}{"name": "x", "type": "cash", "currency": "THB", "initial_balance": "-99999"}, 400},
		{"PATCH", "/api/v1/accounts/" + otherAccount, map[string]interface{}{"name": "mine now"}, 404},
		{"DELETE", "/api/v1/accounts/" + otherAccount, nil, 404},
		{"GET", "/api/v1/transactions?page=9223372036854775807", nil, 400},
		{"GET", "/api/v1/transactions?account=nope", nil, 400},
		{"GET", "/api/v1/reports/summary?period=month&date=2026-09&account_id=nope", nil, 400},
		{"DELETE", "/api/v1/transactions/00000000-0000-4000-8000-000000000000", nil, 404},
		{"POST", "/api/v1/categories", map[string]interface{}{"name": "x", "type": "expense", "parent_id": "nope"}, 400},
		{"POST", "/api/v1/transactions", `{"account_id": "` + account + `", "type": "expense", "amount": "1", "note": "` + strings.Repeat("x", 2<<20) + `"}`, 413},
	}
	for _, tt := range bad {
		res := call(t, tt.method, tt.path, u.Token, tt.body)
		if res.Status != tt.want {
			t.Errorf("%s %s: status %d, want %d: %s", tt.method, tt.path, res.Status, tt.want, res.Raw)
		}
	}
}

func TestCrossUserDeletesAreNotFound(t *testing.T) {
	a, b := register(t), register(t)
	accountB := createAccount(t, b, "B", "THB", "0")
	txB := createTx(t, b, map[string]interface{}{"account_id": accountB, "type": "expense", "amount": "5"}).str("id")

	expect(t, call(t, "DELETE", "/api/v1/transactions/"+txB, a.Token, nil), http.StatusNotFound, "A deletes B's transaction")
	expect(t, call(t, "GET", "/api/v1/transactions/"+txB, b.Token, nil), http.StatusOK, "B's transaction survives")
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+txB, b.Token, nil), http.StatusOK, "B deletes it")
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+txB, b.Token, nil), http.StatusNotFound, "deleting twice")
}

func TestTagsCanBeCleared(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	tx := createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "5", "tags": []string{"x"}}).str("id")
	res := call(t, "PATCH", "/api/v1/transactions/"+tx, u.Token, map[string]interface{}{"tags": []string{}})
	expect(t, res, http.StatusOK, "clear tags")
	if tags := res.Body["tags"].([]interface{}); len(tags) != 0 {
		t.Errorf("tags = %v, want []", tags)
	}
}

func TestTransactionListUsesTimeZoneDays(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "5", "occurred_at": "2027-01-01T00:30:00+07:00"})

	res := call(t, "GET", "/api/v1/transactions?from=2027-01-01&to=2027-01-31&tz=Asia/Bangkok", u.Token, nil)
	expect(t, res, http.StatusOK, "list January in Bangkok")
	if n := len(res.Body["transactions"].([]interface{})); n != 1 {
		t.Errorf("January (Bangkok) has %d transactions, want 1", n)
	}
	report := call(t, "GET", "/api/v1/reports/summary?period=month&date=2027-01&tz=Asia/Bangkok", u.Token, nil)
	if got := report.str("total_expense"); got != "5.00" {
		t.Errorf("report January = %s, want 5.00", got)
	}
}

func TestParallelTransfersKeepBalancesExact(t *testing.T) {
	u := register(t)
	from := createAccount(t, u, "From", "THB", "1000")
	to := createAccount(t, u, "To", "THB", "0")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			createTx(t, u, map[string]interface{}{"account_id": from, "to_account_id": to, "type": "transfer", "amount": "10.05"})
		}()
	}
	wg.Wait()
	if got := balance(t, u, from); got != "799.00" {
		t.Errorf("from balance = %s, want 799.00", got)
	}
	if got := balance(t, u, to); got != "201.00" {
		t.Errorf("to balance = %s, want 201.00", got)
	}
}

func TestBudgetTracksCurrentPeriodSpend(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	food := systemCategory(t, "expense")
	now := time.Now().In(mustLoad("Asia/Bangkok"))
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)

	res := call(t, "POST", "/api/v1/budgets?tz=Asia/Bangkok", u.Token, map[string]interface{}{
		"category_id": food, "amount": "1000", "currency": "THB", "period": "monthly", "start_date": start,
	})
	expect(t, res, http.StatusCreated, "create budget")
	budget := res.str("budget", "id")
	expect(t, call(t, "POST", "/api/v1/budgets", u.Token, map[string]interface{}{
		"category_id": food, "amount": "5", "currency": "THB", "period": "monthly", "start_date": start,
	}), http.StatusConflict, "duplicate budget")

	createTx(t, u, map[string]interface{}{"account_id": account, "category_id": food, "type": "expense", "amount": "850",
		"occurred_at": now.Format(time.RFC3339)})
	createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "999",
		"occurred_at": now.Format(time.RFC3339)}) // other category: not counted

	res = call(t, "GET", "/api/v1/budgets/"+budget+"?tz=Asia/Bangkok", u.Token, nil)
	expect(t, res, http.StatusOK, "get budget")
	if res.str("budget", "current_spend") != "850.00" || res.str("budget", "remaining") != "150.00" || res.str("budget", "percent_used") != "85" {
		t.Errorf("budget progress = %v", res.Body["budget"])
	}
	expect(t, call(t, "DELETE", "/api/v1/budgets/"+budget, u.Token, nil), http.StatusOK, "delete budget")
	expect(t, call(t, "DELETE", "/api/v1/budgets/"+budget, u.Token, nil), http.StatusNotFound, "delete twice")
}

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
