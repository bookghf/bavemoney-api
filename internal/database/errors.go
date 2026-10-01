package database

import (
	"errors"

	"github.com/lib/pq"
)

// PostgreSQL error codes the handlers map to client errors.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
	codeNumericOverflow     = "22003"
)

func hasCode(err error, code string) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && string(pqErr.Code) == code
}

// IsUniqueViolation reports whether err is a unique constraint failure.
func IsUniqueViolation(err error) bool { return hasCode(err, codeUniqueViolation) }

// IsForeignKeyViolation reports whether err references a missing row, such as
// an unknown currency code.
func IsForeignKeyViolation(err error) bool { return hasCode(err, codeForeignKeyViolation) }

// IsCheckViolation reports whether err is a CHECK constraint failure.
func IsCheckViolation(err error) bool { return hasCode(err, codeCheckViolation) }

// IsNumericOverflow reports whether a value or aggregate overflowed its type.
func IsNumericOverflow(err error) bool { return hasCode(err, codeNumericOverflow) }
