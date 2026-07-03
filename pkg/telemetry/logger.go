package telemetry

import "log/slog"

// Err is the one sanctioned way to attach an error to a log record. The key
// is "error" (never "err") so LogQL queries have a single name to target.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "<nil>")
	}
	return slog.String("error", err.Error())
}
