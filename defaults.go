package tel

import "time"

// Default configuration values. envDefault struct tags in model.go must stay in sync.
const (
	defaultServiceUnknown = "unknown"
	defaultVersion        = "dev"
	defaultNamespace      = "default"
	defaultEnvironment    = "dev"
	defaultLogEncode      = "json"
	defaultLogLevel       = "info"
	defaultDebugLogEncode = "console"
	defaultDebugLogLevel  = "debug"

	defaultMonitorAddr       = "0.0.0.0:8011"
	defaultOTLPGRPCAddr      = "127.0.0.1:4317"
	defaultReadHeaderTimeout = 5 * time.Second

	defaultMetricsPeriodicIntervalSec = 15
	defaultMetricsPeriodicInterval    = defaultMetricsPeriodicIntervalSec * time.Second

	defaultMaxCardinality     = 100
	defaultMaxInstruments     = 500
	defaultDiagnosticInterval = 10 * time.Minute
)
