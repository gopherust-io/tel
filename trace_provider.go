package tel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
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

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(shutdownCtx context.Context) error {
		var shutdownErr error

		err := provider.Shutdown(shutdownCtx)
		if err != nil {
			shutdownErr = err
		}

		err = exporter.Shutdown(shutdownCtx)

		if err != nil && shutdownErr == nil {
			shutdownErr = err
		}

		return shutdownErr
	}

	return provider, shutdown, nil
}

func otlpTraceExporterOptions(cfg TelConfig) ([]otlptracegrpc.Option, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Address),
	}

	if cfg.WithInsecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if cfg.WithCompression {
		opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
	}

	if len(cfg.Raw.CA) > 0 || len(cfg.Raw.Cert) > 0 || len(cfg.Raw.Key) > 0 {
		tlsCfg, err := tlsConfigFromRaw(cfg)
		if err != nil {
			return nil, err
		}

		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	}

	return opts, nil
}
