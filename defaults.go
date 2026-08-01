package tel

import "time"

// Default configuration values. envDefault struct tags in model.go must stay in sync.
const (
	defaultServiceUnknown = "unknown"
	defaultVersion        = "dev"
	defaultNamespace      = "default"
	defaultEnvironment    = "dev"
	defaultLogEncode      = "console"
	defaultLogLevel       = "info"
	defaultDebugLogEncode = "console"
	defaultDebugLogLevel  = "debug"

	defaultMonitorAddr         = "127.0.0.1:8011"
	defaultOTLPGRPCAddr        = "127.0.0.1:4317"
	defaultReadHeaderTimeout   = 5 * time.Second
	defaultMonitorWriteTimeout = 5 * time.Second
	defaultMonitorIdleTimeout  = 60 * time.Second

	defaultMetricsPeriodicIntervalSec = 15
	defaultMetricsPeriodicInterval    = defaultMetricsPeriodicIntervalSec * time.Second

	defaultMaxCardinality     = 100
	defaultMaxInstruments     = 500
	defaultDiagnosticInterval = 10 * time.Minute

	defaultTracesSampler      = "parentbased_statustraceidratio:0.1"
	defaultDebugTracesSampler = "always"
)
