package recurring

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestMonthlyClampsToMonthEnd(t *testing.T) {
	s := schedule{frequency: FrequencyMonthly, dayOfMonth: 31}
	got := []string{}
	next := s.onOrAfter(day("2027-01-01"))
	for range 6 {
		got = append(got, next.Format(time.DateOnly))
		next = s.after(next)
	}
	want := []string{"2027-01-31", "2027-02-28", "2027-03-31", "2027-04-30", "2027-05-31", "2027-06-30"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runs = %v, want %v", got, want)
		}
	}
	if leap := s.onOrAfter(day("2028-02-01")); leap.Format(time.DateOnly) != "2028-02-29" {
		t.Errorf("leap February = %v", leap)
	}
}

func TestOnOrAfter(t *testing.T) {
	tests := []struct {
		s          schedule
		from, want string
	}{
		{schedule{frequency: FrequencyMonthly, dayOfMonth: 25}, "2026-10-05", "2026-10-25"},
		{schedule{frequency: FrequencyMonthly, dayOfMonth: 25}, "2026-10-25", "2026-10-25"},
		{schedule{frequency: FrequencyMonthly, dayOfMonth: 1}, "2026-12-02", "2027-01-01"},
		// 2026-10-05 is a Monday.
		{schedule{frequency: FrequencyWeekly, weekday: 1}, "2026-10-05", "2026-10-05"},
		{schedule{frequency: FrequencyWeekly, weekday: 0}, "2026-10-05", "2026-10-11"},
		{schedule{frequency: FrequencyWeekly, weekday: 5}, "2026-10-05", "2026-10-09"},
	}
	for _, tt := range tests {
		if got := tt.s.onOrAfter(day(tt.from)).Format(time.DateOnly); got != tt.want {
			t.Errorf("%+v from %s = %s, want %s", tt.s, tt.from, got, tt.want)
		}
	}
}

func TestNextPeriod(t *testing.T) {
	monthly := schedule{frequency: FrequencyMonthly, dayOfMonth: 25}
	if got := monthly.nextPeriod(day("2026-12-25")).Format(time.DateOnly); got != "2027-01-01" {
		t.Errorf("monthly next period = %s", got)
	}
	weekly := schedule{frequency: FrequencyWeekly, weekday: 3}
	for from, want := range map[string]string{"2026-10-05": "2026-10-12", "2026-10-11": "2026-10-12", "2026-10-07": "2026-10-12"} {
		if got := weekly.nextPeriod(day(from)).Format(time.DateOnly); got != want {
			t.Errorf("weekly next period from %s = %s, want %s", from, got, want)
		}
	}
}

func TestValidateRule(t *testing.T) {
	const (
		accountA = "11111111-1111-4111-8111-111111111111"
		accountB = "22222222-2222-4222-8222-222222222222"
		category = "33333333-3333-4333-8333-333333333333"
	)
	intp := func(n int) *int { return &n }
	base := CreateRequest{Type: "expense", AccountID: accountA, Amount: "7300", Frequency: FrequencyMonthly,
		DayOfMonth: intp(1), StartDate: "2026-10-01", TimeZone: "Asia/Bangkok"}

	tests := []struct {
		name    string
		edit    func(*CreateRequest)
		wantErr bool
	}{
		{"monthly expense", func(*CreateRequest) {}, false},
		{"weekly", func(r *CreateRequest) { r.Frequency = FrequencyWeekly; r.Weekday = intp(0) }, false},
		{"transfer", func(r *CreateRequest) { r.Type = "transfer"; r.ToAccountID = accountB }, false},
		{"no time zone defaults to UTC", func(r *CreateRequest) { r.TimeZone = "" }, false},
		{"bad time zone", func(r *CreateRequest) { r.TimeZone = "Mars/Base" }, true},
		{"day 0", func(r *CreateRequest) { r.DayOfMonth = intp(0) }, true},
		{"day 32", func(r *CreateRequest) { r.DayOfMonth = intp(32) }, true},
		{"monthly without day", func(r *CreateRequest) { r.DayOfMonth = nil }, true},
		{"weekly without weekday", func(r *CreateRequest) { r.Frequency = FrequencyWeekly }, true},
		{"weekday 7", func(r *CreateRequest) { r.Frequency = FrequencyWeekly; r.Weekday = intp(7) }, true},
		{"daily", func(r *CreateRequest) { r.Frequency = "daily" }, true},
		{"bad start", func(r *CreateRequest) { r.StartDate = "2026-02-30" }, true},
		{"end before start", func(r *CreateRequest) { r.EndDate = "2026-09-30" }, true},
		// The transaction rules apply unchanged.
		{"zero amount", func(r *CreateRequest) { r.Amount = "0" }, true},
		{"three decimals", func(r *CreateRequest) { r.Amount = "1.005" }, true},
		{"bad currency", func(r *CreateRequest) { r.Currency = "TH" }, true},
		{"transfer to same account", func(r *CreateRequest) { r.Type = "transfer"; r.ToAccountID = accountA }, true},
		{"transfer with category", func(r *CreateRequest) { r.Type = "transfer"; r.ToAccountID = accountB; r.CategoryID = category }, true},
		{"expense with to_account_id", func(r *CreateRequest) { r.ToAccountID = accountB }, true},
	}
	for _, tt := range tests {
		req := base
		tt.edit(&req)
		if _, _, err := validateRule(&req); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}

	weekly := base
	weekly.Frequency, weekly.Weekday = FrequencyWeekly, intp(3)
	if _, _, err := validateRule(&weekly); err != nil || weekly.DayOfMonth != nil {
		t.Errorf("weekly keeps day_of_month: %v %v", err, weekly.DayOfMonth)
	}
}
