package testing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var nameSanitizer = regexp.MustCompile(`[^a-z0-9_]`)

// FreshDB creates an empty, uniquely-named database off the admin DSN in
// GOCORE_TEST_ADMIN_DSN and returns a DSN for it. It skips the test if the env
// var is unset, and drops the database on t.Cleanup. No superuser required
// beyond CREATEDB.
func FreshDB(t *testing.T) string {
	t.Helper()
	admin := os.Getenv("GOCORE_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("GOCORE_TEST_ADMIN_DSN not set; skipping DB test")
	}
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(admin)
	if err != nil {
		t.Fatalf("parse admin dsn: %v", err)
	}
	name := freshDBName(t.Name())

	admConn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("GOCORE_TEST_ADMIN_DSN set but unreachable (%v) — check that the DB is running", err)
	}
	defer admConn.Close(ctx)
	if _, err := admConn.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`); err != nil {
		t.Fatalf("pre-drop: %v", err)
	}
	if _, err := admConn.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, admin)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
	})

	dsn, err := withDatabase(admin, cfg, name)
	if err != nil {
		t.Fatalf("build fresh db dsn: %v", err)
	}
	return dsn
}

// freshDBName derives a unique, Postgres-identifier-safe database name from
// the test name. PID alone is not enough — two runners sharing a PID+test
// name (e.g. re-exec'd test binaries, containers with recycled PIDs) would
// collide — so a random suffix is appended too.
func freshDBName(testName string) string {
	base := "test_" + nameSanitizer.ReplaceAllString(strings.ToLower(testName), "_")
	if len(base) > 40 {
		base = base[:40]
	}
	return fmt.Sprintf("%s_%d_%s", base, os.Getpid(), randomSuffix())
}

// randomSuffix returns a short random hex string for name uniqueness. It
// falls back to a nanosecond timestamp in the (extremely unlikely) event
// crypto/rand is unavailable, since uniqueness — not unpredictability — is
// the only property this needs.
func randomSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// withDatabase returns a DSN equivalent to admin but pointed at database
// name, preserving everything else (credential escaping, sslmode,
// verify-ca/verify-full, and any other query params) instead of rebuilding
// the DSN from parts.
//
// For URL-form DSNs (postgres:// or postgresql://) this is a straight
// net/url round-trip: parse, swap the path, re-serialize. For keyword/value
// DSNs, cfg (already validated by the caller via pgx.ParseConfig) supplies
// the parts, but the DSN is still assembled through net/url so credentials
// get escaped correctly.
func withDatabase(admin string, cfg *pgx.ConnConfig, name string) (string, error) {
	if strings.HasPrefix(admin, "postgres://") || strings.HasPrefix(admin, "postgresql://") {
		u, err := url.Parse(admin)
		if err != nil {
			return "", fmt.Errorf("parse admin dsn as url: %w", err)
		}
		u.Path = "/" + name
		return u.String(), nil
	}

	sslmode := "disable"
	if cfg.TLSConfig != nil {
		sslmode = "require"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + name,
	}
	q := u.Query()
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
