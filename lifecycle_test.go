package tel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestShutdownRestart(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = false

	tel := NewWithConfig(cfg)
	ctx := context.Background()

	require.NoError(t, tel.Start(ctx))
	require.NoError(t, tel.Shutdown(ctx))
	require.False(t, tel.started.Load())

	require.NoError(t, tel.Start(ctx))
	require.True(t, tel.started.Load())
	require.NoError(t, tel.Shutdown(ctx))
	require.False(t, tel.started.Load())
}

func TestConcurrentShutdownRace(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = false

	tel := NewWithConfig(cfg)
	ctx := context.Background()
	require.NoError(t, tel.Start(ctx))

	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			_ = tel.Registry()
			_ = tel.Meter("race")
			_, span := tel.StartSpan(ctx, "race")
			span.End()
			_ = tel.Shutdown(ctx)
		})
	}
	wg.Wait()
	require.False(t, tel.started.Load())
}

func TestAttrCacheNeverExceedsMax(t *testing.T) {
	const maxEntries = 32
	cache := newAttrCache(maxEntries)

	var wg sync.WaitGroup
	for g := range 64 {
		wg.Go(func() {
			for i := range 200 {
				_ = cache.Subject(fmt.Sprintf("s.%d.%d", g, i))
				assert.LessOrEqual(t, cache.Len(), maxEntries)
			}
		})
	}
	wg.Wait()
	assert.LessOrEqual(t, cache.Len(), maxEntries)
}

func TestPreStartInstrumentDoesNotExport(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	cfg := DefaultDebugConfig()
	tel := NewWithConfig(cfg)

	stale, err := tel.Registry().Counter("pre_start")
	require.NoError(t, err)

	// Simulate Start replacing the registry + bumping epoch while binding a real meter.
	tel.mu.Lock()
	epoch := tel.epoch.Add(1)
	tel.metricProvider = provider
	tel.registry = newRegistryWithCache(
		provider.Meter(tel.cfg.Service),
		newAttrCache(defaultMaxCardinality),
		&tel.epoch,
		epoch,
		maxInstrumentsFromCfg(tel.cfg),
	)
	tel.mu.Unlock()
	tel.started.Store(true)

	stale.Add(context.Background(), 42)

	fresh, err := tel.Registry().Counter("pre_start")
	require.NoError(t, err)
	fresh.Add(context.Background(), 7)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var sum int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "pre_start" {
				continue
			}
			data, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range data.DataPoints {
				sum += dp.Value
			}
		}
	}
	assert.Equal(t, int64(7), sum, "stale pre-Start handle must not export")

	require.NoError(t, tel.Shutdown(context.Background()))
}

func TestEndSpanNil(t *testing.T) {
	EndSpan(nil, nil)
	EndSpan(nil, assert.AnError)
}

func TestMonitorBindFailure(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = true
	cfg.MonitorAddr = ln.Addr().String()

	tel := NewWithConfig(cfg)
	err = tel.Start(context.Background())
	require.Error(t, err)
	require.False(t, tel.started.Load())
}

func TestMaxInstrumentsEnforced(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	reg := newRegistryWithCache(provider.Meter("test"), newAttrCache(10), nil, 0, 2)

	_, err := reg.Counter("a")
	require.NoError(t, err)
	_, err = reg.Histogram("b")
	require.NoError(t, err)
	_, err = reg.Gauge("c")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max instruments")
}

func TestStartSetsPropagatorWhenMetricsOnly(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = false

	tel := NewWithConfig(cfg)
	require.NoError(t, tel.Start(context.Background()))
	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })

	headers := InjectContext(context.Background(), nil)
	assert.NotNil(t, headers)
}
