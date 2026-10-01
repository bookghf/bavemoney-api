// Package migrate applies the SQL migrations embedded in the binary.
//
// db/init only runs when the Postgres volume is first created, so schema
// changes made after that live here instead. Each file runs once, in name
// order, inside its own transaction, and is recorded in schema_migrations.
// Files must stay idempotent so they also apply cleanly on a fresh database
// that db/init has just bootstrapped.
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
)

//go:embed sql/*.sql
var files embed.FS

// Run applies every migration that has not been applied yet.
func Run(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(files, "sql/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if applied {
			continue
		}
		body, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		if err := apply(ctx, db, name, string(body)); err != nil {
			return err
		}
		log.Printf("migrate: applied %s", name)
	}
	return nil
}

func apply(ctx context.Context, db *sql.DB, name, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	return tx.Commit()
}
