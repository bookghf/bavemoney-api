package budget

import (
	"errors"
	"strings"
	"time"

	"ledger-api/internal/validate"
)

const defaultThreshold = 80

// currentWindow returns the [from, to) days of the budget period that contains
// today. Periods repeat back to back from start; before start, the first
// period is current.
func currentWindow(start time.Time, period string, today time.Time) (time.Time, time.Time) {
	step := func(t time.Time, n int) time.Time {
		switch period {
		case "weekly":
			return t.AddDate(0, 0, 7*n)
		case "yearly":
			return addMonths(start, 12*n)
		default:
			// Step from start, not from t, so a 31st start keeps landing on
			// month ends instead of drifting to the 28th.
			return addMonths(start, n)
		}
	}
	if period == "weekly" {
		weeks := int(today.Sub(start).Hours() / 24 / 7)
		if weeks < 0 {
			weeks = 0
		}
		from := step(start, weeks)
		return from, step(from, 1)
	}

	n := 0
	if today.After(start) {
		years := today.Year() - start.Year()
		n = years
		if period == "monthly" {
			n = years*12 + int(today.Month()) - int(start.Month())
		}
		for n > 0 && step(start, n).After(today) {
			n--
		}
	}
	return step(start, n), step(start, n+1)
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
	if !validate.Amount(req.Amount) {
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
	if req.Amount != nil && !validate.Amount(*req.Amount) {
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
