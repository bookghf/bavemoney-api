package validate

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Password limits. bcrypt only hashes the first 72 bytes and errors past
// that, so longer passwords are rejected up front instead of failing as a 500.
const (
	MinPasswordRunes = 8
	MaxPasswordBytes = 72
)

// Password checks a new password: 8+ characters, at most 72 bytes, not only
// whitespace (login trims input, so an all-space password could never be
// typed back in), and no control characters.
func Password(pw string) error {
	switch {
	case strings.TrimSpace(pw) == "":
		return errors.New("password can not be blank")
	case utf8.RuneCountInString(pw) < MinPasswordRunes:
		return errors.New("password must be at least 8 characters")
	case len(pw) > MaxPasswordBytes:
		return errors.New("password must be at most 72 bytes")
	case hasControl(pw, false):
		return errors.New("password contains invalid characters")
	}
	return nil
}

// Name checks a single-line name (display name, account, category): trimmed
// length within max runes and no control characters such as NUL or newlines,
// which PostgreSQL rejects or which break single-line UI.
func Name(name string, maxRunes int, field string) error {
	if utf8.RuneCountInString(name) > maxRunes {
		return errors.New(field + " is too long")
	}
	if !utf8.ValidString(name) || hasControl(name, false) {
		return errors.New(field + " contains invalid characters")
	}
	return nil
}

// FreeText checks multi-line text such as notes: like Name, but newlines and
// tabs are allowed.
func FreeText(text string, maxRunes int, field string) error {
	if utf8.RuneCountInString(text) > maxRunes {
		return errors.New(field + " is too long")
	}
	if !utf8.ValidString(text) || hasControl(text, true) {
		return errors.New(field + " contains invalid characters")
	}
	return nil
}

func hasControl(s string, allowLineBreaks bool) bool {
	for _, r := range s {
		if allowLineBreaks && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
