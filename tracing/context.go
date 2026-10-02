package tracing

import "context"

// ctxKey is an unexported type so this package's context keys can never
// collide with keys from other packages using the same string value.
type ctxKey int

const skipTraceContextKey ctxKey = iota

func WithSkipTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipTraceContextKey, true)
}
