package tel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"
)

func newTracerProvider(ctx context.Context, cfg Config) (trace.TracerProvider, func(context.Context) error, error) {
	if !cfg.Traces.Enable {
		return noop.NewTracerProvider(), func(context.Context) error { return nil }, nil
	}

	opts, err := otlpTraceExporterOptions(cfg.TelConfig)
	if err != nil {
		return nil, nil, err
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res := newResource(cfg)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	installPropagator()

	return provider, shutdownProviderAndExporter(provider, exporter), nil
}

func otlpTraceExporterOptions(cfg TelConfig) ([]otlptracegrpc.Option, error) {
	dial, err := otlpDialSettings(cfg)
	if err != nil {
		return nil, err
	}
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(dial.endpoint),
	}
	if dial.compress {
		opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
	}
	if dial.insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	if dial.tlsConfig != nil {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(dial.tlsConfig)))
	}

	return opts, nil
}
