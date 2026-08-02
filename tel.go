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

	"github.com/gopherust-io/tel/internal/bytesconv"
)

var globalTelemetry atomic.Pointer[Telemetry]

func init() {
	// Placeholder global: debug/noop exporters, without forcing debug logging on every importer.
	globalTelemetry.Store(newTelemetry(DefaultDebugConfig()))
}

func Global() *Telemetry {
	return globalTelemetry.Load()
}

func SetGlobal(t *Telemetry) {
	globalTelemetry.Store(t)
}

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
	allowedLabels  []string
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
	return newTelemetry(cfg)
}

func newTelemetry(cfg Config) *Telemetry {
	service := cfg.Service
	if bytesconv.IsEmpty(service) {
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
	n := cfg.TelConfig.Metrics.CardinalityDetector.MaxInstruments
	if n <= 0 {
		return defaultMaxInstruments
	}

	return n
}

func DefaultConfig() Config {
	host, _ := os.Hostname()
	service := strings.ToLower(strings.ReplaceAll(host, "-", "_"))

	return Config{
		Service:     service,
		Pod:         host,
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
			Metrics: MetricsConfig{
				CardinalityDetector: CardinalityDetectorConfig{
					MaxCardinality:     defaultMaxCardinality,
					MaxInstruments:     defaultMaxInstruments,
					DiagnosticInterval: defaultDiagnosticInterval,
					WarnUtilizationPct: defaultWarnUtilizationPct,
					Enable:             true,
				},
			},
			Traces: TracesConfig{
				Enable:  true,
				Sampler: defaultTracesSampler,
			},
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
	c.TelConfig.Traces.Sampler = defaultDebugTracesSampler

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
	return newTelemetry(t.cfg)
}

func (t *Telemetry) Config() Config {
	return t.cfg
}

func (t *Telemetry) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.started.Load() {
		t.mu.Unlock()

		return nil
	}
	t.mu.Unlock()

	// Always install propagators so Inject/Extract work even when traces export is off.
	installPropagator()

	providers, err := t.initProviders(ctx)
	if err != nil {
		return err
	}

	maxCardinality := t.cfg.TelConfig.Metrics.CardinalityDetector.MaxCardinality
	if maxCardinality <= 0 {
		maxCardinality = defaultMaxCardinality
	}
	attrCache := newAttrCache(maxCardinality)
	detectorCfg := t.cfg.TelConfig.Metrics.CardinalityDetector
	attrCache.SetDenyUnknown(detectorCfg.DenyUnknown)
	t.mu.RLock()
	allowed := append([]string(nil), t.allowedLabels...)
	t.mu.RUnlock()
	if len(allowed) > 0 {
		attrCache.Allow(allowed...)
	}

	var cardinality *cardinalityDetector
	if detectorCfg.Enable {
		cardinality = newCardinalityDetector(cardinalitySettings(detectorCfg), attrCache)
		attrCache.SetDetector(cardinality)
	}

	var monitor *monitorServer
	if err := t.startMonitor(ctx, providers, &monitor); err != nil {
		return err
	}

	t.mu.Lock()
	if t.started.Load() {
		t.mu.Unlock()
		// Another Start won the race; tear down what we built outside the lock.
		rollbackStart(ctx, monitor, providers)

		return nil
	}

	epoch := t.epoch.Add(1)
	t.metricProvider = providers.metricProvider
	t.traceProvider = providers.traceProvider
	t.shutdownFn = providers.shutdownFn
	t.traceShutdown = providers.traceShutdown
	t.traceInstalled = providers.traceInstalled
	t.refreshTracerLocked()
	t.registry = newRegistryWithCache(
		t.metricProvider.Meter(t.cfg.Service),
		attrCache,
		&t.epoch,
		epoch,
		maxInstrumentsFromCfg(t.cfg),
	)
	t.cardinality = cardinality
	t.monitor = monitor
	t.started.Store(true)
	t.mu.Unlock()

	if monitor != nil {
		monitor.bind(t)
	}

	if cardinality != nil {
		cardinality.Start()
	}

	return nil
}

type startProviders struct {
	metricProvider metric.MeterProvider
	traceProvider  trace.TracerProvider
	shutdownFn     func(context.Context) error
	traceShutdown  func(context.Context) error
	traceInstalled bool
}

func (t *Telemetry) initProviders(ctx context.Context) (startProviders, error) {
	out := startProviders{
		metricProvider: noop.NewMeterProvider(),
		traceProvider:  tracenoop.NewTracerProvider(),
	}
	if !t.cfg.TelConfig.Enable {
		return out, nil
	}

	configureExportCompression(t.cfg.TelConfig.WithCompression)

	provider, shutdownFn, err := newMeterProvider(ctx, t.cfg)
	if err != nil {
		return startProviders{}, err
	}
	tp, traceShutdown, err := newTracerProvider(ctx, t.cfg)
	if err != nil {
		_ = shutdownFn(ctx)

		return startProviders{}, err
	}
	out.metricProvider = provider
	out.traceProvider = tp
	out.shutdownFn = shutdownFn
	out.traceShutdown = traceShutdown
	if t.cfg.TelConfig.Traces.Enable {
		otel.SetTracerProvider(tp)
		out.traceInstalled = true
	}

	return out, nil
}

func (t *Telemetry) startMonitor(ctx context.Context, providers startProviders, out **monitorServer) error {
	if !t.cfg.MonitorConfig.Enable {
		return nil
	}
	monitor := newMonitorServer(t.cfg.MonitorConfig.MonitorAddr)
	if err := monitor.start(ctx); err != nil {
		rollbackStart(ctx, nil, providers)
		// Clear any previously marked install so a failed Start never leaves
		// a sticky global tracer provider attributed to this Telemetry.
		t.mu.Lock()
		if t.traceInstalled {
			otel.SetTracerProvider(tracenoop.NewTracerProvider())
			t.traceInstalled = false
		}
		t.mu.Unlock()

		return err
	}
	*out = monitor

	return nil
}

func rollbackStart(ctx context.Context, monitor *monitorServer, providers startProviders) {
	if monitor != nil {
		_ = monitor.shutdown(ctx)
	}
	if providers.shutdownFn != nil {
		_ = providers.shutdownFn(ctx)
	}
	if providers.traceShutdown != nil {
		_ = providers.traceShutdown(ctx)
	}
	if providers.traceInstalled {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
	}
}

// Shutdown flushes exporters and releases resources. Safe to call after Start,
// and again after a subsequent Start (restart-safe; not sync.Once).
func (t *Telemetry) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	if !t.started.Load() {
		t.mu.Unlock()

		return nil
	}

	t.started.Store(false)
	t.epoch.Add(1)

	cardinality := t.cardinality
	t.cardinality = nil
	shutdownFn := t.shutdownFn
	t.shutdownFn = nil
	traceShutdown := t.traceShutdown
	t.traceShutdown = nil
	monitor := t.monitor
	t.monitor = nil
	traceInstalled := t.traceInstalled
	t.traceInstalled = false

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
	t.mu.Unlock()

	var errs []error
	if cardinality != nil {
		cardinality.Stop()
	}
	if shutdownFn != nil {
		if shutdownErr := shutdownFn(ctx); shutdownErr != nil {
			errs = append(errs, fmt.Errorf("metric provider shutdown: %w", shutdownErr))
		}
	}
	if traceShutdown != nil {
		if shutdownErr := traceShutdown(ctx); shutdownErr != nil {
			errs = append(errs, fmt.Errorf("trace provider shutdown: %w", shutdownErr))
		}
	}
	if monitor != nil {
		if shutdownErr := monitor.shutdown(ctx); shutdownErr != nil {
			errs = append(errs, fmt.Errorf("monitor shutdown: %w", shutdownErr))
		}
	}
	if traceInstalled {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
	}

	return errors.Join(errs...)
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

// AllowSubjects registers label values for DenyUnknown mode. Safe before or after Start;
// pre-Start values are applied when the AttrCache is created.
func (t *Telemetry) AllowSubjects(labels ...string) {
	if t == nil || len(labels) == 0 {
		return
	}

	t.mu.Lock()
	t.allowedLabels = append(t.allowedLabels, labels...)
	cache := (*AttrCache)(nil)
	if t.registry != nil {
		cache = t.registry.cache
	}
	t.mu.Unlock()

	if cache != nil {
		cache.Allow(labels...)
	}
}

// CardinalityStats returns the cardinality cockpit snapshot for /stats and diagnostics.
func (t *Telemetry) CardinalityStats() CardinalitySnapshot {
	if t == nil {
		return CardinalitySnapshot{}
	}

	t.mu.RLock()
	det := t.cardinality
	reg := t.registry
	maxInst := maxInstrumentsFromCfg(t.cfg)
	t.mu.RUnlock()

	instruments := 0
	if reg != nil {
		reg.mu.RLock()
		instruments = reg.instrumentCount()
		reg.mu.RUnlock()
	}
	if det != nil {
		return det.Snapshot(instruments, maxInst)
	}

	snap := CardinalitySnapshot{
		Instruments:    instruments,
		MaxInstruments: maxInst,
	}
	if reg != nil && reg.cache != nil {
		snap.CacheEntries = reg.cache.Len()
		snap.MaxCardinality = reg.cache.MaxEntries()
		snap.Subjects = reg.cache.Subjects()
		snap.DenyUnknown = reg.cache.DenyUnknown()
		if snap.MaxCardinality > 0 {
			snap.UtilizationPct = (snap.CacheEntries * 100) / snap.MaxCardinality
		}
	}

	return snap
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
