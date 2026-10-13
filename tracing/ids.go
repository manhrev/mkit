package tracing

import (
	"context"
	"crypto/rand"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// traceContext is the W3C propagator that also remembers the incoming
// trace ID, so idGenerator can reuse it for a new root span.
type traceContext struct{ propagation.TraceContext }

func (p traceContext) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	ctx = p.TraceContext.Extract(ctx, carrier)
	if sc := trace.SpanContextFromContext(ctx); sc.IsRemote() {
		ctx = context.WithValue(ctx, remoteTraceIDKey, sc.TraceID())
	}
	return ctx
}

// idGenerator gives a new root span (e.g. otelhttp with a public endpoint)
// the incoming trace ID when there is one. The trace then shares its ID
// with the caller's (Cloud Run's request log) yet has its own root span,
// so the backend never waits for a parent it will not receive.
type idGenerator struct{}

func (idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	tid, ok := ctx.Value(remoteTraceIDKey).(trace.TraceID)
	if !ok {
		_, _ = rand.Read(tid[:])
	}
	return tid, newSpanID()
}

func (idGenerator) NewSpanID(context.Context, trace.TraceID) trace.SpanID {
	return newSpanID()
}

func newSpanID() (sid trace.SpanID) {
	_, _ = rand.Read(sid[:])
	return sid
}
