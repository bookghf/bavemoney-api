package month

import (
	"testing"
	"time"
)

func TestRange(t *testing.T) {
	tests := []struct {
		day      string
		startDay int
		from, to string
	}{
		{"2026-10-05", 1, "2026-10-01", "2026-10-31"},
		{"2026-02-14", 1, "2026-02-01", "2026-02-28"},
		{"2026-10-05", 25, "2026-09-25", "2026-10-24"},
		{"2026-10-24", 25, "2026-09-25", "2026-10-24"},
		{"2026-10-25", 25, "2026-10-25", "2026-11-24"},
		{"2026-01-10", 25, "2025-12-25", "2026-01-24"},
		{"2026-12-31", 25, "2026-12-25", "2027-01-24"},
		{"2026-03-01", 28, "2026-02-28", "2026-03-27"},
		{"2026-02-28", 28, "2026-02-28", "2026-03-27"},
		// Out-of-range days fall back to calendar months.
		{"2026-10-05", 0, "2026-10-01", "2026-10-31"},
		{"2026-10-05", 31, "2026-10-01", "2026-10-31"},
	}
	for _, tt := range tests {
		d, _ := time.Parse(time.DateOnly, tt.day)
		from, to := Range(d, tt.startDay)
		if from.Format(time.DateOnly) != tt.from || to.Format(time.DateOnly) != tt.to {
			t.Errorf("Range(%s, %d) = %s..%s, want %s..%s", tt.day, tt.startDay,
				from.Format(time.DateOnly), to.Format(time.DateOnly), tt.from, tt.to)
		}
	}
}

func TestValidStartDay(t *testing.T) {
	for day, want := range map[int]bool{0: false, 1: true, 15: true, 28: true, 29: false, -3: false} {
		if got := ValidStartDay(day); got != want {
			t.Errorf("ValidStartDay(%d) = %v, want %v", day, got, want)
		}
	}
}
