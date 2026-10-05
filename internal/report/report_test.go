package report

import (
	"database/sql"
	"math/big"
	"testing"
)

func TestPeriodRange(t *testing.T) {
	tests := []struct {
		name               string
		filter             SummaryFilter
		wantStart, wantEnd string
		wantErr            bool
	}{
		{"day", SummaryFilter{Period: "day", Date: "2026-09-30"}, "2026-09-30", "2026-09-30", false},
		{"week starts sunday", SummaryFilter{Period: "week", Date: "2026-10-01"}, "2026-09-27", "2026-10-03", false},
		{"month short", SummaryFilter{Period: "month", Date: "2026-02"}, "2026-02-01", "2026-02-28", false},
		{"month by date", SummaryFilter{Period: "month", Date: "2026-10-05"}, "2026-10-01", "2026-10-31", false},
		{"month start day 1", SummaryFilter{Period: "month", Date: "2026-10-05", MonthStartDay: 1}, "2026-10-01", "2026-10-31", false},
		{"payday month by date", SummaryFilter{Period: "month", Date: "2026-10-05", MonthStartDay: 25}, "2026-09-25", "2026-10-24", false},
		{"payday month on payday", SummaryFilter{Period: "month", Date: "2026-10-25", MonthStartDay: 25}, "2026-10-25", "2026-11-24", false},
		{"payday month across years", SummaryFilter{Period: "month", Date: "2026-01-03", MonthStartDay: 25}, "2025-12-25", "2026-01-24", false},
		{"payday month by YYYY-MM", SummaryFilter{Period: "month", Date: "2026-10", MonthStartDay: 25}, "2026-10-25", "2026-11-24", false},
		{"year", SummaryFilter{Period: "year", Date: "2026"}, "2026-01-01", "2026-12-31", false},
		{"year ignores start day", SummaryFilter{Period: "year", Date: "2026", MonthStartDay: 25}, "2026-01-01", "2026-12-31", false},
		{"custom", SummaryFilter{Period: "custom", From: "2026-09-01", To: "2026-09-15"}, "2026-09-01", "2026-09-15", false},
		{"custom single day", SummaryFilter{Period: "custom", From: "2026-09-01", To: "2026-09-01"}, "2026-09-01", "2026-09-01", false},
		{"custom reversed", SummaryFilter{Period: "custom", From: "2026-09-15", To: "2026-09-01"}, "", "", true},
		{"custom too long", SummaryFilter{Period: "custom", From: "2020-01-01", To: "2026-01-01"}, "", "", true},
		{"custom missing to", SummaryFilter{Period: "custom", From: "2026-09-01"}, "", "", true},
		{"day without date", SummaryFilter{Period: "day"}, "", "", true},
		{"unknown period", SummaryFilter{Period: "decade", Date: "2026"}, "", "", true},
	}
	for _, tt := range tests {
		start, end, err := periodRange(tt.filter)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if got := start.Format(dateLayout); got != tt.wantStart {
			t.Errorf("%s: start = %s, want %s", tt.name, got, tt.wantStart)
		}
		if got := end.Format(dateLayout); got != tt.wantEnd {
			t.Errorf("%s: end = %s, want %s", tt.name, got, tt.wantEnd)
		}
	}
}

func TestNormalizeDefaultsAndValidation(t *testing.T) {
	filter, _, _, err := normalize(SummaryFilter{Period: "day", Date: "2026-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	if filter.Type != "expense" || filter.TimeZone != "UTC" {
		t.Errorf("defaults = %q/%q, want expense/UTC", filter.Type, filter.TimeZone)
	}

	for _, bad := range []SummaryFilter{
		{Period: "day", Date: "2026-09-30", Type: "transfer"},
		{Period: "day", Date: "2026-09-30", TimeZone: "Mars/Olympus"},
		{Period: "day", Date: "2026-09-30", TimeZone: "Local"},
	} {
		if _, _, _, err := normalize(bad); err == nil {
			t.Errorf("normalize(%+v): want error", bad)
		}
	}
	if _, _, _, err := normalize(SummaryFilter{Period: "day", Date: "2026-09-30", TimeZone: "Asia/Bangkok"}); err != nil {
		t.Errorf("Asia/Bangkok: %v", err)
	}
}

func TestFormatCents(t *testing.T) {
	for cents, want := range map[int64]string{0: "0.00", 5: "0.05", 1234: "12.34", -250: "-2.50"} {
		if got := formatCents(big.NewInt(cents)); got != want {
			t.Errorf("formatCents(%d) = %s, want %s", cents, got, want)
		}
	}
}

func TestBuildCategories(t *testing.T) {
	s := func(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
	none := sql.NullString{}
	grouped := []categoryRow{
		// Transport: one subcategory plus a transaction filed on the parent.
		{topID: s("t"), topName: s("Transport"), subID: s("bus"), subName: s("Bus"), cents: big.NewInt(3000), count: 2},
		{topID: s("t"), topName: s("Transport"), cents: big.NewInt(1000), count: 1},
		// Food: no subcategories at all.
		{topID: s("f"), topName: s("Food"), cents: big.NewInt(5000), count: 3},
		// No category.
		{topID: none, topName: none, cents: big.NewInt(1000), count: 1},
	}

	got := buildCategories(grouped, "expense", big.NewInt(10000))
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Category.Name != "Food" || got[0].Total != "50.00" || got[0].Percentage != 50 || len(got[0].Subcategories) != 0 {
		t.Errorf("first = %+v, want Food 50.00 50%% with no subcategories", got[0])
	}
	transport := got[1]
	if transport.Category.Name != "Transport" || transport.Total != "40.00" || transport.TransactionCount != 3 {
		t.Errorf("second = %+v, want Transport 40.00 x3", transport)
	}
	if len(transport.Subcategories) != 2 || transport.Subcategories[0].Name != "Bus" || transport.Subcategories[1].Name != "Other" {
		t.Errorf("transport subcategories = %+v, want Bus then Other", transport.Subcategories)
	}
	if got[2].Category.Name != "Uncategorized" || got[2].Category.ID != "" {
		t.Errorf("third = %+v, want Uncategorized", got[2])
	}
}

func TestFormatCentsDoesNotOverflow(t *testing.T) {
	// 10 x 9999999999999999.99 in cents exceeds int64.
	huge, _ := new(big.Int).SetString("9999999999999999999", 10)
	huge.Mul(huge, big.NewInt(10))
	if got, want := formatCents(huge), "999999999999999999.90"; got != want {
		t.Errorf("formatCents = %s, want %s", got, want)
	}
}
