//go:build integration

package integration

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"ledger-api/internal/month"
)

func setMonthStartDay(t *testing.T, u user, day int) {
	t.Helper()
	res := call(t, "PATCH", "/api/v1/me", u.Token, map[string]int{"month_start_day": day})
	expect(t, res, http.StatusOK, "set month_start_day")
	if got := res.str("user", "month_start_day"); got != strconv.Itoa(day) {
		t.Fatalf("month_start_day = %s, want %d", got, day)
	}
}

func TestMonthStartDayProfile(t *testing.T) {
	u := register(t)

	res := call(t, "GET", "/api/v1/me", u.Token, nil)
	expect(t, res, http.StatusOK, "get profile")
	if got := res.str("user", "month_start_day"); got != "1" {
		t.Errorf("default month_start_day = %q, want 1", got)
	}

	setMonthStartDay(t, u, 25)
	// Other edits leave it alone, and it comes back on GET /me and login.
	expect(t, call(t, "PATCH", "/api/v1/me", u.Token, map[string]string{"display_name": "Payday"}), http.StatusOK, "rename")
	if got := call(t, "GET", "/api/v1/me", u.Token, nil).str("user", "month_start_day"); got != "25" {
		t.Errorf("GET /me month_start_day = %q, want 25", got)
	}
	login := call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"})
	expect(t, login, http.StatusOK, "login")
	if got := login.str("user", "month_start_day"); got != "25" {
		t.Errorf("login month_start_day = %q, want 25", got)
	}

	for name, body := range map[string]interface{}{
		"zero":      map[string]int{"month_start_day": 0},
		"too late":  map[string]int{"month_start_day": 29},
		"negative":  map[string]int{"month_start_day": -1},
		"not a num": map[string]string{"month_start_day": "25"},
	} {
		expect(t, call(t, "PATCH", "/api/v1/me", u.Token, body), http.StatusBadRequest, name)
	}
	if got := call(t, "GET", "/api/v1/me", u.Token, nil).str("user", "month_start_day"); got != "25" {
		t.Errorf("a rejected edit changed month_start_day to %q", got)
	}
}

func TestMonthStartDayMigrationIsIdempotent(t *testing.T) {
	body, err := os.ReadFile("../migrate/sql/009_month_start_day.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.ExecContext(context.Background(), string(body)); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	u := register(t)
	if _, err := db.Exec(`UPDATE users SET month_start_day = 29 WHERE id = $1`, u.ID); err == nil {
		t.Error("the database accepted month_start_day 29")
	}
}

func TestReportMonthFollowsMonthStartDay(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	for day, amount := range map[string]string{"2026-09-24": "1", "2026-09-25": "20", "2026-10-24": "300", "2026-10-25": "4000"} {
		expect(t, createTx(t, u, map[string]interface{}{"account_id": account, "type": "income", "amount": amount,
			"occurred_at": day + "T09:00:00+07:00"}), http.StatusCreated, "income on "+day)
	}

	check := func(date, wantStart, wantEnd, wantIncome string) {
		t.Helper()
		res := call(t, "GET", "/api/v1/reports/summary?period=month&tz=Asia/Bangkok&date="+date, u.Token, nil)
		expect(t, res, http.StatusOK, "report "+date)
		if res.str("start_date") != wantStart || res.str("end_date") != wantEnd || res.str("total_income") != wantIncome {
			t.Errorf("month of %s = %s..%s income %s, want %s..%s income %s", date,
				res.str("start_date"), res.str("end_date"), res.str("total_income"), wantStart, wantEnd, wantIncome)
		}
	}

	check("2026-10-05", "2026-10-01", "2026-10-31", "4300.00")

	setMonthStartDay(t, u, 25)
	check("2026-10-05", "2026-09-25", "2026-10-24", "320.00")
	check("2026-10-25", "2026-10-25", "2026-11-24", "4000.00")
	check("2026-09-24", "2026-08-25", "2026-09-24", "1.00")
	// YYYY-MM names the month that starts in it.
	check("2026-09", "2026-09-25", "2026-10-24", "320.00")

	// Other periods keep their calendar meaning.
	res := call(t, "GET", "/api/v1/reports/summary?period=year&date=2026&tz=Asia/Bangkok", u.Token, nil)
	expect(t, res, http.StatusOK, "year report")
	if res.str("start_date") != "2026-01-01" || res.str("total_income") != "4321.00" {
		t.Errorf("year = %s income %s", res.str("start_date"), res.str("total_income"))
	}

	setMonthStartDay(t, u, 1)
	check("2026-10-05", "2026-10-01", "2026-10-31", "4300.00")
}

func TestMonthlyBudgetFollowsMonthStartDay(t *testing.T) {
	u := register(t)
	account := createAccount(t, u, "A", "THB", "0")
	loc := mustLoad("Asia/Bangkok")
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	setMonthStartDay(t, u, 25)
	from, to := month.Range(today, 25)
	res := call(t, "POST", "/api/v1/budgets?tz=Asia/Bangkok", u.Token, map[string]interface{}{
		"amount": "1000", "currency": "THB", "period": "monthly", "start_date": from.Format(time.DateOnly),
	})
	expect(t, res, http.StatusCreated, "create budget")
	budget := res.str("budget", "id")

	at := func(day time.Time) string {
		return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc).Format(time.RFC3339)
	}
	createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "100", "occurred_at": at(from)})
	createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "20", "occurred_at": at(to)})
	// The day before the period starts is last month.
	createTx(t, u, map[string]interface{}{"account_id": account, "type": "expense", "amount": "7", "occurred_at": at(from.AddDate(0, 0, -1))})

	res = call(t, "GET", "/api/v1/budgets/"+budget+"?tz=Asia/Bangkok", u.Token, nil)
	expect(t, res, http.StatusOK, "get budget")
	if res.str("budget", "period_start") != from.Format(time.DateOnly) || res.str("budget", "period_end") != to.Format(time.DateOnly) {
		t.Errorf("period = %s..%s, want %s..%s", res.str("budget", "period_start"), res.str("budget", "period_end"),
			from.Format(time.DateOnly), to.Format(time.DateOnly))
	}
	if got := res.str("budget", "current_spend"); got != "120.00" {
		t.Errorf("current_spend = %s, want 120.00", got)
	}

	// Back to calendar months: the window follows the profile right away.
	setMonthStartDay(t, u, 1)
	calFrom, calTo := month.Range(today, 1)
	list := call(t, "GET", "/api/v1/budgets?tz=Asia/Bangkok", u.Token, nil)
	expect(t, list, http.StatusOK, "list budgets")
	got := list.Body["budgets"].([]interface{})[0].(map[string]interface{})
	if got["period_start"] != calFrom.Format(time.DateOnly) || got["period_end"] != calTo.Format(time.DateOnly) {
		t.Errorf("calendar period = %v..%v, want %s..%s", got["period_start"], got["period_end"],
			calFrom.Format(time.DateOnly), calTo.Format(time.DateOnly))
	}
}
