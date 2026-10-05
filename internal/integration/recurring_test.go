//go:build integration

package integration

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"ledger-api/internal/recurring"
)

// runRecurring runs the recurring rule runner for one user as of now.
func runRecurring(t *testing.T, u user, now time.Time) int {
	t.Helper()
	created, err := recurring.NewRepository(db).RunDue(context.Background(), now, u.ID)
	if err != nil {
		t.Fatalf("run recurring: %v", err)
	}
	return created
}

// ruleTransactions returns the occurred_at days (Bangkok) of a rule's transactions.
func ruleTransactions(t *testing.T, u user, ruleID string) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT to_char(occurred_at AT TIME ZONE 'Asia/Bangkok', 'YYYY-MM-DD') FROM transactions
		WHERE user_id = $1 AND deleted_at IS NULL AND $2 = ANY(tags) ORDER BY occurred_at
	`, u.ID, recurring.TagPrefix+ruleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	days := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		days = append(days, d)
	}
	return days
}

func bangkok(t *testing.T, value string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Bangkok")
	at, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRecurringCRUDAndValidation(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "50000")
	daily := createAccount(t, u, "Daily Life", "THB", "0")
	usd := createAccount(t, u, "USD", "USD", "0")
	salary := systemCategory(t, "income")
	food := systemCategory(t, "expense")

	// Rules start in the future so saving them creates nothing yet.
	res := call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "income", "account_id": bank, "category_id": salary, "amount": "47000", "note": "เงินเดือน",
		"frequency": "monthly", "day_of_month": 25, "start_date": "2030-01-01", "time_zone": "Asia/Bangkok",
	})
	expect(t, res, http.StatusCreated, "create salary")
	id := res.str("rule", "id")
	if res.str("rule", "next_run_on") != "2030-01-25" || res.str("rule", "currency") != "THB" ||
		res.str("rule", "amount") != "47000.00" || res.str("rule", "is_active") != "true" ||
		res.str("rule", "category", "id") != salary {
		t.Fatalf("created rule = %s", res.Raw)
	}

	res = call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "transfer", "account_id": bank, "to_account_id": daily, "amount": 25000,
		"frequency": "weekly", "weekday": 1, "start_date": "2030-01-01", "time_zone": "Asia/Bangkok",
	})
	expect(t, res, http.StatusCreated, "create transfer")
	// 2030-01-01 is a Tuesday; the first Monday after is the 7th.
	if res.str("rule", "next_run_on") != "2030-01-07" || res.str("rule", "to_account_name") != "Daily Life" {
		t.Fatalf("transfer rule = %s", res.Raw)
	}

	list := call(t, "GET", "/api/v1/recurring", u.Token, nil)
	expect(t, list, http.StatusOK, "list")
	if n := len(list.Body["rules"].([]interface{})); n != 2 {
		t.Fatalf("listed %d rules, want 2", n)
	}

	for name, body := range map[string]map[string]interface{}{
		"zero amount":         {"type": "expense", "account_id": bank, "amount": "0", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
		"category type":       {"type": "expense", "account_id": bank, "category_id": salary, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
		"currency mismatch":   {"type": "expense", "account_id": bank, "currency": "USD", "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
		"transfer currencies": {"type": "transfer", "account_id": bank, "to_account_id": usd, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
		"day 32":              {"type": "expense", "account_id": bank, "amount": "1", "frequency": "monthly", "day_of_month": 32, "start_date": "2030-01-01"},
		"no weekday":          {"type": "expense", "account_id": bank, "amount": "1", "frequency": "weekly", "start_date": "2030-01-01"},
		"bad time zone":       {"type": "expense", "account_id": bank, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01", "time_zone": "Mars/Base"},
		"too far back":        {"type": "expense", "account_id": bank, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2020-01-01"},
	} {
		expect(t, call(t, "POST", "/api/v1/recurring", u.Token, body), http.StatusBadRequest, name)
	}

	// PATCH merges onto the stored rule and revalidates the whole rule.
	res = call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"amount": "48000", "day_of_month": 31, "category_id": food})
	expect(t, res, http.StatusBadRequest, "expense category on income rule")
	res = call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"amount": "48000", "day_of_month": 31})
	expect(t, res, http.StatusOK, "update")
	if res.str("rule", "amount") != "48000.00" || res.str("rule", "next_run_on") != "2030-01-31" || res.str("rule", "note") != "เงินเดือน" {
		t.Fatalf("updated rule = %s", res.Raw)
	}
	res = call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"is_active": false})
	expect(t, res, http.StatusOK, "pause")
	if res.str("rule", "is_active") != "false" {
		t.Fatalf("paused rule = %s", res.Raw)
	}
	expect(t, call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{}), http.StatusBadRequest, "empty patch")

	expect(t, call(t, "DELETE", "/api/v1/recurring/"+id, u.Token, nil), http.StatusOK, "delete")
	expect(t, call(t, "GET", "/api/v1/recurring/"+id, u.Token, nil), http.StatusNotFound, "get deleted")
}

func TestRecurringOwnership(t *testing.T) {
	owner, other := register(t), register(t)
	ownerAccount := createAccount(t, owner, "Owner", "THB", "0")
	otherAccount := createAccount(t, other, "Other", "THB", "0")
	otherCategory := call(t, "POST", "/api/v1/categories", other.Token, map[string]string{"name": "Mine", "type": "expense"}).str("id")

	res := call(t, "POST", "/api/v1/recurring", owner.Token, map[string]interface{}{
		"type": "expense", "account_id": ownerAccount, "amount": "7300", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01",
	})
	expect(t, res, http.StatusCreated, "create")
	id := res.str("rule", "id")

	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		expect(t, call(t, method, "/api/v1/recurring/"+id, other.Token, map[string]interface{}{"amount": "1"}), http.StatusNotFound, "other user "+method)
	}
	if n := len(call(t, "GET", "/api/v1/recurring", other.Token, nil).Body["rules"].([]interface{})); n != 0 {
		t.Errorf("other user lists %d rules", n)
	}
	expect(t, call(t, "GET", "/api/v1/recurring", "", nil), http.StatusUnauthorized, "no token")

	// Someone else's account, to-account, or category is rejected.
	for name, body := range map[string]map[string]interface{}{
		"account":    {"type": "expense", "account_id": otherAccount, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
		"to account": {"type": "transfer", "account_id": ownerAccount, "to_account_id": otherAccount, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
		"category":   {"type": "expense", "account_id": ownerAccount, "category_id": otherCategory, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01"},
	} {
		expect(t, call(t, "POST", "/api/v1/recurring", owner.Token, body), http.StatusBadRequest, "other user's "+name)
	}
	expect(t, call(t, "PATCH", "/api/v1/recurring/"+id, owner.Token, map[string]interface{}{"account_id": otherAccount}),
		http.StatusBadRequest, "patch to other user's account")
}

func TestRecurringRunnerCatchesUpAndClampsMonthEnd(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "100000")
	rent := createAccount(t, u, "Rent", "THB", "0")

	res := call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "transfer", "account_id": bank, "to_account_id": rent, "amount": "7300", "note": "ค่าเช่า",
		"frequency": "monthly", "day_of_month": 31, "start_date": "2030-01-15", "time_zone": "Asia/Bangkok",
	})
	expect(t, res, http.StatusCreated, "create")
	id := res.str("rule", "id")

	// Nothing is due before the first run day, local time: 23:59 on the 30th
	// in Bangkok is still the 30th.
	if n := runRecurring(t, u, bangkok(t, "2030-01-30 23:59")); n != 0 {
		t.Fatalf("created %d before the due day", n)
	}
	// Missed months are caught up in one run, the 31st clamped to month end.
	if n := runRecurring(t, u, bangkok(t, "2030-05-01 08:00")); n != 4 {
		t.Fatalf("created %d, want 4", n)
	}
	want := []string{"2030-01-31", "2030-02-28", "2030-03-31", "2030-04-30"}
	if got := ruleTransactions(t, u, id); !equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	rule := call(t, "GET", "/api/v1/recurring/"+id, u.Token, nil)
	if rule.str("rule", "next_run_on") != "2030-05-31" || rule.str("rule", "last_run_on") != "2030-04-30" {
		t.Fatalf("rule after run = %s", rule.Raw)
	}
	if got := balance(t, u, rent); got != "29200.00" {
		t.Errorf("rent balance = %s, want 29200.00", got)
	}

	// Running again for the same moment creates nothing more.
	if n := runRecurring(t, u, bangkok(t, "2030-05-01 08:00")); n != 0 {
		t.Fatalf("second run created %d", n)
	}

	// The created transactions carry the trace tag and the rule's fields.
	tx := call(t, "GET", "/api/v1/transactions?tags="+recurring.TagPrefix+id+"&limit=1&sort=occurred_at", u.Token, nil)
	expect(t, tx, http.StatusOK, "list by tag")
	first := tx.Body["transactions"].([]interface{})[0].(map[string]interface{})
	if first["type"] != "transfer" || first["amount"] != "7300.00" || first["note"] != "ค่าเช่า" ||
		first["occurred_at"] != "2030-01-30T17:00:00Z" {
		t.Errorf("first transaction = %v", first)
	}

	// An end date stops the rule; next_run_on is then null.
	expect(t, call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"end_date": "2030-05-31"}), http.StatusOK, "set end")
	runRecurring(t, u, bangkok(t, "2030-09-01 08:00"))
	if got := ruleTransactions(t, u, id); len(got) != 5 {
		t.Fatalf("runs with end date = %v", got)
	}
	rule = call(t, "GET", "/api/v1/recurring/"+id, u.Token, nil)
	if rule.Body["rule"].(map[string]interface{})["next_run_on"] != nil {
		t.Errorf("ended rule next_run_on = %v", rule.str("rule", "next_run_on"))
	}
}

func TestRecurringRunnerIsIdempotentAcrossInstances(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "0")
	salary := systemCategory(t, "income")
	res := call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "income", "account_id": bank, "category_id": salary, "amount": "47000",
		"frequency": "weekly", "weekday": 5, "start_date": "2030-01-01", "time_zone": "Asia/Bangkok",
	})
	expect(t, res, http.StatusCreated, "create")
	id := res.str("rule", "id")

	// Four "instances" run at once; every Friday is created exactly once.
	now := bangkok(t, "2030-03-01 12:00")
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := recurring.NewRepository(db).RunDue(context.Background(), now, u.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	runRecurring(t, u, now)

	got := ruleTransactions(t, u, id)
	want := []string{"2030-01-04", "2030-01-11", "2030-01-18", "2030-01-25", "2030-02-01", "2030-02-08", "2030-02-15", "2030-02-22", "2030-03-01"}
	if !equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if got := balance(t, u, bank); got != "423000.00" {
		t.Errorf("balance = %s, want 423000.00", got)
	}
}

func TestRecurringRunnerPausesOnArchivedAccount(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "0")
	travel := createAccount(t, u, "Travel", "THB", "0")
	res := call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "transfer", "account_id": bank, "to_account_id": travel, "amount": "3000",
		"frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01", "time_zone": "Asia/Bangkok",
	})
	expect(t, res, http.StatusCreated, "create")
	id := res.str("rule", "id")

	if n := runRecurring(t, u, bangkok(t, "2030-02-15 08:00")); n != 2 {
		t.Fatalf("created %d, want 2", n)
	}
	expect(t, call(t, "DELETE", "/api/v1/accounts/"+travel, u.Token, nil), http.StatusOK, "archive travel")
	if n := runRecurring(t, u, bangkok(t, "2030-04-15 08:00")); n != 0 {
		t.Fatalf("created %d into an archived account", n)
	}
	rule := call(t, "GET", "/api/v1/recurring/"+id, u.Token, nil)
	if rule.str("rule", "is_active") != "false" || rule.str("rule", "pause_reason") != recurring.ReasonAccountArchived ||
		rule.str("rule", "next_run_on") != "2030-03-01" {
		t.Fatalf("rule after archive = %s", rule.Raw)
	}
	// It can not resume onto the archived account.
	expect(t, call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"is_active": true}),
		http.StatusBadRequest, "resume onto archived account")
}

func TestRecurringResumeSkipsPausedPeriods(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "0")
	// Started in the past: saving it back-fills the runs already due.
	start := time.Now().AddDate(0, -2, 0).Format(time.DateOnly)
	res := call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "expense", "account_id": bank, "amount": "100", "frequency": "weekly", "weekday": int(time.Now().Weekday()),
		"start_date": start, "time_zone": "UTC",
	})
	expect(t, res, http.StatusCreated, "create")
	id := res.str("rule", "id")
	backfilled := len(ruleTransactions(t, u, id))
	if backfilled < 8 {
		t.Fatalf("back-filled %d runs, want at least 8", backfilled)
	}

	// Pause, let periods pass, resume: nothing is back-filled for the pause,
	// and the next run is in the future.
	expect(t, call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"is_active": false}), http.StatusOK, "pause")
	if _, err := db.Exec(`UPDATE recurring_rules SET next_run_on = next_run_on - 21 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	res = call(t, "PATCH", "/api/v1/recurring/"+id, u.Token, map[string]interface{}{"is_active": true})
	expect(t, res, http.StatusOK, "resume")
	if got := len(ruleTransactions(t, u, id)); got != backfilled {
		t.Fatalf("resume created %d more runs", got-backfilled)
	}
	if next := res.str("rule", "next_run_on"); next <= time.Now().UTC().Format(time.DateOnly) {
		t.Errorf("next_run_on after resume = %s", next)
	}
}

func TestRecurringRulesDoNotBlockReset(t *testing.T) {
	u := register(t)
	bank := createAccount(t, u, "Bank", "THB", "0")
	expect(t, call(t, "POST", "/api/v1/recurring", u.Token, map[string]interface{}{
		"type": "expense", "account_id": bank, "amount": "1", "frequency": "monthly", "day_of_month": 1, "start_date": "2030-01-01",
	}), http.StatusCreated, "create")
	expect(t, call(t, "POST", "/api/v1/me/reset", u.Token, map[string]string{"password": "correct horse"}), http.StatusOK, "reset")
	if n := len(call(t, "GET", "/api/v1/recurring", u.Token, nil).Body["rules"].([]interface{})); n != 0 {
		t.Errorf("%d rules left after reset", n)
	}
}
