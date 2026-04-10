package log

import (
	"io"
	stdlog "log"
	"log/slog"
	"os"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// New creates the daemon logger.
func New(path string, debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	writer := io.Writer(&lumberjack.Logger{Filename: path, MaxSize: 10, MaxBackups: 5, MaxAge: 14})
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	stdlog.SetOutput(writer)
	return logger
}

// With returns a child logger.
func With(logger *slog.Logger, args ...any) *slog.Logger {
	if logger == nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)).With(args...)
	}
	return logger.With(args...)
}
