package tel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

var globalTelemetry atomic.Pointer[Telemetry]

func init() {
	globalTelemetry.Store(NewWithConfig(DefaultDebugConfig()))
}

func Global() *Telemetry {
	return globalTelemetry.Load()
}

func SetGlobal(t *Telemetry) {
	globalTelemetry.Store(t)
}

// Telemetry is the process-wide metrics/traces runtime.
// Telemetry is the process-wide metrics/traces runtime.
//
// goalign:ignore
type Telemetry struct {
	metricProvider metric.MeterProvider
	traceProvider  trace.TracerProvider
	tracer         trace.Tracer
	registry       *Registry
	cardinality    *cardinalityDetector
	monitor        *monitorServer
	shutdownFn     func(context.Context) error
	traceShutdown  func(context.Context) error
	cfg            Config
	epoch          atomic.Uint64
	mu             sync.RWMutex
	started        atomic.Bool
	traceInstalled bool
}

type tKey struct{}

func New() *Telemetry {
	return NewWithConfig(DefaultDebugConfig())
}

func NewWithConfig(cfg Config) *Telemetry {
	service := cfg.Service
	if service == "" {
		service = defaultServiceUnknown
	}

	provider := noop.NewMeterProvider()

	tel := &Telemetry{
		cfg:            cfg,
		metricProvider: provider,
		traceProvider:  tracenoop.NewTracerProvider(),
	}
	tel.epoch.Store(0)
	tel.registry = newRegistryWithCache(
		provider.Meter(service),
		newAttrCache(defaultMaxCardinality),
		&tel.epoch,
		0,
		maxInstrumentsFromCfg(cfg),
	)
	tel.refreshTracerLocked()

	return tel
}

func maxInstrumentsFromCfg(cfg Config) int {
	n := cfg.Metrics.CardinalityDetector.MaxInstruments
	if n <= 0 {
		return defaultMaxInstruments
	}

	return n
}

func DefaultConfig() Config {
	host, _ := os.Hostname()
	host = strings.ToLower(strings.ReplaceAll(host, "-", "_"))

	return Config{
		Service:     host,
		Version:     defaultVersion,
		Namespace:   defaultNamespace,
		Environment: defaultEnvironment,
		LogEncode:   defaultLogEncode,
		LogLevel:    defaultLogLevel,
		MonitorConfig: MonitorConfig{
			Enable:      true,
			MonitorAddr: defaultMonitorAddr,
		},
		TelConfig: TelConfig{
			Address:                    defaultOTLPGRPCAddr,
			WithInsecure:               true,
			Enable:                     true,
			WithCompression:            true,
			MetricsPeriodicIntervalSec: defaultMetricsPeriodicIntervalSec,
		},
	}
}

func DefaultDebugConfig() Config {
	c := DefaultConfig()
	c.Debug = true
	c.LogLevel = defaultDebugLogLevel
	c.LogEncode = defaultDebugLogEncode
	c.MonitorConfig.Enable = false
	c.TelConfig.Enable = false

	return c
}

func WrapContext(ctx context.Context, l *Telemetry) context.Context {
	return context.WithValue(ctx, tKey{}, l)
}

func FromCtx(ctx context.Context) *Telemetry {
	if t, ok := ctx.Value(tKey{}).(*Telemetry); ok {
		return t
	}

	return Global()
}

func (t *Telemetry) copy() *Telemetry {
	return NewWithConfig(t.cfg)
}

func (t *Telemetry) Config() Config {
	return t.cfg
}

func (t *Telemetry) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.started.Load() {
		return nil
	}

	// Always install propagators so Inject/Extract work even when traces export is off.
	installPropagator()

	if t.cfg.TelConfig.Enable {
		if err := t.startOTelLocked(ctx); err != nil {
			return err
		}
	} else {
		t.metricProvider = noop.NewMeterProvider()
		t.traceProvider = tracenoop.NewTracerProvider()
		t.refreshTracerLocked()
	}

	maxCardinality := t.cfg.Metrics.CardinalityDetector.MaxCardinality
	if maxCardinality <= 0 {
		maxCardinality = defaultMaxCardinality
	}

	// Bump epoch so pre-Start instruments no-op; callers must re-fetch after Start.
	epoch := t.epoch.Add(1)
	attrCache := newAttrCache(maxCardinality)
	t.registry = newRegistryWithCache(
		t.metricProvider.Meter(t.cfg.Service),
		attrCache,
		&t.epoch,
		epoch,
		maxInstrumentsFromCfg(t.cfg),
	)

	if t.cfg.Metrics.CardinalityDetector.Enable {
		detectorCfg := t.cfg.Metrics.CardinalityDetector
		t.cardinality = newCardinalityDetector(cardinalitySettings{
			MaxCardinality:     detectorCfg.MaxCardinality,
			MaxInstruments:     detectorCfg.MaxInstruments,
			DiagnosticInterval: detectorCfg.DiagnosticInterval,
			Enable:             detectorCfg.Enable,
		}, attrCache)
		attrCache.SetDetector(t.cardinality)
		t.cardinality.Start()
	}

	if t.cfg.MonitorConfig.Enable {
		t.monitor = newMonitorServer(t.cfg.MonitorAddr)
		if err := t.monitor.start(ctx); err != nil {
			_ = errors.Join(t.shutdownGracefulLocked(ctx)...)

			return err
		}
	}

	t.started.Store(true)

	return nil
}

