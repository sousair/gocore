package pgxdb

import (
	"context"
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
