// Package obs provides slog setup, prometheus metrics, and health handlers.
package obs

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger builds a JSON slog.Logger writing to stdout at the given level
// ("debug", "info", "warn", "error"), tagged with the service name.
func NewLogger(level, service string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h).With("service", service)
}
