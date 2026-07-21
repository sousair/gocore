// Package env reads environment variables into typed values with a default
// fallback, so callers don't repeat os.LookupEnv + parse + error-handling.
package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// GetEnvAs reads the environment variable key and parses it into T. It returns
// defaultValue when the variable is unset, empty, or fails to parse, so a caller
// always gets a usable value. Supported T: string, bool, int, int64, float64,
// and time.Duration; any other type returns defaultValue unparsed.
func GetEnvAs[T any](key string, defaultValue T) T {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return defaultValue
	}

	var parsed any
	switch any(defaultValue).(type) {
	case string:
		parsed = raw
	case bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return defaultValue
		}
		parsed = v
	case int:
		v, err := strconv.Atoi(raw)
		if err != nil {
			return defaultValue
		}
		parsed = v
	case int64:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return defaultValue
		}
		parsed = v
	case float64:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return defaultValue
		}
		parsed = v
	case time.Duration:
		v, err := time.ParseDuration(raw)
		if err != nil {
			return defaultValue
		}
		parsed = v
	default:
		return defaultValue
	}

	return parsed.(T)
}

// SecretString resolves a plaintext secret from either a mounted file or an
// inline env var, preferring the file when fileEnvVar is set. Unlike
// NewKeyWrapper, an unset pair is not an error: callers treat "" as the
// feature being disabled.
func SecretString(fileEnvVar, valueEnvVar string) (string, error) {
	path := os.Getenv(fileEnvVar)
	if path == "" {
		return os.Getenv(valueEnvVar), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("env: read secret file %s: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", fmt.Errorf("env: secret file %s is empty", path)
	}
	return trimmed, nil
}
