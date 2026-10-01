package validate

import "testing"

func TestAmount(t *testing.T) {
	valid := []string{"1", "0.01", "12.5", "9999999999999999.99"}
	invalid := []string{"", "0", "0.00", "-5", "1.234", "1e5", "1,000", "12abc", "99999999999999999", ".5"}
	for _, s := range valid {
		if !Amount(s) {
			t.Errorf("Amount(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if Amount(s) {
			t.Errorf("Amount(%q) = true, want false", s)
		}
	}
}

func TestSignedAmount(t *testing.T) {
	for _, s := range []string{"0", "-120.50", "1000.00"} {
		if !SignedAmount(s) {
			t.Errorf("SignedAmount(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "--1", "1.001", "1e20"} {
		if SignedAmount(s) {
			t.Errorf("SignedAmount(%q) = true, want false", s)
		}
	}
}

func TestTimestampAndDate(t *testing.T) {
	if _, ok := Timestamp("2026-02-30T10:00:00Z"); ok {
		t.Error("Timestamp accepted Feb 30")
	}
	if _, ok := Timestamp("2026-09-25T10:00:00+07:00"); !ok {
		t.Error("Timestamp rejected a valid offset timestamp")
	}
	if _, ok := Date("2026-13-01"); ok {
		t.Error("Date accepted month 13")
	}
}

func TestUUIDAndCurrency(t *testing.T) {
	if !UUID("8f14e45f-ceea-467e-a0b6-4f2c5d2b1a3c") || UUID("not-a-uuid") {
		t.Error("UUID check is wrong")
	}
	if !Currency("THB") || Currency("thb") || Currency("THBB") {
		t.Error("Currency check is wrong")
	}
}

func TestEmail(t *testing.T) {
	if !Email("a@b.co") || Email("notanemail") || Email("a b@c.d") {
		t.Error("Email check is wrong")
	}
	if NormalizeEmail("  QA@Example.COM ") != "qa@example.com" {
		t.Error("NormalizeEmail did not lowercase and trim")
	}
}
