package response

import "context"

// ctxKey is an unexported type so this package's context keys can never
// collide with keys from other packages using the same string value.
type ctxKey int

const metaContextKey ctxKey = iota

// WithMeta stores meta in ctx for later retrieval by MetaFromContext.
func WithMeta(ctx context.Context, meta Meta) context.Context {
	return context.WithValue(ctx, metaContextKey, meta)
}

// MetaFromContext retrieves the Meta stored by WithMeta, if any.
func MetaFromContext(ctx context.Context) (Meta, bool) {
	meta, ok := ctx.Value(metaContextKey).(Meta)
	return meta, ok
}
