package tel

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type FastCounter struct {
	counter metric.Int64Counter
	cache   *AttrCache
	opts    []metric.AddOption
}

type FastHistogram struct {
	histogram  metric.Float64Histogram
	cache      *AttrCache
	recordOpts []metric.RecordOption
}

type FastGauge struct {
	gauge      metric.Int64Gauge
	cache      *AttrCache
	recordOpts []metric.RecordOption
}

type Registry struct {
	meter    metric.Meter
	cache    *AttrCache
	counters map[string]*FastCounter
	hists    map[string]*FastHistogram
	gauges   map[string]*FastGauge
	mu       sync.RWMutex
}

func newRegistry(meter metric.Meter) *Registry {
	return newRegistryWithCache(meter, newAttrCache(defaultMaxCardinality))
}

func newRegistryWithCache(meter metric.Meter, cache *AttrCache) *Registry {
	return &Registry{
		meter:    meter,
		cache:    cache,
		counters: make(map[string]*FastCounter),
		hists:    make(map[string]*FastHistogram),
		gauges:   make(map[string]*FastGauge),
	}
}

func (r *Registry) AttrCache() *AttrCache {
	return r.cache
}

func (r *Registry) Counter(name string, opts ...metric.Int64CounterOption) (*FastCounter, error) {
	r.mu.RLock()

	if counter, ok := r.counters[name]; ok {
		r.mu.RUnlock()

		return counter, nil
	}

	r.mu.RUnlock()

	counter, err := r.meter.Int64Counter(name, opts...)
	if err != nil {
		return nil, err
	}

	fast := &FastCounter{counter: counter, cache: r.cache}
	r.mu.Lock()

	if existing, ok := r.counters[name]; ok {
		r.mu.Unlock()

		return existing, nil
	}

	r.counters[name] = fast
	r.mu.Unlock()

	return fast, nil
}

func (r *Registry) Histogram(name string, opts ...metric.Float64HistogramOption) (*FastHistogram, error) {
	r.mu.RLock()

	if hist, ok := r.hists[name]; ok {
		r.mu.RUnlock()

		return hist, nil
	}

	r.mu.RUnlock()

	histogram, err := r.meter.Float64Histogram(name, opts...)
	if err != nil {
		return nil, err
	}

	fast := &FastHistogram{histogram: histogram, cache: r.cache}
	r.mu.Lock()

	if existing, ok := r.hists[name]; ok {
		r.mu.Unlock()

		return existing, nil
	}

	r.hists[name] = fast
	r.mu.Unlock()

	return fast, nil
}

func (r *Registry) Gauge(name string, opts ...metric.Int64GaugeOption) (*FastGauge, error) {
	r.mu.RLock()

	if gauge, ok := r.gauges[name]; ok {
		r.mu.RUnlock()

		return gauge, nil
	}

	r.mu.RUnlock()

	gauge, err := r.meter.Int64Gauge(name, opts...)
	if err != nil {
		return nil, err
	}

	fast := &FastGauge{gauge: gauge, cache: r.cache}
	r.mu.Lock()

	if existing, ok := r.gauges[name]; ok {
		r.mu.Unlock()

		return existing, nil
	}

	r.gauges[name] = fast
	r.mu.Unlock()

	return fast, nil
}

func (c *FastCounter) WithAttrs(attrs attribute.Set) *FastCounter {
	return &FastCounter{
		counter: c.counter,
		opts:    attrsToAddOpts(attrs),
		cache:   c.cache,
	}
}

func (c *FastCounter) Add(ctx context.Context, n int64) {
	if len(c.opts) > 0 {
		c.counter.Add(ctx, n, c.opts...)

		return
	}

	c.counter.Add(ctx, n)
}

func (c *FastCounter) AddWith(ctx context.Context, n int64, subject string) {
	if subject == "" {
		c.Add(ctx, n)

		return
	}

	c.counter.Add(ctx, n, c.cache.SubjectOpts(subject)...)
}

func (h *FastHistogram) WithAttrs(attrs attribute.Set) *FastHistogram {
	return &FastHistogram{
		histogram:  h.histogram,
		recordOpts: attrsToRecordOpts(attrs),
		cache:      h.cache,
	}
}

func (h *FastHistogram) Record(ctx context.Context, value float64) {
	if len(h.recordOpts) > 0 {
		h.histogram.Record(ctx, value, h.recordOpts...)

		return
	}

	h.histogram.Record(ctx, value)
}

func (h *FastHistogram) RecordWith(ctx context.Context, value float64, subject string) {
	if subject == "" {
		h.Record(ctx, value)

		return
	}

	h.histogram.Record(ctx, value, h.cache.SubjectRecordOpts(subject)...)
}

func (g *FastGauge) WithAttrs(attrs attribute.Set) *FastGauge {
	return &FastGauge{
		gauge:      g.gauge,
		recordOpts: attrsToRecordOpts(attrs),
		cache:      g.cache,
	}
}

func (g *FastGauge) Record(ctx context.Context, value int64) {
	if len(g.recordOpts) > 0 {
		g.gauge.Record(ctx, value, g.recordOpts...)

		return
	}

	g.gauge.Record(ctx, value)
}

func (g *FastGauge) RecordWith(ctx context.Context, value int64, subject string) {
	if subject == "" {
		g.Record(ctx, value)

		return
	}

	g.gauge.Record(ctx, value, g.cache.SubjectRecordOpts(subject)...)
}

type Timer struct {
	hist  *FastHistogram
	start int64
}

func NewTimer(hist *FastHistogram) Timer {
	return Timer{hist: hist}
}

func (t *Timer) Start() {
	t.start = time.Now().UnixNano()
}

func (t *Timer) elapsed() float64 {
	if t.start == 0 {
		return 0
	}

	return float64(time.Now().UnixNano()-t.start) / float64(time.Second)
}

func (t *Timer) Stop(ctx context.Context) {
	if t.hist == nil || t.start == 0 {
		return
	}

	t.hist.Record(ctx, t.elapsed())
	t.start = 0
}

func (t *Timer) StopWith(ctx context.Context, subject string) {
	if t.hist == nil || t.start == 0 {
		return
	}

	t.hist.RecordWith(ctx, t.elapsed(), subject)
	t.start = 0
}
