// Package database opens the PostgreSQL connection pool and holds small SQL
// helpers shared by the repositories.
package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Connect opens a connection pool and verifies it is reachable.
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

// NullIfEmpty maps an empty string to a SQL NULL so optional columns stay null
// instead of being written as "".
func NullIfEmpty(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
