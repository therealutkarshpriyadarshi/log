package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceName    = "logaggregator"
	serviceVersion = "0.2.0"
)

// Config holds tracing configuration
type Config struct {
	Enabled      bool
	Endpoint     string
	SampleRate   float64
	EnableStdout bool
}

// Provider wraps the OpenTelemetry tracer provider
type Provider struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

// NewProvider creates a new tracing provider
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		// Return a no-op provider
		return &Provider{
			tracer: otel.Tracer(serviceName),
		}, nil
	}

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create OTLP exporter
	var exporter *otlptrace.Exporter
	if cfg.Endpoint != "" {
		client := otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
			otlptracegrpc.WithInsecure(), // Use TLS in production
		)
		exporter, err = otlptrace.New(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	}

	// Configure sampler
	sampler := sdktrace.AlwaysSample()
	if cfg.SampleRate > 0 && cfg.SampleRate < 1 {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create tracer provider
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
	}

	if exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	tp := sdktrace.NewTracerProvider(opts...)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(serviceName),
	}, nil
}

// Tracer returns the tracer
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Shutdown shuts down the tracer provider
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp != nil {
		return p.tp.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span
func (p *Provider) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, name, opts...)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
}

// Helper functions for common operations

// TraceInput creates a span for input operations
func TraceInput(ctx context.Context, tracer trace.Tracer, inputName, inputType string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "input.receive",
		trace.WithAttributes(
			attribute.String("input.name", inputName),
			attribute.String("input.type", inputType),
		),
	)
}

// TraceParser creates a span for parsing operations
func TraceParser(ctx context.Context, tracer trace.Tracer, parserType string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "parser.parse",
		trace.WithAttributes(
			attribute.String("parser.type", parserType),
		),
	)
}

// TraceOutput creates a span for output operations
func TraceOutput(ctx context.Context, tracer trace.Tracer, outputName, outputType string, eventCount int) (context.Context, trace.Span) {
	return tracer.Start(ctx, "output.send",
		trace.WithAttributes(
			attribute.String("output.name", outputName),
			attribute.String("output.type", outputType),
			attribute.Int("event.count", eventCount),
		),
	)
}

// TraceWAL creates a span for WAL operations
func TraceWAL(ctx context.Context, tracer trace.Tracer, operation string) (context.Context, trace.Span) {
	return tracer.Start(ctx, fmt.Sprintf("wal.%s", operation))
}

// TraceBuffer creates a span for buffer operations
func TraceBuffer(ctx context.Context, tracer trace.Tracer, operation string) (context.Context, trace.Span) {
	return tracer.Start(ctx, fmt.Sprintf("buffer.%s", operation))
}
