package tel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAttrCacheSubjectHitMiss(t *testing.T) {
	cache := newAttrCache(10)

	first := cache.Subject("orders.created")
	second := cache.Subject("orders.created")
	assert.Equal(t, first, second)
	assert.Equal(t, 1, cache.Len())

	other := cache.Subject("orders.updated")
	assert.NotEqual(t, first, other)
	assert.Equal(t, 2, cache.Len())
}

func TestAttrCacheSubjectOptsCached(t *testing.T) {
	cache := newAttrCache(10)
	opts1 := cache.SubjectOpts("orders.created")
	opts2 := cache.SubjectOpts("orders.created")
	assert.Len(t, opts1, 1)
	assert.Equal(t, opts1, opts2)

	ropts1 := cache.SubjectRecordOpts("orders.created")
	ropts2 := cache.SubjectRecordOpts("orders.created")
	assert.Len(t, ropts1, 1)
	assert.Equal(t, ropts1, ropts2)
}

func TestAttrCacheOverflow(t *testing.T) {
	cache := newAttrCache(2)
	cache.Subject("a")
	cache.Subject("b")
	overflow := cache.Subject("c")
	again := cache.Subject("d")

	assert.Equal(t, overflow, again)
	val, _ := overflow.Value(attribute.Key("subject"))
	assert.Equal(t, overflowSubject, val.AsString())
}

func TestAttrCacheConcurrentInsert(t *testing.T) {
	const goroutines = 32
	const perG = 50
	cache := newAttrCache(goroutines*perG + 64)

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := range perG {
				subj := fmt.Sprintf("orders.%d.%d", g, i)
				first := cache.Subject(subj)
				second := cache.Subject(subj)
				assert.Equal(t, first, second)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, goroutines*perG, cache.Len())
}

