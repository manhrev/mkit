package tracing

import (
	"context"
	"fmt"
	"github.com/manhrev/mkit/config"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otellog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Service struct {
	serviceName    string
	tracerProvider *sdktrace.TracerProvider
	loggerProvider *otellog.LoggerProvider
	meterProvider  *sdkmetric.MeterProvider
	resources      *resource.Resource
}

func NewService(ctx context.Context, appCfg *config.App) (*Service, error) {
	var (
		s   = &Service{}
		cfg = appCfg.Tracing
	)

	if !cfg.Enabled {
		return s, nil
	}

	var (
		serviceName = cfg.ServiceName
		err         error
	)

	s.serviceName = serviceName

	s.resources, err = resource.New(
		ctx,
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("library.language", "go"),
		),
	)
	if err != nil {
		return nil, err
	}

	if cfg.Trace {
		s.tracerProvider, err = initTracer(ctx, cfg, s.resources)
		if err != nil {
			return nil, fmt.Errorf("cant init tracer: %w", err)
		}

		// Required for trace context (traceparent) to propagate across
		// service boundaries via HTTP/gRPC headers — without this, spans on
		// either side of a network call are never linked into the same trace.
		otel.SetTextMapPropagator(traceContext{})
	}

	if cfg.Metric {
		s.meterProvider, err = initMetrics(ctx, cfg, s.resources)
		if err != nil {
			return nil, fmt.Errorf("cant init metric: %w", err)
		}
	}

	if cfg.Log {
		s.loggerProvider, err = initLogCollector(ctx, cfg, s.resources)
		if err != nil {
			return nil, fmt.Errorf("cant init log collector: %w", err)
		}
	}

	return s, nil
}

// Stop shuts down the providers (not the raw exporters directly) so any
// buffered spans/logs/metrics sitting in their batch processors get
// flushed before the underlying exporter connection closes.
func (s *Service) Stop(ctx context.Context) error {
	if s.tracerProvider != nil {
		if err := s.tracerProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown tracer provider: %w", err)
		}
	}
	if s.meterProvider != nil {
		if err := s.meterProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown meter provider: %w", err)
		}
	}
	if s.loggerProvider != nil {
		if err := s.loggerProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown logger provider: %w", err)
		}
	}

	return nil
}

// All signals export over OTLP/gRPC to cfg.Endpoint; its scheme picks TLS
// (https) or plaintext (http).
func initTracer(
	ctx context.Context, cfg config.Tracing, resources *resource.Resource,
) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpointURL(cfg.Endpoint),
			otlptracegrpc.WithHeaders(cfg.Headers),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(skipSampler{}),
		sdktrace.WithIDGenerator(idGenerator{}),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resources),
	)
	otel.SetTracerProvider(tp)

	return tp, nil
}

func initLogCollector(
	ctx context.Context, cfg config.Tracing, resources *resource.Resource,
) (*otellog.LoggerProvider, error) {
	logExporter, err := otlploggrpc.New(
		ctx,
		otlploggrpc.WithEndpointURL(cfg.Endpoint),
		otlploggrpc.WithHeaders(cfg.Headers),
	)
	if err != nil {
		return nil, err
	}

	return otellog.NewLoggerProvider(
		otellog.WithResource(resources),
		otellog.WithProcessor(otellog.NewBatchProcessor(logExporter)),
	), nil
}

// Handler returns an slog.Handler that forwards records to the OTEL log
// collector, for use as one leg of a fan-out with the app's normal console
// handler (see pkg/log). Returns nil if the log signal isn't enabled —
// callers should skip adding it to the fan-out in that case.
func (s *Service) Handler() slog.Handler {
	if s.loggerProvider == nil {
		return nil
	}
	return otelslog.NewHandler(s.serviceName, otelslog.WithLoggerProvider(s.loggerProvider))
}

func initMetrics(ctx context.Context, cfg config.Tracing, resources *resource.Resource) (*sdkmetric.MeterProvider, error) {
	metricExporter, err := otlpmetricgrpc.New(
		ctx,
		otlpmetricgrpc.WithEndpointURL(cfg.Endpoint),
		otlpmetricgrpc.WithHeaders(cfg.Headers),
	)
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(resources),
	)
	otel.SetMeterProvider(meterProvider)

	return meterProvider, nil
}
