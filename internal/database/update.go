package database

import (
	"fmt"
	"strings"
)

// UpdateBuilder assembles a partial UPDATE scoped to a single owned row:
//
//	UPDATE <table> SET updated_at = now(), <col> = $1, ... WHERE id = $n AND user_id = $n+1
//
// Placeholders are numbered as columns are added, so only the fields present in
// a PATCH request end up in the statement.
type UpdateBuilder struct {
	table string
	sets  []string
	args  []interface{}
}

// NewUpdate starts a builder for the given table.
func NewUpdate(table string) *UpdateBuilder {
	return &UpdateBuilder{table: table}
}

// Set adds a column assignment.
func (b *UpdateBuilder) Set(column string, value interface{}) *UpdateBuilder {
	b.args = append(b.args, value)
	b.sets = append(b.sets, fmt.Sprintf("%s = $%d", column, len(b.args)))
	return b
}

// Empty reports whether any column was set.
func (b *UpdateBuilder) Empty() bool {
	return len(b.sets) == 0
}

// Build returns the statement and its arguments for the row owned by userID.
func (b *UpdateBuilder) Build(id, userID string) (string, []interface{}) {
	args := append(append([]interface{}{}, b.args...), id, userID)

	var query strings.Builder
	query.WriteString("UPDATE " + b.table + " SET updated_at = now()")
	for _, set := range b.sets {
		query.WriteString(", " + set)
	}
	fmt.Fprintf(&query, " WHERE id = $%d AND user_id = $%d", len(args)-1, len(args))

	return query.String(), args
}
