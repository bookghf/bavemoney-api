package budget

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func TestCurrentWindow(t *testing.T) {
	tests := []struct {
		start, period, today, from, to string
	}{
		{"2026-08-01", "monthly", "2026-10-01", "2026-10-01", "2026-11-01"},
		{"2026-08-15", "monthly", "2026-10-14", "2026-09-15", "2026-10-15"},
		{"2026-08-15", "monthly", "2026-10-15", "2026-10-15", "2026-11-15"},
		{"2026-01-31", "monthly", "2026-03-15", "2026-02-28", "2026-03-31"},
		{"2024-02-29", "yearly", "2025-03-01", "2025-02-28", "2026-02-28"},
		{"2026-09-28", "weekly", "2026-10-06", "2026-10-05", "2026-10-12"},
		{"2025-03-01", "yearly", "2026-10-01", "2026-03-01", "2027-03-01"},
		// Before the budget starts, its first period is current.
		{"2026-12-01", "monthly", "2026-10-01", "2026-12-01", "2027-01-01"},
	}
	for _, tt := range tests {
		from, to := currentWindow(day(tt.start), tt.period, day(tt.today))
		if from.Format(time.DateOnly) != tt.from || to.Format(time.DateOnly) != tt.to {
			t.Errorf("%s %s on %s: got [%s, %s), want [%s, %s)", tt.period, tt.start, tt.today,
				from.Format(time.DateOnly), to.Format(time.DateOnly), tt.from, tt.to)
		}
	}
}

func TestValidateCreate(t *testing.T) {
	ok := CreateRequest{Amount: "8000", Currency: "thb", Period: "monthly", StartDate: "2026-10-01"}
	if err := validateCreate(&ok); err != nil || ok.AlertThresholdPct != 80 || ok.Currency != "THB" {
		t.Errorf("valid budget: err=%v req=%+v", err, ok)
	}
	for name, req := range map[string]CreateRequest{
		"zero amount":  {Amount: "0", Currency: "THB", Period: "monthly", StartDate: "2026-10-01"},
		"bad period":   {Amount: "1", Currency: "THB", Period: "daily", StartDate: "2026-10-01"},
		"bad date":     {Amount: "1", Currency: "THB", Period: "monthly", StartDate: "2026-02-30"},
		"bad category": {CategoryID: "nope", Amount: "1", Currency: "THB", Period: "monthly", StartDate: "2026-10-01"},
		"threshold":    {Amount: "1", Currency: "THB", Period: "monthly", StartDate: "2026-10-01", AlertThresholdPct: 150},
	} {
		if err := validateCreate(&req); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
