package gooser_test

import (
	"context"
	"embed"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/sousair/gocore/pkg/gooser"
	gocoretest "github.com/sousair/gocore/pkg/testing"
)

//go:embed testdata/migrations/*.sql
var testMigrations embed.FS

//go:embed testdata/migrations_b/*.sql
var testMigrationsB embed.FS

func TestUp_appliesEmbeddedMigrations(t *testing.T) {
	dsn := gocoretest.FreshDB(t) // Task 1A helper; skips if GOCORE_TEST_ADMIN_DSN unset
	ctx := context.Background()

	if err := gooser.Up(ctx, dsn, testMigrations, "testdata/migrations"); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	// idempotent: a second Up is a no-op, not an error
	if err := gooser.Up(ctx, dsn, testMigrations, "testdata/migrations"); err != nil {
		t.Fatalf("second Up (idempotent): %v", err)
	}
}

// TestUp_concurrentCallsSerializeSafely runs two Up calls in parallel
// against two distinct databases with two distinct migration sets, guarding
// against the goose v3 package-global race (SetBaseFS/SetDialect mutate
// state that UpContext reads): each call must apply only its own
// migrations, with no cross-contamination, and neither call may error.
func TestUp_concurrentCallsSerializeSafely(t *testing.T) {
	dsnA := gocoretest.FreshDB(t)
	dsnB := gocoretest.FreshDB(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- gooser.Up(ctx, dsnA, testMigrations, "testdata/migrations")
	}()
	go func() {
		defer wg.Done()
		errs <- gooser.Up(ctx, dsnB, testMigrationsB, "testdata/migrations_b")
	}()
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Up: %v", err)
		}
	}

	assertTableExists(ctx, t, dsnA, "gooser_probe")
	assertTableNotExists(ctx, t, dsnA, "gooser_probe_b")
	assertTableExists(ctx, t, dsnB, "gooser_probe_b")
	assertTableNotExists(ctx, t, dsnB, "gooser_probe")
}

func assertTableExists(ctx context.Context, t *testing.T, dsn, table string) {
	t.Helper()
	if !tableExists(ctx, t, dsn, table) {
		t.Fatalf("expected table %q to exist in %s but it does not", table, dsn)
	}
}

func assertTableNotExists(ctx context.Context, t *testing.T, dsn, table string) {
	t.Helper()
	if tableExists(ctx, t, dsn, table) {
		t.Fatalf("expected table %q to NOT exist in %s but it does", table, dsn)
	}
}

func tableExists(ctx context.Context, t *testing.T, dsn, table string) bool {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to %s: %v", dsn, err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var exists bool
	err = conn.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %q existence: %v", table, err)
	}
	return exists
}
