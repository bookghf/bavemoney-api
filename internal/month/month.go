// Package month works out a user's months. Someone paid on the 25th can start
// their month on that day, so the month containing 5 Oct runs 25 Sep - 24 Oct.
// A start day of 1 gives calendar months.
package month

import "time"

// MaxStartDay is the latest allowed start day, so every month has that day.
const MaxStartDay = 28

// ValidStartDay reports whether day can start a month (1-28).
func ValidStartDay(day int) bool {
	return day >= 1 && day <= MaxStartDay
}

// Start returns the first day of the month that contains day t when months
// begin on startDay. An invalid startDay falls back to calendar months.
func Start(t time.Time, startDay int) time.Time {
	if !ValidStartDay(startDay) {
		startDay = 1
	}
	start := time.Date(t.Year(), t.Month(), startDay, 0, 0, 0, 0, t.Location())
	if t.Day() < startDay {
		// startDay <= 28, so stepping back a month never overflows.
		start = start.AddDate(0, -1, 0)
	}
	return start
}

// Range returns the first and last day (inclusive) of the month containing t.
func Range(t time.Time, startDay int) (time.Time, time.Time) {
	start := Start(t, startDay)
	return start, start.AddDate(0, 1, -1)
}
