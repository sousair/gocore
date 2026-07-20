// Package gooser runs embedded goose migrations at application boot,
// using the application's own database role (no superuser).
package gooser

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// Up applies all pending Up migrations from fsys/dir against dsn.
// It is idempotent: with nothing pending it is a no-op.
func Up(ctx context.Context, dsn string, fsys fs.FS, dir string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("gooser: open: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("gooser: dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("gooser: up: %w", err)
	}
	return nil
}
