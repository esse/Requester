package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Setup configures the default structured logger with the given level.
// Valid levels: "debug", "info", "warn", "error". Defaults to "info".
func Setup(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
