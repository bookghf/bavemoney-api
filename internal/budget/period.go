package budget

import (
	"errors"
	"strings"
	"time"

	"ledger-api/internal/month"
	"ledger-api/internal/validate"
)

const defaultThreshold = 80

// currentWindow returns the [from, to) days of the budget period that contains
// today. Periods repeat back to back from start; before start, the first
// period is current. Monthly periods are the user's months, which begin on
// monthStartDay, so the first one is the month that contains start.
func currentWindow(start time.Time, period string, today time.Time, monthStartDay int) (time.Time, time.Time) {
	switch period {
	case "monthly":
		from := month.Start(today, monthStartDay)
		if first := month.Start(start, monthStartDay); from.Before(first) {
			from = first
		}
		return from, from.AddDate(0, 1, 0)
	case "weekly":
		weeks := int(today.Sub(start).Hours() / 24 / 7)
		if weeks < 0 {
			weeks = 0
		}
		from := start.AddDate(0, 0, 7*weeks)
		return from, from.AddDate(0, 0, 7)
	}

	// Yearly: step from start, not from the previous period, so a Feb 29
	// start keeps landing on Feb 29 in leap years.
	n := 0
	if today.After(start) {
		n = today.Year() - start.Year()
		for n > 0 && addMonths(start, 12*n).After(today) {
			n--
		}
	}
	return addMonths(start, 12*n), addMonths(start, 12*(n+1))
}

// addMonths adds n months, clamping the day to the target month's length
// (Jan 31 + 1 month = Feb 28) instead of overflowing into the next month.
func addMonths(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	lastDay := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), lastDay)-1)
}

func validateCreate(req *CreateRequest) error {
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.CategoryID != "" && !validate.UUID(req.CategoryID) {
		return errors.New("category_id must be a UUID")
	}
	if !validate.Amount(string(req.Amount)) {
		return errors.New("amount must be a positive decimal with at most 2 decimal places")
	}
	if !validate.Currency(req.Currency) {
		return errors.New("currency must be a 3-letter code such as THB")
	}
	if !periods[req.Period] {
		return errors.New("period must be weekly, monthly, or yearly")
	}
	if _, ok := validate.Date(req.StartDate); !ok {
		return errors.New("start_date must be YYYY-MM-DD")
	}
	if req.AlertThresholdPct == 0 {
		req.AlertThresholdPct = defaultThreshold
	}
	return checkThreshold(req.AlertThresholdPct)
}

func validateUpdate(req UpdateRequest) error {
	if req.Amount != nil && !validate.Amount(string(*req.Amount)) {
		return errors.New("amount must be a positive decimal with at most 2 decimal places")
	}
	if req.Period != nil && !periods[*req.Period] {
		return errors.New("period must be weekly, monthly, or yearly")
	}
	if req.AlertThresholdPct != nil {
		return checkThreshold(*req.AlertThresholdPct)
	}
	return nil
}

func checkThreshold(pct int) error {
	if pct < 1 || pct > 100 {
		return errors.New("alert_threshold_pct must be between 1 and 100")
	}
	return nil
}
