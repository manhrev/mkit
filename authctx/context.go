// Package authctx carries the authenticated user ID through a request context.
package authctx

import "context"

type userIDKey struct{}

// WithUserID stores the authenticated user ID in ctx.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserIDFrom returns the authenticated user ID, or an empty string on public requests.
func UserIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey{}).(string)
	return id
}
