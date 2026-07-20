package testing

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

var nameSanitizer = regexp.MustCompile(`[^a-z0-9_]`)

// FreshDB creates an empty, uniquely-named database off the admin DSN in
// GOCORE_TEST_ADMIN_DSN and returns a DSN for it. It skips the test if the env
// var is unset, and drops the database on t.Cleanup. No superuser required
// beyond CREATEDB.
func FreshDB(t *testing.T) string {
	t.Helper()
	admin := os.Getenv("GOCORE_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("GOCORE_TEST_ADMIN_DSN not set; skipping DB test")
	}
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(admin)
	if err != nil {
		t.Fatalf("parse admin dsn: %v", err)
	}
	name := "test_" + nameSanitizer.ReplaceAllString(strings.ToLower(t.Name()), "_")
	if len(name) > 50 {
		name = name[:50]
	}
	name = fmt.Sprintf("%s_%d", name, os.Getpid())

	admConn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admConn.Close(ctx)
	if _, err := admConn.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`); err != nil {
		t.Fatalf("pre-drop: %v", err)
	}
	if _, err := admConn.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, admin)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
	})

	cfg.Database = name
	return connString(cfg)
}

// connString rebuilds a DSN from cfg with the overridden database name.
func connString(cfg *pgx.ConnConfig) string {
	sslmode := "disable"
	if cfg.TLSConfig != nil {
		sslmode = "require"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, sslmode)
}
