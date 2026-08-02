package tel

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gopherust-io/env"
)

const defaultDotEnvPath = ".env"

// GetConfigFromEnv loads `.env` (if present), then parses Config from the process
// environment. Empty TEL_SERVICE_NAME falls back to a hostname-derived service name.
func GetConfigFromEnv() (Config, error) {
	if err := loadDotEnv(); err != nil {
		return Config{}, err
	}
	// LoadDotEnv mutates process env; v0.6+ LoadConfig reads Snapshot without Reload.
	env.Reload()
	cfg, err := LoadConfig()
	if err != nil {
		return Config{}, err
	}
	applyNestedEnv(&cfg)
	if strings.TrimSpace(cfg.Service) == "" {
		host, _ := os.Hostname()
		cfg.Service = strings.ToLower(strings.ReplaceAll(host, "-", "_"))
		if cfg.Service == "" {
			cfg.Service = defaultServiceUnknown
		}
	}
	if strings.TrimSpace(cfg.Pod) == "" {
		cfg.Pod = resolvePod(cfg)
	}

	return cfg, nil
}

func loadDotEnv() error {
	path := strings.TrimSpace(os.Getenv("TEL_DOTENV"))
	if path == "" {
		path = defaultDotEnvPath
	}
	err := env.LoadDotEnv(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// applyNestedEnv fills Metrics / Traces / TLS raw fields (envgen skips nested structs without prefix).
func applyNestedEnv(cfg *Config) {
	cfg.TelConfig.Traces.Enable = envBool("TEL_TRACES_ENABLE", true)
	cfg.TelConfig.Traces.Sampler = envString("TEL_TRACES_SAMPLER", defaultTracesSampler)

	cd := &cfg.TelConfig.Metrics.CardinalityDetector
	cd.MaxCardinality = envInt("METRICS_CARDINALITY_DETECTOR_MAX_CARDINALITY", defaultMaxCardinality)
	cd.MaxInstruments = envInt("METRICS_CARDINALITY_DETECTOR_MAX_INSTRUMENTS", defaultMaxInstruments)
	cd.DiagnosticInterval = envDuration("METRICS_CARDINALITY_DETECTOR_DIAGNOSTIC_INTERVAL", defaultDiagnosticInterval)
	cd.WarnUtilizationPct = envInt("METRICS_CARDINALITY_WARN_UTILIZATION_PCT", defaultWarnUtilizationPct)
	cd.Enable = envBool("METRICS_CARDINALITY_DETECTOR_ENABLE", true)
	cd.DenyUnknown = envBool("METRICS_CARDINALITY_DENY_UNKNOWN", false)
	cfg.TelConfig.Metrics.EnableRetry = envBool("METRICS_ENABLE_RETRY", false)

	if v := os.Getenv("OTEL_COLLECTOR_TLS_CA_CERT"); v != "" {
		cfg.TelConfig.Raw.CA = []byte(v)
	}
	if v := os.Getenv("OTEL_COLLECTOR_TLS_CLIENT_CERT"); v != "" {
		cfg.TelConfig.Raw.Cert = []byte(v)
	}
	if v := os.Getenv("OTEL_COLLECTOR_TLS_CLIENT_KEY"); v != "" {
		cfg.TelConfig.Raw.Key = []byte(v)
	}
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}

	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}

	return b
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}

	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}

	return d
}

// Init loads .env + env config, configures the logger, creates telemetry, sets
// the process global, and starts exporters. On failure it Fatals (does not return an error).
// The returned shutdown flushes exporters; call it on process exit (e.g. defer).
func Init(ctx context.Context) (*Telemetry, func(context.Context) error) {
	cfg, err := GetConfigFromEnv()
	if err != nil {
		Fatal().Err(err).Msg("tel: load config")
	}
	t, shutdown, err := InitWithConfig(ctx, cfg)
	if err != nil {
		Fatal().Err(err).Msg("tel: start")
	}

	return t, shutdown
}

// InitWithConfig is like Init but uses an explicit Config (tests / custom setup).
// It does not load .env. The returned shutdown flushes exporters.
func InitWithConfig(ctx context.Context, cfg Config) (*Telemetry, func(context.Context) error, error) {
	ConfigureLogger(cfg)
	t := NewWithConfig(cfg)
	SetGlobal(t)
	if err := t.Start(ctx); err != nil {
		return nil, nil, err
	}

	return t, t.Shutdown, nil
}
