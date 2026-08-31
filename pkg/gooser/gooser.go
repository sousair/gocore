// Package gooser runs embedded goose migrations at application boot,
// using the application's own database role (no superuser).
package gooser

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sync"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// mu serializes access to goose v3's package-level globals (base FS,
// dialect), which SetBaseFS/SetDialect mutate and UpContext reads. Without
// it, concurrent Up calls could race and leak one caller's fsys/dialect into
// another's migration run.
var mu sync.Mutex

// Up applies all pending Up migrations from fsys/dir against dsn.
// It is idempotent: with nothing pending it is a no-op. Up is safe for
// concurrent use — internally it serializes so only one migration run
// happens at a time.
func Up(ctx context.Context, dsn string, fsys fs.FS, dir string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("gooser: open: %w", err)
	}
	defer func() { _ = db.Close() }()

	mu.Lock()
	defer mu.Unlock()

	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("gooser: dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("gooser: up: %w", err)
	}
	return nil
}
