package response

import (
	"context"
	"time"
)

// Output is the common huma operation Output shape: any operation that
// returns a body uses *Output[T] directly (e.g. huma.Register's Output
// type param) instead of a bespoke per-operation output struct.
type Output[T any] struct {
	Body Response[T]
}

// Response is the generic envelope every success response body uses.
type Response[T any] struct {
	Meta Meta `json:"meta"`
	Data T    `json:"data"`
}

// NewOutput wraps data in the common Output envelope. Meta is read back out
// of ctx (stamped by middleware.Metadata earlier in the request); ResponseAt
// is set to now. A ctx with no stamped Meta (e.g. in tests) just yields a
// zero Meta plus ResponseAt, same as before this existed.
func NewOutput[T any](ctx context.Context, data T) *Output[T] {
	meta, _ := MetaFromContext(ctx)
	meta.ResponseAt = time.Now()
	return &Output[T]{Body: Response[T]{Meta: meta, Data: data}}
}
