package gooser_test

import (
	"context"
	"embed"
	"testing"

	"github.com/sousair/gocore/pkg/gooser"
	gocoretest "github.com/sousair/gocore/pkg/testing"
)

//go:embed testdata/migrations/*.sql
var testMigrations embed.FS

func TestUp_appliesEmbeddedMigrations(t *testing.T) {
	dsn := gocoretest.FreshDB(t) // Task 1A helper; skips if GOCORE_TEST_ADMIN_DSN unset
	ctx := context.Background()

	if err := gooser.Up(ctx, dsn, testMigrations, "testdata/migrations"); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	// idempotent: a second Up is a no-op, not an error
	if err := gooser.Up(ctx, dsn, testMigrations, "testdata/migrations"); err != nil {
		t.Fatalf("second Up (idempotent): %v", err)
	}
}
