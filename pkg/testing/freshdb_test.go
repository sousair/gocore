package testing_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	gocoretest "github.com/sousair/gocore/pkg/testing"
)

func TestFreshDB_isConnectableAndEmpty(t *testing.T) {
	dsn := gocoretest.FreshDB(t) // skips if GOCORE_TEST_ADMIN_DSN unset
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect fresh db: %v", err)
	}
	defer conn.Close(ctx)
	var n int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("fresh db has %d public tables, want 0", n)
	}
}

// TestFreshDB_preservesAdminDSNQueryParams guards against connString-style
// rebuilds that silently drop query params (or mangle escaped credentials)
// when swapping in the fresh database name. The returned DSN must still be a
// valid, connectable DSN that carries over every runtime param the admin DSN
// had — proven here via application_name, which pgx surfaces on
// cfg.RuntimeParams after a round-trip through pgx.ParseConfig.
func TestFreshDB_preservesAdminDSNQueryParams(t *testing.T) {
	admin := os.Getenv("GOCORE_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("GOCORE_TEST_ADMIN_DSN not set; skipping DB test")
	}
	t.Setenv("GOCORE_TEST_ADMIN_DSN", admin+"&application_name=freshdb_test")

	dsn := gocoretest.FreshDB(t)

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("returned dsn does not parse: %v", err)
	}
	if cfg.Database == "" || cfg.Database == "postgres" {
		t.Fatalf("expected dsn to point at a fresh database, got %q", cfg.Database)
	}
	if got := cfg.RuntimeParams["application_name"]; got != "freshdb_test" {
		t.Fatalf("expected application_name=freshdb_test preserved from admin dsn, got runtime params %v", cfg.RuntimeParams)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect with returned dsn: %v", err)
	}
	defer conn.Close(ctx)
}
