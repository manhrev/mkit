package log

import (
	"context"
	"errors"
	"log/slog"
)

// multiHandler fans a record out to multiple slog.Handlers (e.g. console +
// OTEL export). Not exported: build one via NewLogger's extra handlers.
type multiHandler struct {
	handlers []slog.Handler
}

// newMultiHandler drops nil handlers and skips wrapping entirely when only
// one handler remains, so the common (no extra handlers) case has no
// fan-out overhead.
func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	var live []slog.Handler
	for _, h := range handlers {
		if h != nil {
			live = append(live, h)
		}
	}
	if len(live) == 1 {
		return live[0]
	}
	return &multiHandler{handlers: live}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}