func (t *Telemetry) startOTelLocked(ctx context.Context) error {
	configureExportCompression(t.cfg.WithCompression)

	provider, shutdown, err := newMeterProvider(ctx, t.cfg)
	if err != nil {
		return err
	}

	t.metricProvider = provider
	t.shutdownFn = shutdown

	traceProvider, traceShutdown, err := newTracerProvider(ctx, t.cfg)
	if err != nil {
		if t.shutdownFn != nil {
			_ = t.shutdownFn(ctx)
			t.shutdownFn = nil
		}

		return err
	}

	t.traceProvider = traceProvider
	t.traceShutdown = traceShutdown
	if t.cfg.Traces.Enable {
		otel.SetTracerProvider(traceProvider)
		t.traceInstalled = true
	}
	t.refreshTracerLocked()

	return nil
}

// Shutdown flushes exporters and releases resources. Safe to call after Start,
// and again after a subsequent Start (restart-safe; not sync.Once).
func (t *Telemetry) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started.Load() {
		return nil
	}

	t.started.Store(false)
	errs := t.shutdownGracefulLocked(ctx)

	return errors.Join(errs...)
}

func (t *Telemetry) shutdownGracefulLocked(ctx context.Context) []error {
	var errs []error

	// Invalidate instruments created before this shutdown.
	t.epoch.Add(1)

	if t.cardinality != nil {
		t.cardinality.Stop()
		t.cardinality = nil
	}

	if t.shutdownFn != nil {
		if shutdownErr := t.shutdownFn(ctx); shutdownErr != nil {
			errs = append(errs, fmt.Errorf("metric provider shutdown: %w", shutdownErr))
		}

		t.shutdownFn = nil
	}

	if t.traceShutdown != nil {
		if shutdownErr := t.traceShutdown(ctx); shutdownErr != nil {
			errs = append(errs, fmt.Errorf("trace provider shutdown: %w", shutdownErr))
		}

		t.traceShutdown = nil
	}

	if t.monitor != nil {
		if shutdownErr := t.monitor.shutdown(ctx); shutdownErr != nil {
			errs = append(errs, fmt.Errorf("monitor shutdown: %w", shutdownErr))
		}

		t.monitor = nil
	}

	t.metricProvider = noop.NewMeterProvider()
	t.traceProvider = tracenoop.NewTracerProvider()
	t.refreshTracerLocked()
	t.registry = newRegistryWithCache(
		noop.NewMeterProvider().Meter("noop"),
		newAttrCache(defaultMaxCardinality),
		&t.epoch,
		t.epoch.Load(),
		maxInstrumentsFromCfg(t.cfg),
	)

	if t.traceInstalled {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		t.traceInstalled = false
	}

	return errs
}

func (t *Telemetry) Meter(ins string, opts ...metric.MeterOption) metric.Meter {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.metricProvider.Meter(ins, opts...)
}

func (t *Telemetry) Registry() *Registry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.registry == nil {
		return newRegistryWithCache(
			t.metricProvider.Meter(t.cfg.Service),
			newAttrCache(defaultMaxCardinality),
			&t.epoch,
			t.epoch.Load(),
			maxInstrumentsFromCfg(t.cfg),
		)
	}

	return t.registry
}

func (t *Telemetry) refreshTracerLocked() {
	if t.traceProvider == nil {
		t.tracer = otel.Tracer(t.cfg.Service)

		return
	}

	t.tracer = t.traceProvider.Tracer(t.cfg.Service)
}

func (t *Telemetry) refreshTracer() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refreshTracerLocked()
}
