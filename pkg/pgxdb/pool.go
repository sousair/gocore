package pgxdb

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
	// WithTrimSQLInSpanName must stay alongside WithSpanNameFunc: it is what
	// makes otelpgx call our func for the span name at all, not just for the
	// db.operation.name attribute.
	cfg.ConnConfig.Tracer = &queryOnlyTracer{inner: otelpgx.NewTracer(
		otelpgx.WithTrimSQLInSpanName(),
		otelpgx.WithSpanNameFunc(spanName),
		otelpgx.WithDisableAcquireTracer(),
	)}
	return cfg, nil
}

// queryOnlyTracer implements only pgx.QueryTracer, deliberately not embedding
// otelpgx.Tracer. Embedding would promote TracePrepareStart/TraceBatchStart/
// etc. and pgx type-asserts each tracer interface separately, so a full
// otelpgx.Tracer emits a span per prepared-statement cache miss and per
// batch on top of every query. Wrapping to expose only QueryTracer drops
// those spans with no change in what queries actually run.
type queryOnlyTracer struct {
	inner *otelpgx.Tracer
}

var _ pgx.QueryTracer = (*queryOnlyTracer)(nil)

// txControlSkipped marks a context returned by TraceQueryStart for a
// transaction-control statement. TraceQueryEnd must check it: the inner
// tracer's End unconditionally ends whatever span is on the context, and for
// a context that never got a query span that is the enclosing usecase span.
type txControlSkipped struct{}

func (t *queryOnlyTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if isTxControlSQL(data.SQL) {
		return context.WithValue(ctx, txControlSkipped{}, true)
	}
	return t.inner.TraceQueryStart(ctx, conn, data)
}

func (t *queryOnlyTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if skipped, _ := ctx.Value(txControlSkipped{}).(bool); skipped {
		return
	}
	t.inner.TraceQueryEnd(ctx, conn, data)
}

var txControlStatements = map[string]bool{
	"BEGIN": true, "COMMIT": true, "ROLLBACK": true, "SAVEPOINT": true, "RELEASE": true,
}

func isTxControlSQL(sql string) bool {
	return txControlStatements[strings.ToUpper(firstWord(sql))]
}

var sqlcNameHeader = regexp.MustCompile(`(?i)^\s*--\s*name:\s*(\S+)`)

// spanName names a span after the sqlc query it runs (`ListBudgetTargetUserIDs`
// from `-- name: ListBudgetTargetUserIDs :many`), since otelpgx's own
// first-token trim just returns "--" for every sqlc-generated statement. Hand
// written SQL and River's internal queries have no such header, so they fall
// back to the first word — the same behaviour WithTrimSQLInSpanName gave
// before. Never returns an empty string.
func spanName(stmt string) string {
	if m := sqlcNameHeader.FindStringSubmatch(stmt); m != nil {
		return m[1]
	}
	if word := firstWord(stmt); word != "" {
		return strings.ToUpper(word)
	}
	return "unknown"
}

func firstWord(stmt string) string {
	fields := strings.Fields(stmt)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
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
