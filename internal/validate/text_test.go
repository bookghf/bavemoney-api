package validate

import (
	"strings"
	"testing"
)

func TestPassword(t *testing.T) {
	for _, ok := range []string{"correct horse", "รหัสผ่านยาวมาก", strings.Repeat("a", 72)} {
		if err := Password(ok); err != nil {
			t.Errorf("Password(%q) = %v", ok, err)
		}
	}
	for name, bad := range map[string]string{
		"blank":         "        ",
		"short":         "1234567",
		"73 bytes":      strings.Repeat("a", 73),
		"NUL":           "abc\x00defgh",
		"80 Thai bytes": strings.Repeat("ก", 25), // 25 runes, 75 bytes
	} {
		if Password(bad) == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestNameAndFreeText(t *testing.T) {
	if Name("สมชาย 😀", 60, "name") != nil {
		t.Error("Thai and emoji names must pass")
	}
	if Name(strings.Repeat("ก", 61), 60, "name") == nil {
		t.Error("61 runes must fail")
	}
	for _, bad := range []string{"a\x00b", "line\nbreak", "tab\there"} {
		if Name(bad, 60, "name") == nil {
			t.Errorf("Name(%q): want error", bad)
		}
	}
	if FreeText("line one\nline two\ttab", 500, "note") != nil {
		t.Error("notes may contain newlines and tabs")
	}
	if FreeText("a\x00b", 500, "note") == nil {
		t.Error("NUL in a note must fail")
	}
}
