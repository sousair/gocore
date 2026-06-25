// Package env reads environment variables into typed values with a default
// fallback, so callers don't repeat os.LookupEnv + parse + error-handling.
package env

import (
	"os"
	"strconv"
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
