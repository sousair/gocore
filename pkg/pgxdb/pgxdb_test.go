package pgxdb_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sousair/gocore/pkg/pgxdb"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("GOCORE_PGX_TEST_DSN")
	if dsn == "" {
		t.Skip("GOCORE_PGX_TEST_DSN unset — set it to run integration tests")
	}
	pool, err := pgxdb.NewPool(context.Background(), dsn)
	if err != nil {
		t.Skipf("GOCORE_PGX_TEST_DSN set but unreachable (%v) — check that the DB is running", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRLS_WithGUC(t *testing.T) {
	t.Run("given a nil pool/when NewRLS constructs with defaults/then it returns a usable value", func(t *testing.T) {
		pool := (*pgxpool.Pool)(nil)
		r := pgxdb.NewRLS(pool)
		if r == nil {
			t.Fatal("NewRLS returned nil")
		}
	})

	t.Run("given a nil pool/when InUserTx is called/then it returns an error not a panic", func(t *testing.T) {
		r := pgxdb.NewRLS((*pgxpool.Pool)(nil))
		err := r.InUserTx(context.Background(), uuid.New(), func(pgx.Tx) error { return nil })
		if err == nil {
			t.Fatal("expected error for nil pool, got nil")
		}
	})

	t.Run("given WithGUC option/when NewRLS is called/then the option is applied without error", func(t *testing.T) {
		pool := (*pgxpool.Pool)(nil)
		r := pgxdb.NewRLS(pool, pgxdb.WithGUC("my.user_id"))
		if r == nil {
			t.Fatal("NewRLS returned nil with WithGUC option")
		}
	})
}

func TestRLS_FailClosedPolicy(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	rls := pgxdb.NewRLS(pool)

	table := fmt.Sprintf("pgxdb_rls_test_%s", uuid.New().String()[:8])

	setup := fmt.Sprintf(`
		CREATE TABLE %s (
			id   uuid PRIMARY KEY,
			owner_id uuid NOT NULL
		);
		ALTER TABLE %s ENABLE ROW LEVEL SECURITY;
		ALTER TABLE %s FORCE ROW LEVEL SECURITY;
		CREATE POLICY select_own ON %s
			FOR SELECT
			USING (owner_id = NULLIF(current_setting('app.current_user_id', true), '')::uuid);
	`, table, table, table, table)

	if _, err := pool.Exec(ctx, setup); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", table)) //nolint:errcheck
	})

	alice, bob := uuid.New(), uuid.New()

	for _, id := range []uuid.UUID{alice, bob} {
		err := rls.InUserTx(ctx, id, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				fmt.Sprintf("INSERT INTO %s (id, owner_id) VALUES ($1, $2)", table),
				id, id,
			)
			return err
		})
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	t.Run("given alice's row exists/when alice reads it/then it is visible", func(t *testing.T) {
		err := rls.InUserTx(ctx, alice, func(tx pgx.Tx) error {
			var got uuid.UUID
			row := tx.QueryRow(ctx,
				fmt.Sprintf("SELECT id FROM %s WHERE id = $1", table), alice)
			if err := row.Scan(&got); err != nil {
				return err
			}
			if got != alice {
				t.Errorf("got %s, want %s", got, alice)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("read own row: %v", err)
		}
	})

	t.Run("given alice's row exists/when bob reads it/then it is invisible", func(t *testing.T) {
		err := rls.InUserTx(ctx, bob, func(tx pgx.Tx) error {
			var got uuid.UUID
			row := tx.QueryRow(ctx,
				fmt.Sprintf("SELECT id FROM %s WHERE id = $1", table), alice)
			err := row.Scan(&got)
			if err != pgx.ErrNoRows {
				t.Errorf("fail-closed violated: want pgx.ErrNoRows, got %v (id=%s)", err, got)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("tx: %v", err)
		}
	})

	t.Run("given no user context is set/when any row is read/then zero rows return", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		var got uuid.UUID
		row := tx.QueryRow(ctx,
			fmt.Sprintf("SELECT id FROM %s WHERE id = $1", table), alice)
		err = row.Scan(&got)
		if err != pgx.ErrNoRows {
			t.Errorf("fail-closed violated: want pgx.ErrNoRows, got %v (id=%s)", err, got)
		}
	})
}