func TestFastCounterAddPrebound(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	counter, err := registry.Counter("events")
	require.NoError(t, err)

	bound := counter.WithAttrs(attribute.NewSet(attribute.String("subject", "fixed")))
	bound.Add(context.Background(), 2)
	bound.Add(context.Background(), 3)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestFastCounterAddWithCachedSubject(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	counter, err := registry.Counter("events")
	require.NoError(t, err)

	counter.AddWith(context.Background(), 1, "orders.created")
	counter.AddWith(context.Background(), 1, "orders.created")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestFastCounterAddWith2(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	counter, err := registry.Counter("events")
	require.NoError(t, err)

	counter.AddWith2(context.Background(), 1, "ORDERS", "ok")
	counter.AddWith2(context.Background(), 1, "ORDERS", "ok")
	assert.Equal(t, 1, registry.AttrCache().Len())

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestFastCounterAddWith3(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	counter, err := registry.Counter("events")
	require.NoError(t, err)

	counter.AddWith3(context.Background(), 1, "ORDERS", "ok", "worker")
	assert.Equal(t, 1, registry.AttrCache().Len())

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestAttrCacheDenyUnknown(t *testing.T) {
	cache := newAttrCache(10)
	cache.SetDenyUnknown(true)
	cache.Allow("orders.created")

	_, ok := cache.lookup("orders.created")
	assert.True(t, ok)
	_, ok = cache.lookup("orders.evil")
	assert.False(t, ok)
	assert.Equal(t, 1, cache.Len())
}

func TestAttrCacheDuoTrioHit(t *testing.T) {
	cache := newAttrCache(10)
	a := cache.Subject2Opts("ORDERS", "ok")
	b := cache.Subject2Opts("ORDERS", "ok")
	assert.Equal(t, a, b)
	assert.Equal(t, 1, cache.Len())

	c := cache.Subject3Opts("ORDERS", "ok", "worker")
	d := cache.Subject3Opts("ORDERS", "ok", "worker")
	assert.Equal(t, c, d)
	assert.Equal(t, 2, cache.Len())
}

func TestFastHistogramRecordWith(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	hist, err := registry.Histogram("latency")
	require.NoError(t, err)
	hist.RecordWith(context.Background(), 0.25, "orders.created")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestFastGaugeRecordWith(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	gauge, err := registry.Gauge("depth")
	require.NoError(t, err)
	gauge.RecordWith(context.Background(), 7, "queue")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestTimerStop(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	hist, err := registry.Histogram("duration")
	require.NoError(t, err)

	timer := NewTimer(hist)
	timer.Start()
	timer.Stop(context.Background())

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestRegistryCachesInstruments(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	first, err := registry.Counter("same")
	require.NoError(t, err)
	second, err := registry.Counter("same")
	require.NoError(t, err)
	assert.Same(t, first, second)

	h1, err := registry.Histogram("latency")
	require.NoError(t, err)
	h2, err := registry.Histogram("latency")
	require.NoError(t, err)
	assert.Same(t, h1, h2)

	g1, err := registry.Gauge("depth")
	require.NoError(t, err)
	g2, err := registry.Gauge("depth")
	require.NoError(t, err)
	assert.Same(t, g1, g2)
}

func TestFastCounterAddWithEmptySubject(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	counter, err := registry.Counter("events")
	require.NoError(t, err)
	counter.AddWith(context.Background(), 1, "")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestTimerStopWith(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	hist, err := registry.Histogram("duration")
	require.NoError(t, err)

	timer := NewTimer(hist)
	timer.Start()
	timer.StopWith(context.Background(), "orders.created")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestTimerStopNoop(t *testing.T) {
	timer := NewTimer(nil)
	timer.Start()
	timer.Stop(context.Background())
	timer.StopWith(context.Background(), "orders.created")
}

func TestAttrCacheSubjectRecordOpts(t *testing.T) {
	cache := newAttrCache(10)
	ropts := cache.SubjectRecordOpts("orders.created")
	assert.Len(t, ropts, 1)
	assert.Equal(t, ropts, cache.SubjectRecordOpts("orders.created"))
}

func TestFastCounterWithAttrsEmpty(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	counter, err := registry.Counter("events")
	require.NoError(t, err)
	bound := counter.WithAttrs(attribute.NewSet())
	bound.Add(context.Background(), 1)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
}

func TestRegistryAttrCacheAndBoundInstruments(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("test"))

	require.NotNil(t, registry.AttrCache())
	registry.AttrCache().SetDetector(nil)

	counter, err := registry.Counter("events")
	require.NoError(t, err)
	hist, err := registry.Histogram("latency")
	require.NoError(t, err)
	gauge, err := registry.Gauge("depth")
	require.NoError(t, err)

	attrs := attribute.NewSet(attribute.String("subject", "orders.created"))
	counter.WithAttrs(attrs).Add(context.Background(), 1)
	hist.WithAttrs(attrs).Record(context.Background(), 0.01)
	gauge.WithAttrs(attrs).Record(context.Background(), 3)
	hist.Record(context.Background(), 0.02)
	gauge.Record(context.Background(), 4)
	hist.RecordWith(context.Background(), 0.03, "")
	gauge.RecordWith(context.Background(), 5, "")

	assert.NotEmpty(t, attrsToRecordOpts(attrs))
	assert.Nil(t, attrsToRecordOpts(attribute.NewSet()))
	assert.NotEmpty(t, attrsToAddOpts(attrs))
	assert.Nil(t, attrsToAddOpts(attribute.NewSet()))
}

func TestMessagingAttributeHelpers(t *testing.T) {
	assert.Equal(t, "orders.created", MessagingSubject("orders.created").Value.AsString())
	assert.Equal(t, "nats", MessagingSystem().Value.AsString())
	assert.Equal(t, "publish", MessagingOperationPublish().Value.AsString())
	assert.Equal(t, "process", MessagingOperationProcess().Value.AsString())
	assert.Equal(t, "request", MessagingOperationRequest().Value.AsString())
	assert.Equal(t, "reply", MessagingOperationReply().Value.AsString())
	assert.Equal(t, "ORDERS", MessagingStream("ORDERS").Value.AsString())
	assert.Equal(t, "orders-worker", MessagingConsumer("orders-worker").Value.AsString())
	assert.Equal(t, int64(42), MessagingStreamSequence(42).Value.AsInt64())
	assert.Equal(t, int64(3), MessagingDeliveryCount(3).Value.AsInt64())
}

func TestTelemetryNewAndMeter(t *testing.T) {
	tel := New()
	require.NotNil(t, tel)
	require.NotNil(t, tel.Registry())
	assert.NotNil(t, tel.Meter("test"))
}

func TestCardinalityDetectorReport(t *testing.T) {
	cache := newAttrCache(2)
	d := newCardinalityDetector(cardinalitySettings{
		Enable:             true,
		MaxCardinality:     2,
		DiagnosticInterval: time.Hour,
	}, cache)
	cache.SetDetector(d)
	cache.Subject("a")
	cache.Subject("b")
	cache.Subject("c") // overflow
	d.Observe("c")
	d.report()
	records, overflows := d.Stats()
	assert.Positive(t, records)
	assert.Positive(t, overflows)
}
