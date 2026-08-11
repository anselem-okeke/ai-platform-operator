package logging

import (
	"fmt"
	"log/slog"
	"os"
)

func New(level string) (*slog.Logger, error) {
	var slogLevel slog.Level

	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf(
			"unsupported log level %q",
			level,
		)
	}

	handler := slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: slogLevel,
		},
	)

	return slog.New(handler), nil
}
