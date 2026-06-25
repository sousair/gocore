package env_test

import (
	"testing"
	"time"

	"github.com/sousair/gocore/pkg/env"
)

func TestGetEnvAs(t *testing.T) {
	t.Run("given unset key/when read/then default", func(t *testing.T) {
		if got := env.GetEnvAs("GOCORE_ENV_UNSET", 42); got != 42 {
			t.Fatalf("want 42, got %d", got)
		}
	})

	t.Run("given empty value/when read/then default", func(t *testing.T) {
		t.Setenv("GOCORE_ENV_EMPTY", "")
		if got := env.GetEnvAs("GOCORE_ENV_EMPTY", "fallback"); got != "fallback" {
			t.Fatalf("want fallback, got %q", got)
		}
	})

	t.Run("given valid values/when read/then parsed per type", func(t *testing.T) {
		t.Setenv("GOCORE_S", "hello")
		t.Setenv("GOCORE_B", "true")
		t.Setenv("GOCORE_I", "7")
		t.Setenv("GOCORE_I64", "9000000000")
		t.Setenv("GOCORE_F", "1.5")
		t.Setenv("GOCORE_D", "30s")
		if got := env.GetEnvAs("GOCORE_S", "x"); got != "hello" {
			t.Errorf("string: got %q", got)
		}
		if got := env.GetEnvAs("GOCORE_B", false); !got {
			t.Errorf("bool: got %v", got)
		}
		if got := env.GetEnvAs("GOCORE_I", 0); got != 7 {
			t.Errorf("int: got %d", got)
		}
		if got := env.GetEnvAs("GOCORE_I64", int64(0)); got != int64(9000000000) {
			t.Errorf("int64: got %d", got)
		}
		if got := env.GetEnvAs("GOCORE_F", 0.0); got != 1.5 {
			t.Errorf("float64: got %v", got)
		}
		if got := env.GetEnvAs("GOCORE_D", time.Second); got != 30*time.Second {
			t.Errorf("duration: got %v", got)
		}
	})

	t.Run("given unparseable value/when read/then default", func(t *testing.T) {
		t.Setenv("GOCORE_BADINT", "notanint")
		if got := env.GetEnvAs("GOCORE_BADINT", 99); got != 99 {
			t.Fatalf("want 99, got %d", got)
		}
	})
}
