package tel

import (
	"context"
	"crypto/tls"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// otlpDial holds shared OTLP gRPC dial settings for metric and trace exporters.
//
// goalign:ignore
type otlpDial struct {
	tlsConfig *tls.Config
	endpoint  string
	compress  bool
	insecure  bool
}

func otlpDialSettings(cfg TelConfig) (otlpDial, error) {
	useTLS := cfg.ServerName != "" || len(cfg.Raw.CA) > 0 || len(cfg.Raw.Cert) > 0 || len(cfg.Raw.Key) > 0
	dial := otlpDial{
		endpoint: cfg.Address,
		compress: cfg.WithCompression,
		insecure: cfg.WithInsecure && !useTLS,
	}
	if useTLS {
		tlsCfg, err := tlsConfigFromRaw(cfg)
		if err != nil {
			return otlpDial{}, err
		}
		dial.tlsConfig = tlsCfg
	}

	return dial, nil
}

type shutdowner interface {
	Shutdown(ctx context.Context) error
}

func shutdownProviderAndExporter(provider, exporter shutdowner) func(context.Context) error {
	return func(shutdownCtx context.Context) error {
		var shutdownErr error
		if err := provider.Shutdown(shutdownCtx); err != nil {
			shutdownErr = err
		}
		if err := exporter.Shutdown(shutdownCtx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}

		return shutdownErr
	}
}

func installPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}
