package tel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc/credentials"
)

func newMeterProvider(ctx context.Context, cfg Config) (metric.MeterProvider, func(context.Context) error, error) {
	opts, err := otlpExporterOptions(cfg.TelConfig)
	if err != nil {
		return nil, nil, err
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create otlp metric exporter: %w", err)
	}

	interval := time.Duration(cfg.MetricsPeriodicIntervalSec) * time.Second
	if cfg.ExportIntervalSec > 0 {
		interval = time.Duration(cfg.ExportIntervalSec) * time.Second
	}
	if interval <= 0 {
		interval = defaultMetricsPeriodicInterval
	}

	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval))

	res := newResource(cfg)

	providerOpts := []sdkmetric.Option{
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	}
	for _, view := range viewsFromBucketView(cfg.BucketView) {
		providerOpts = append(providerOpts, sdkmetric.WithView(view))
	}

	provider := sdkmetric.NewMeterProvider(providerOpts...)
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

func otlpExporterOptions(cfg TelConfig) ([]otlpmetricgrpc.Option, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Address),
	}

	if cfg.WithCompression {
		opts = append(opts, otlpmetricgrpc.WithCompressor("gzip"))
	}

	useTLS := cfg.ServerName != "" || len(cfg.Raw.CA) > 0 || len(cfg.Raw.Cert) > 0 || len(cfg.Raw.Key) > 0
	if cfg.WithInsecure && !useTLS {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	if useTLS {
		tlsCfg, err := tlsConfigFromRaw(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	}

	return opts, nil
}

func tlsConfigFromRaw(cfg TelConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if cfg.ServerName != "" {
		tlsCfg.ServerName = cfg.ServerName
	}

	if len(cfg.Raw.CA) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.Raw.CA) {
			return nil, fmt.Errorf("append CA cert from PEM")
		}

		tlsCfg.RootCAs = pool
	}

	if len(cfg.Raw.Cert) > 0 && len(cfg.Raw.Key) > 0 {
		cert, err := tls.X509KeyPair(cfg.Raw.Cert, cfg.Raw.Key)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}

		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

func newResource(cfg Config) *resource.Resource {
	service := cfg.Service
	if service == "" {
		service = defaultServiceUnknown
	}

	version := cfg.Version
	if version == "" {
		version = defaultVersion
	}

	namespace := cfg.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}

	environment := cfg.Environment
	if environment == "" {
		environment = defaultEnvironment
	}

	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(service),
		semconv.ServiceVersion(version),
		semconv.ServiceNamespace(namespace),
		semconv.DeploymentEnvironment(environment),
	)
}

func viewsFromBucketView(buckets []HistogramOpt) []sdkmetric.View {
	if len(buckets) == 0 {
		return nil
	}

	views := make([]sdkmetric.View, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.Name == "" || len(bucket.Boundaries) == 0 {
			continue
		}

		name := bucket.Name
		boundaries := bucket.Boundaries
		views = append(views, sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: boundaries,
			}},
		))
	}

	return views
}
