package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestNewRootReusesIncomingTraceID(t *testing.T) {
	const incoming = "4bf92f3577b34da6a3ce929d0e0e4736"
	ctx := traceContext{}.Extract(context.Background(), propagation.MapCarrier{
		"traceparent": "00-" + incoming + "-00f067aa0ba902b7-01",
	})
	tr := sdktrace.NewTracerProvider(sdktrace.WithIDGenerator(idGenerator{})).Tracer("t")

	_, root := tr.Start(ctx, "root", trace.WithNewRoot())
	if got := root.SpanContext().TraceID().String(); got != incoming {
		t.Fatalf("trace ID %s, want %s", got, incoming)
	}
	if root.(sdktrace.ReadOnlySpan).Parent().IsValid() {
		t.Fatal("new root has a parent")
	}

	_, other := tr.Start(context.Background(), "other")
	if !other.SpanContext().TraceID().IsValid() || other.SpanContext().TraceID().String() == incoming {
		t.Fatalf("no incoming context: got trace ID %s", other.SpanContext().TraceID())
	}
}
