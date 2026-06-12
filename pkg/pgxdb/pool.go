// Package pgxdb provides a pgx connection pool and a fail-closed
// row-level-security (RLS) transaction helper.
//
// # Fail-closed RLS pattern
//
// Policies should be written so that an absent or empty GUC matches no rows:
//
//	USING (id = NULLIF(current_setting('app.current_user_id', true), '')::uuid)
//
// The NULLIF is essential because set_config with the third argument true
// (transaction-local) reverts the GUC to its pre-transaction value when the
// transaction ends — not to NULL, but to the empty string on a pooled
// connection that has never had the GUC set at the session level. Without
// NULLIF the empty string would be cast to a zero UUID and silently match
// rows, defeating RLS. With NULLIF, '' folds to NULL and the comparison
// returns NULL (not true), so no rows are visible — the policy is
// fail-closed.
package pgxdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgxpool.Pool from dsn and verifies connectivity with a
// Ping. If the ping fails the pool is closed before the error is returned, so
// callers never receive a pool in a broken state.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxdb: new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxdb: ping: %w", err)
	}
	return pool, nil
}
