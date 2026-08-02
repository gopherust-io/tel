package tel

import (
	"time"
)

//go:generate envgen -type Config -output config_env_gen.go

// Config holds process-wide telemetry settings.
// Embedded configs stay first (embeddedstructfieldcheck); fieldalignment is
// excluded for this file in .golangci.yml and make align.
//
// goalign:ignore
type Config struct {
	MonitorConfig MonitorConfig
	TelConfig     TelConfig

	Service string `env:"TEL_SERVICE_NAME"`
	// Pod is the instance identity (K8s pod name). Falls back to HOSTNAME / os.Hostname.
	Pod                       string `env:"POD_NAME"`
	Namespace                 string `env:"NAMESPACE"                          default:"default"`
	Environment               string `env:"DEPLOY_ENVIRONMENT"                 default:"dev"`
	Version                   string `env:"VERSION"                            default:"dev"`
	LogLevel                  string `env:"LOG_LEVEL"                          default:"info"`
	LogEncode                 string `env:"LOG_ENCODE"                         default:"console"`
	MaxMessagesPerSecond      int    `env:"LOGS_MAX_MESSAGES_PER_SECOND"       default:"0"`
	MaxLevelMessagesPerSecond string `env:"LOGS_MAX_LEVEL_MESSAGES_PER_SECOND"`
	Debug                     bool   `env:"DEBUG"                              default:"false"`
}

// MonitorConfig holds health/stats monitor settings.
//
// goalign:ignore
type MonitorConfig struct {
	MonitorAddr string `env:"MONITOR_ADDR"   default:"127.0.0.1:8011"`
	Enable      bool   `env:"MONITOR_ENABLE" default:"true"`
}

// TelConfig holds OTLP export and related collector settings.
//
// goalign:ignore
type TelConfig struct {
	Address                    string         `env:"TEL_COLLECTOR_GRPC_ADDR"       default:"127.0.0.1:4317"`
	ServerName                 string         `env:"TEL_COLLECTOR_TLS_SERVER_NAME"`
	Raw                        TLSRawConfig   `env:"-"`
	BucketView                 []HistogramOpt `env:"-"` // runtime-only
	Metrics                    MetricsConfig  `env:"-"`
	MetricsPeriodicIntervalSec int            `env:"TEL_METRIC_PERIODIC_INTERVAL_SEC" default:"15"`
	ExportIntervalSec          int            `env:"TEL_EXPORT_INTERVAL_SEC"          default:"0"`
	WithInsecure               bool           `env:"TEL_EXPORTER_WITH_INSECURE"       default:"true"`
	Enable                     bool           `env:"TEL_ENABLE"                       default:"true"`
	WithCompression            bool           `env:"TEL_ENABLE_COMPRESSION"           default:"true"`
	Traces                     TracesConfig   `env:"-"`
}

// TLSRawConfig holds PEM material for collector mTLS.
//
// goalign:ignore
type TLSRawConfig struct {
	CA   []byte `env:"OTEL_COLLECTOR_TLS_CA_CERT"`
	Cert []byte `env:"OTEL_COLLECTOR_TLS_CLIENT_CERT"`
	Key  []byte `env:"OTEL_COLLECTOR_TLS_CLIENT_KEY"`
}

// MetricsConfig holds metric export / cardinality settings.
//
// goalign:ignore
type MetricsConfig struct {
	CardinalityDetector CardinalityDetectorConfig
	EnableRetry         bool `env:"METRICS_ENABLE_RETRY" default:"false"`
}

// CardinalityDetectorConfig limits metric label / instrument cardinality.
//
// goalign:ignore
type CardinalityDetectorConfig struct {
	MaxCardinality     int           `env:"METRICS_CARDINALITY_DETECTOR_MAX_CARDINALITY"     default:"100"`
	MaxInstruments     int           `env:"METRICS_CARDINALITY_DETECTOR_MAX_INSTRUMENTS"     default:"500"`
	DiagnosticInterval time.Duration `env:"METRICS_CARDINALITY_DETECTOR_DIAGNOSTIC_INTERVAL" default:"10m"`
	// WarnUtilizationPct logs when cache fill reaches this percent of MaxCardinality (0 disables).
	WarnUtilizationPct int  `env:"METRICS_CARDINALITY_WARN_UTILIZATION_PCT" default:"80"`
	Enable             bool `env:"METRICS_CARDINALITY_DETECTOR_ENABLE"       default:"true"`
	// DenyUnknown drops *With labels not registered via AttrCache.Allow / Telemetry.AllowSubjects.
	DenyUnknown bool `env:"METRICS_CARDINALITY_DENY_UNKNOWN" default:"false"`
}

// TracesConfig holds trace export settings.
//
// goalign:ignore
type TracesConfig struct {
	Enable  bool   `env:"TEL_TRACES_ENABLE"  default:"true"`
	Sampler string `env:"TEL_TRACES_SAMPLER" default:"parentbased_statustraceidratio:0.1"`
}

type HistogramOpt struct {
	Name       string
	Boundaries []float64
}
