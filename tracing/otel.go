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
	"go.opentelemetry.io/otel/propagation"
	otellog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
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
		collectorUrl = fmt.Sprintf("%s:%d", cfg.CollectorHost, cfg.CollectorPort)
		serviceName  = cfg.ServiceName
		err          error
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
		s.tracerProvider, err = initTracer(ctx, collectorUrl, s.resources, cfg.Secure)
		if err != nil {
			return nil, fmt.Errorf("cant init tracer: %w", err)
		}

		// Required for trace context (traceparent) to propagate across
		// service boundaries via HTTP/gRPC headers — without this, spans on
		// either side of a network call are never linked into the same trace.
		otel.SetTextMapPropagator(propagation.TraceContext{})
	}

	if cfg.Metric {
		s.meterProvider, err = initMetrics(ctx, collectorUrl, s.resources, cfg.Secure)
		if err != nil {
			return nil, fmt.Errorf("cant init metric: %w", err)
		}
	}

	if cfg.Log {
		s.loggerProvider, err = initLogCollector(ctx, collectorUrl, s.resources, cfg.Secure)
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

func secureOption(isSecure bool) otlptracegrpc.Option {
	if isSecure {
		return otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, ""))
	}
	return otlptracegrpc.WithInsecure()
}

func initTracer(
	ctx context.Context, collectorURL string, resources *resource.Resource, isSecure bool,
) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			secureOption(isSecure),
			otlptracegrpc.WithEndpoint(collectorURL),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(skipSampler{}),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resources),
	)
	otel.SetTracerProvider(tp)

	return tp, nil
}

func initLogCollector(
	ctx context.Context, collectorURL string, resources *resource.Resource, isSecure bool,
) (*otellog.LoggerProvider, error) {
	var opt otlploggrpc.Option
	if isSecure {
		opt = otlploggrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, ""))
	} else {
		opt = otlploggrpc.WithInsecure()
	}

	logExporter, err := otlploggrpc.New(
		ctx,
		otlploggrpc.WithEndpoint(collectorURL),
		opt,
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

func initMetrics(ctx context.Context, collectorURL string, resources *resource.Resource, isSecure bool) (*sdkmetric.MeterProvider, error) {
	var opt otlpmetricgrpc.Option
	if isSecure {
		opt = otlpmetricgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, ""))
	} else {
		opt = otlpmetricgrpc.WithInsecure()
	}

	metricExporter, err := otlpmetricgrpc.New(
		ctx,
		otlpmetricgrpc.WithEndpoint(collectorURL),
		opt,
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
