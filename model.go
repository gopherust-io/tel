package tel

import (
	"time"
)

// Config holds process-wide telemetry settings.
// Embedded configs stay first (embeddedstructfieldcheck); fieldalignment is
// excluded for this file in .golangci.yml and make align.
type Config struct {
	MonitorConfig
	TelConfig

	Service     string `env:"TEL_SERVICE_NAME"`
	Namespace   string `env:"NAMESPACE"          envDefault:"default"`
	Environment string `env:"DEPLOY_ENVIRONMENT" envDefault:"dev"`
	Version     string `env:"VERSION"            envDefault:"dev"`
	LogLevel    string `env:"LOG_LEVEL"          envDefault:"info"`
	LogEncode   string `env:"LOG_ENCODE"         envDefault:"json"`
	Debug       bool   `env:"DEBUG"              envDefault:"false"`
}

type MonitorConfig struct {
	MonitorAddr string `env:"MONITOR_ADDR"   envDefault:"0.0.0.0:8011"`
	Enable      bool   `env:"MONITOR_ENABLE" envDefault:"true"`
}

type TelConfig struct {
	Address    string `env:"TEL_COLLECTOR_GRPC_ADDR"       envDefault:"127.0.0.1:4317"`
	ServerName string `env:"TEL_COLLECTOR_TLS_SERVER_NAME"`
	Raw        struct {
		CA   []byte `env:"OTEL_COLLECTOR_TLS_CA_CERT"`
		Cert []byte `env:"OTEL_COLLECTOR_TLS_CLIENT_CERT"`
		Key  []byte `env:"OTEL_COLLECTOR_TLS_CLIENT_KEY"`
	}

	BucketView []HistogramOpt

	Metrics struct {
		CardinalityDetector struct {
			MaxCardinality     int           `env:"METRICS_CARDINALITY_DETECTOR_MAX_CARDINALITY"     envDefault:"100"`
			MaxInstruments     int           `env:"METRICS_CARDINALITY_DETECTOR_MAX_INSTRUMENTS"     envDefault:"500"`
			DiagnosticInterval time.Duration `env:"METRICS_CARDINALITY_DETECTOR_DIAGNOSTIC_INTERVAL" envDefault:"10m"`
			Enable             bool          `env:"METRICS_CARDINALITY_DETECTOR_ENABLE"              envDefault:"true"`
		}
		EnableRetry bool `env:"METRICS_ENABLE_RETRY" envDefault:"false"`
	}
	MetricsPeriodicIntervalSec int `env:"TEL_METRIC_PERIODIC_INTERVAL_SEC" envDefault:"15"`
	// ExportIntervalSec overrides MetricsPeriodicIntervalSec when > 0 (OTLP push cadence).
	ExportIntervalSec int  `env:"TEL_EXPORT_INTERVAL_SEC" envDefault:"0"`
	WithInsecure      bool `env:"TEL_EXPORTER_WITH_INSECURE"       envDefault:"true"`
	Enable            bool `env:"TEL_ENABLE"                       envDefault:"true"`
	WithCompression   bool `env:"TEL_ENABLE_COMPRESSION"           envDefault:"true"`
	Traces            struct {
		Enable bool `env:"TEL_TRACES_ENABLE" envDefault:"true"`
	}
}

type HistogramOpt struct {
	Name       string
	Boundaries []float64
}
