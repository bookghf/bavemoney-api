// Package validate holds the input checks shared by every handler, so that
// malformed ids, dates, and amounts are rejected with a 400 instead of
// reaching PostgreSQL and surfacing as a 500.
package validate

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	emailPattern    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	// Amounts fit NUMERIC(18,2): at most 16 integer digits and 2 decimals.
	amountPattern       = regexp.MustCompile(`^\d{1,16}(\.\d{1,2})?$`)
	signedAmountPattern = regexp.MustCompile(`^-?\d{1,16}(\.\d{1,2})?$`)
)

// UUID reports whether s is a canonical UUID.
func UUID(s string) bool { return uuidPattern.MatchString(s) }

// Currency reports whether s looks like an ISO 4217 code ("THB").
func Currency(s string) bool { return currencyPattern.MatchString(s) }

// Email reports whether s is a plausible email address.
func Email(s string) bool { return len(s) <= 254 && emailPattern.MatchString(s) }

// NormalizeEmail trims and lowercases an email so lookups are case-insensitive.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Amount reports whether s is a strictly positive decimal that fits NUMERIC(18,2).
func Amount(s string) bool {
	return amountPattern.MatchString(s) && strings.Trim(s, "0.") != ""
}

// SignedAmount reports whether s is a decimal (possibly negative or zero) that
// fits NUMERIC(18,2).
func SignedAmount(s string) bool { return signedAmountPattern.MatchString(s) }

// Timestamp parses an RFC 3339 timestamp, rejecting impossible dates.
func Timestamp(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

// Date parses a YYYY-MM-DD calendar date, rejecting impossible dates.
func Date(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

// Location resolves an IANA time zone name, defaulting to UTC when empty.
func Location(name string) (*time.Location, bool) {
	if name == "" {
		return time.UTC, true
	}
	loc, err := time.LoadLocation(name)
	return loc, err == nil
}

// Decimal is a JSON money input that accepts a decimal string ("1000.00") as
// the API spec defines, and also a bare JSON number for older clients. The
// raw text is kept, so no float rounding happens; validate it with
// SignedAmount or Amount.
type Decimal string

// UnmarshalJSON implements json.Unmarshaler.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, `"`) {
		unquoted, err := strconv.Unquote(text)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(unquoted)
	}
	*d = Decimal(text)
	return nil
}
