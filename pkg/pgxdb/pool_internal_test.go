package pgxdb

import (
	"context"
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const testDSN = "postgres://user:pass@localhost:5432/db"

func TestBuildConfig(t *testing.T) {
	t.Run("given a valid dsn/when buildConfig runs/then the conn config carries an otelpgx tracer", func(t *testing.T) {
		cfg, err := buildConfig(testDSN)
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.ConnConfig.Tracer == nil {
			t.Fatal("ConnConfig.Tracer is nil — DB spans would never be emitted")
		}
		if _, ok := cfg.ConnConfig.Tracer.(*queryOnlyTracer); !ok {
			t.Fatalf("ConnConfig.Tracer is %T, want *queryOnlyTracer", cfg.ConnConfig.Tracer)
		}
	})

	t.Run("given an unparseable dsn/when buildConfig runs/then it returns an error", func(t *testing.T) {
		if _, err := buildConfig("://not-a-dsn"); err == nil {
			t.Fatal("expected an error for an unparseable dsn, got nil")
		}
	})

	t.Run("given a floor above the dsn value/when WithMaxConnsFloor is applied/then MaxConns is raised", func(t *testing.T) {
		cfg, err := buildConfig(testDSN+"?pool_max_conns=4", WithMaxConnsFloor(12))
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.MaxConns != 12 {
			t.Fatalf("MaxConns = %d, want 12", cfg.MaxConns)
		}
	})

	t.Run("given a dsn already above the floor/when WithMaxConnsFloor is applied/then MaxConns is left alone", func(t *testing.T) {
		cfg, err := buildConfig(testDSN+"?pool_max_conns=20", WithMaxConnsFloor(12))
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.MaxConns != 20 {
			t.Fatalf("MaxConns = %d, want 20 — the floor must not lower an explicit request", cfg.MaxConns)
		}
	})

	t.Run("given an after-connect hook/when WithAfterConnect is applied/then the hook is registered", func(t *testing.T) {
		called := false
		hook := func(context.Context, *pgx.Conn) error { called = true; return nil }
		cfg, err := buildConfig(testDSN, WithAfterConnect(hook))
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.AfterConnect == nil {
			t.Fatal("AfterConnect is nil — the consumer's hook was dropped")
		}
		if err := cfg.AfterConnect(context.Background(), nil); err != nil {
			t.Fatalf("AfterConnect: %v", err)
		}
		if !called {
			t.Fatal("AfterConnect did not invoke the supplied hook")
		}
	})

	t.Run("given an option that clears the tracer/when buildConfig runs/then the tracer is still set", func(t *testing.T) {
		clear := Option(func(cfg *pgxpool.Config) { cfg.ConnConfig.Tracer = nil })
		cfg, err := buildConfig(testDSN, clear)
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.ConnConfig.Tracer == nil {
			t.Fatal("an option cleared the tracer — options must be applied before it is set")
		}
	})
}

func TestQueryOnlyTracerInterfaces(t *testing.T) {
	tracer, err := buildConfig(testDSN)
	if err != nil {
		t.Fatalf("buildConfig: %v", err)
	}

	t.Run("given the configured tracer/when it is assigned to ConnConfig.Tracer/then it is a non-nil *queryOnlyTracer", func(t *testing.T) {
		// pgx.ConnConfig.Tracer is itself typed pgx.QueryTracer, so satisfying
		// that interface is enforced at compile time by `var _ pgx.QueryTracer
		// = (*queryOnlyTracer)(nil)` in pool.go.
		if _, ok := tracer.ConnConfig.Tracer.(*queryOnlyTracer); !ok {
			t.Fatalf("ConnConfig.Tracer is %T, want *queryOnlyTracer", tracer.ConnConfig.Tracer)
		}
	})

	t.Run("given the configured tracer/when asserted against pgx.PrepareTracer/then it does not satisfy the interface", func(t *testing.T) {
		// This is the whole point of the change: pgx type-asserts each tracer
		// interface separately, and otelpgx's inner tracer would satisfy
		// PrepareTracer if we embedded it instead of wrapping it.
		if _, ok := tracer.ConnConfig.Tracer.(pgx.PrepareTracer); ok {
			t.Fatalf("%T satisfies pgx.PrepareTracer — prepare spans would fire again", tracer.ConnConfig.Tracer)
		}
	})
}

func TestSpanName(t *testing.T) {
	t.Run("given a sqlc-generated query/when spanName runs/then it returns the sqlc query name", func(t *testing.T) {
		sql := "-- name: ListBudgetTargetUserIDs :many\nSELECT DISTINCT user_id FROM budget_target"
		if got := spanName(sql); got != "ListBudgetTargetUserIDs" {
			t.Fatalf("spanName = %q, want %q", got, "ListBudgetTargetUserIDs")
		}
	})

	t.Run("given hand-written SQL with no sqlc header/when spanName runs/then it falls back to the first word", func(t *testing.T) {
		if got := spanName("SELECT 1"); got != "SELECT" {
			t.Fatalf("spanName = %q, want %q", got, "SELECT")
		}
	})

	t.Run("given empty SQL/when spanName runs/then it returns a non-empty fallback without panicking", func(t *testing.T) {
		if got := spanName(""); got == "" {
			t.Fatal("spanName returned empty string for empty input")
		}
	})

	t.Run("given whitespace-only SQL/when spanName runs/then it returns a non-empty fallback without panicking", func(t *testing.T) {
		if got := spanName("   \n\t  "); got == "" {
			t.Fatal("spanName returned empty string for whitespace-only input")
		}
	})
}

func TestQueryOnlyTracerSkipsTxControl(t *testing.T) {
	newRecordedTracer := func() (*queryOnlyTracer, *tracetest.SpanRecorder, oteltrace.Tracer) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		inner := otelpgx.NewTracer(otelpgx.WithTracerProvider(tp), otelpgx.WithTrimSQLInSpanName())
		return &queryOnlyTracer{inner: inner}, rec, tp.Tracer("test")
	}

	for _, stmt := range []string{"BEGIN", "begin", "  COMMIT", "ROLLBACK", "SAVEPOINT s1", "RELEASE s1"} {
		t.Run("given "+stmt+"/when TraceQueryStart runs/then the returned ctx carries no span", func(t *testing.T) {
			tracer, rec, tr := newRecordedTracer()
			ctx, parent := tr.Start(context.Background(), "parent")
			defer parent.End()

			skipCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: stmt})
			tracer.TraceQueryEnd(skipCtx, nil, pgx.TraceQueryEndData{})

			if len(rec.Ended()) != 0 {
				t.Fatalf("got %d ended spans for a skipped tx-control statement, want 0", len(rec.Ended()))
			}
		})
	}

	t.Run("given a tx-control statement/when TraceQueryEnd runs on the skipped ctx/then the ambient parent span is not ended", func(t *testing.T) {
		tracer, rec, tr := newRecordedTracer()
		ctx, parent := tr.Start(context.Background(), "parent")

		skipCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "BEGIN"})
		tracer.TraceQueryEnd(skipCtx, nil, pgx.TraceQueryEndData{})

		if !parent.IsRecording() {
			t.Fatal("parent span was ended by TraceQueryEnd on a skipped statement — the wrong span was closed")
		}

		parent.End()
		ended := rec.Ended()
		if len(ended) != 1 || ended[0].Name() != "parent" {
			t.Fatalf("ended spans = %v, want exactly [parent]", ended)
		}
	})

	t.Run("given a real query/when TraceQueryStart and TraceQueryEnd run/then a query span is emitted and the parent survives", func(t *testing.T) {
		tracer, rec, tr := newRecordedTracer()
		ctx, parent := tr.Start(context.Background(), "parent")

		qCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
		tracer.TraceQueryEnd(qCtx, nil, pgx.TraceQueryEndData{})

		if !parent.IsRecording() {
			t.Fatal("parent span was ended while tracing an unrelated real query")
		}
		parent.End()

		ended := rec.Ended()
		if len(ended) != 2 {
			t.Fatalf("got %d ended spans, want 2 (query + parent)", len(ended))
		}
	})
}
