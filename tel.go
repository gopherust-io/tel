package tel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

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
	started        atomic.Bool
	shutdownOnce   sync.Once
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
		registry:       newRegistry(provider.Meter(service)),
	}
	tel.refreshTracer()

	return tel
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
	if !t.started.CompareAndSwap(false, true) {
		return nil
	}

	if t.cfg.TelConfig.Enable {
		if err := t.startOTel(ctx); err != nil {
			return err
		}
	} else {
		t.metricProvider = noop.NewMeterProvider()
		t.traceProvider = tracenoop.NewTracerProvider()
		t.refreshTracer()
	}

	maxCardinality := t.cfg.Metrics.CardinalityDetector.MaxCardinality
	if maxCardinality <= 0 {
		maxCardinality = defaultMaxCardinality
	}

	attrCache := newAttrCache(maxCardinality)
	t.registry = newRegistryWithCache(t.metricProvider.Meter(t.cfg.Service), attrCache)

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

		err := t.monitor.start()
		if err != nil {
			_ = t.Shutdown(ctx)

			return err
		}
	}

	return nil
}

func (t *Telemetry) startOTel(ctx context.Context) error {
	configureExportCompression(t.cfg.WithCompression)

	provider, shutdown, err := newMeterProvider(ctx, t.cfg)
	if err != nil {
		t.started.Store(false)

		return err
	}

	t.metricProvider = provider
	t.shutdownFn = shutdown

	traceProvider, traceShutdown, err := newTracerProvider(ctx, t.cfg)
	if err != nil {
		if t.shutdownFn != nil {
			_ = t.shutdownFn(ctx)
		}

		t.started.Store(false)

		return err
	}

	t.traceProvider = traceProvider
	t.traceShutdown = traceShutdown
	t.refreshTracer()

	return nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error

	t.shutdownOnce.Do(func() {
		if !t.started.Load() {
			return
		}

		errs = t.shutdownGraceful(ctx)
	})

	return errors.Join(errs...)
}

func (t *Telemetry) shutdownGraceful(ctx context.Context) []error {
	var errs []error

	// Stop diagnostic sampling before flushing exporters.
	if t.cardinality != nil {
		t.cardinality.Stop()
		t.cardinality = nil
	}

	// Flush and close OTLP exporters while the process is still healthy.
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
	t.refreshTracer()
	t.registry = newRegistry(noop.NewMeterProvider().Meter("noop"))
	t.started.Store(false)

	return errs
}

func (t *Telemetry) Meter(ins string, opts ...metric.MeterOption) metric.Meter {
	return t.metricProvider.Meter(ins, opts...)
}

func (t *Telemetry) Registry() *Registry {
	if t.registry == nil {
		return newRegistry(t.metricProvider.Meter(t.cfg.Service))
	}

	return t.registry
}
