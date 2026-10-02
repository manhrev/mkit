package log

import (
	"context"
	"log/slog"
)

// ctxKey is an unexported type so this package's context keys can never
// collide with keys from other packages using the same string value.
type ctxKey int

const loggerContextKey ctxKey = iota

// WithLogger stores logger in ctx for later retrieval by FromContext.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// FromContext retrieves the logger stored by WithLogger, falling back to
// slog.Default() if ctx has none (e.g. a context that never passed through
// the request middleware chain).
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
