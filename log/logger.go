// Package log builds the app's slog.Logger: human-readable text when
// attached to a terminal, JSON otherwise (files, pipes, prod log collectors).
package log

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/lmittmann/tint"

	"github.com/manhrev/mkit/config"
)

// NewLogger builds a *slog.Logger from app config: level from cfg.Log.Level,
// format auto-detected from whether stdout is a terminal. Any extra handlers
// (e.g. tracing.Service.Handler() for OTEL log export) are fanned out to
// alongside the console handler; nil entries are ignored, so callers can
// pass a possibly-nil handler unconditionally.
func NewLogger(cfg *config.App, extra ...slog.Handler) *slog.Logger {
	handlers := append([]slog.Handler{newHandler(os.Stdout, parseLevel(cfg.Log.Level))}, extra...)
	return slog.New(newMultiHandler(handlers...))
}

// Bootstrap is a plain stderr text logger, for errors that happen before
// the real logger (built by NewLogger, which itself depends on config/
// tracing having initialized successfully) is available.
func Bootstrap() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newHandler(w io.Writer, level slog.Level) slog.Handler {
	if isTerminal(w) {
		// tint: same key=value shape as slog's TextHandler, but colorized
		// (level + keys) so key=value pairs are easy to pick out at a glance.
		return tint.NewTextHandler(w, &tint.Options{Level: level, TimeFormat: "15:04:05"})
	}
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
