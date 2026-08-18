package pgxdb

import (
	"context"
	"fmt"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Option adjusts the pool config before the pool is opened.
type Option func(*pgxpool.Config)

// WithMaxConnsFloor raises MaxConns to n when the DSN asks for fewer. It never
// lowers an explicit request.
func WithMaxConnsFloor(n int32) Option {
	return func(cfg *pgxpool.Config) {
		if cfg.MaxConns < n {
			cfg.MaxConns = n
		}
	}
}

// WithAfterConnect registers a hook run on each new connection — the seam for
// per-connection type registration such as pgvector.
func WithAfterConnect(fn func(context.Context, *pgx.Conn) error) Option {
	return func(cfg *pgxpool.Config) { cfg.AfterConnect = fn }
}

// buildConfig parses dsn, applies opts, then attaches the tracer. The tracer is
// set last on purpose: a consumer option must not be able to remove it, because
// a pool without it emits no DB spans and the omission is invisible until
// someone opens a trace and finds nothing under the usecase span.
func buildConfig(dsn string, opts ...Option) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxdb: parse config: %w", err)
	}
	for _, opt := range opts {
		opt(cfg)
	}
	// WithIncludeQueryParameters is deliberately absent — it would put query
	// arguments into span attributes, and these pools carry financial and
	// personal data. WithDisableAcquireTracer drops a span per pool acquire,
	// which is pure volume with no diagnostic value at 100% sampling.
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(
		otelpgx.WithTrimSQLInSpanName(),
		otelpgx.WithDisableAcquireTracer(),
	)
	return cfg, nil
}

// NewPool creates a pgxpool.Pool from dsn and verifies connectivity with a
// Ping. If the ping fails the pool is closed before the error is returned, so
// callers never receive a pool in a broken state. Every pool it returns emits
// OTel spans for the queries run through it.
func NewPool(ctx context.Context, dsn string, opts ...Option) (*pgxpool.Pool, error) {
	cfg, err := buildConfig(dsn, opts...)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxdb: new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxdb: ping: %w", err)
	}
	return pool, nil
}
