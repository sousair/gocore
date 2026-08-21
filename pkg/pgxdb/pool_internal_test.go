package pgxdb

import (
	"context"
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
		if _, ok := cfg.ConnConfig.Tracer.(*otelpgx.Tracer); !ok {
			t.Fatalf("ConnConfig.Tracer is %T, want *otelpgx.Tracer", cfg.ConnConfig.Tracer)
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

func TestBuildConfigTracerNamesSpansAfterSqlcQuery(t *testing.T) {
	t.Run("given a pool built by buildConfig/when a sqlc-generated statement is traced/then the emitted span is named after the sqlc query, not the trimmed SQL", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		prevTP := otel.GetTracerProvider()
		otel.SetTracerProvider(tp)
		defer otel.SetTracerProvider(prevTP)

		cfg, err := buildConfig(testDSN)
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		tracer := cfg.ConnConfig.Tracer
		if tracer == nil {
			t.Fatal("ConnConfig.Tracer is nil")
		}

		parentCtx, parent := tp.Tracer("test").Start(context.Background(), "parent")

		sql := "-- name: ListBudgetTargetUserIDs :many\nSELECT DISTINCT user_id FROM budget_target"
		ctx := tracer.TraceQueryStart(parentCtx, nil, pgx.TraceQueryStartData{SQL: sql})
		tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
		parent.End()

		var queriedName string
		var found bool
		for _, s := range rec.Ended() {
			if s.Name() != "parent" {
				queriedName, found = s.Name(), true
			}
		}
		if !found {
			t.Fatal("no query span was recorded")
		}
		if queriedName != "query ListBudgetTargetUserIDs" {
			t.Fatalf("span name = %q, want %q", queriedName, "query ListBudgetTargetUserIDs")
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

	t.Run("given a :one sqlc header/when spanName runs/then it returns the sqlc query name", func(t *testing.T) {
		sql := "-- name: GetBudgetTargetByID :one\nSELECT * FROM budget_target WHERE id = $1"
		if got := spanName(sql); got != "GetBudgetTargetByID" {
			t.Fatalf("spanName = %q, want %q", got, "GetBudgetTargetByID")
		}
	})

	t.Run("given an :exec sqlc header/when spanName runs/then it returns the sqlc query name", func(t *testing.T) {
		sql := "-- name: DeleteBudgetTarget :exec\nDELETE FROM budget_target WHERE id = $1"
		if got := spanName(sql); got != "DeleteBudgetTarget" {
			t.Fatalf("spanName = %q, want %q", got, "DeleteBudgetTarget")
		}
	})

	t.Run("given odd spacing in the sqlc header/when spanName runs/then it still returns the sqlc query name", func(t *testing.T) {
		sql := "--name:Foo :many\nSELECT 1"
		if got := spanName(sql); got != "Foo" {
			t.Fatalf("spanName = %q, want %q", got, "Foo")
		}
	})

	t.Run("given hand-written SQL with no sqlc header/when spanName runs/then it falls back to the first word", func(t *testing.T) {
		if got := spanName("SELECT 1"); got != "SELECT" {
			t.Fatalf("spanName = %q, want %q", got, "SELECT")
		}
	})

	t.Run("given a transaction-control statement/when spanName runs/then it falls back to the uppercased first word", func(t *testing.T) {
		if got := spanName("begin"); got != "BEGIN" {
			t.Fatalf("spanName = %q, want %q", got, "BEGIN")
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

	t.Run("given a name: comment that is not at the start of the statement/when spanName runs/then it is not treated as the sqlc header", func(t *testing.T) {
		sql := "SELECT 1 -- name: NotAHeader :many"
		if got := spanName(sql); got != "SELECT" {
			t.Fatalf("spanName = %q, want %q (the anchored regex must not match mid-statement)", got, "SELECT")
		}
	})

	t.Run("given a name: comment on a second line after real SQL/when spanName runs/then it is not treated as the sqlc header", func(t *testing.T) {
		sql := "SELECT 1\n-- name: NotAHeader :many"
		if got := spanName(sql); got != "SELECT" {
			t.Fatalf("spanName = %q, want %q (the header must anchor the whole statement, not just a line)", got, "SELECT")
		}
	})
}
