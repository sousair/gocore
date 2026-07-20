package testing_test

import (
	"context"
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
