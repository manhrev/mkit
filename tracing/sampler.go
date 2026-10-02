package tracing

import sdktrace "go.opentelemetry.io/otel/sdk/trace"

type skipSampler struct{}

func (s skipSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	if v, ok := p.ParentContext.Value(skipTraceContextKey).(bool); ok && v {
		return sdktrace.SamplingResult{Decision: sdktrace.Drop}
	}
	return sdktrace.SamplingResult{Decision: sdktrace.RecordAndSample}
}

func (s skipSampler) Description() string { return "SkipSampler" }
