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
