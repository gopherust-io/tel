package tel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/gopherust-io/tel/internal/bytesconv"
)

type FastCounter struct {
	counter  metric.Int64Counter
	cache    *AttrCache
	epochPtr *atomic.Uint64
	opts     []metric.AddOption
	epoch    uint64
}

type FastHistogram struct {
	histogram  metric.Float64Histogram
	cache      *AttrCache
	epochPtr   *atomic.Uint64
	recordOpts []metric.RecordOption
	epoch      uint64
}

type FastGauge struct {
	gauge      metric.Int64Gauge
	cache      *AttrCache
	epochPtr   *atomic.Uint64
	recordOpts []metric.RecordOption
	epoch      uint64
}

type Registry struct {
	meter          metric.Meter
	cache          *AttrCache
	epochPtr       *atomic.Uint64
	counters       map[string]*FastCounter
	hists          map[string]*FastHistogram
	gauges         map[string]*FastGauge
	epoch          uint64
	maxInstruments int
	mu             sync.RWMutex
}

func newRegistry(meter metric.Meter) *Registry {
	return newRegistryWithCache(meter, newAttrCache(defaultMaxCardinality), nil, 0, defaultMaxInstruments)
}

func newRegistryWithCache(
	meter metric.Meter,
	cache *AttrCache,
	epochPtr *atomic.Uint64,
	epoch uint64,
	maxInstruments int,
) *Registry {
	if maxInstruments <= 0 {
		maxInstruments = defaultMaxInstruments
	}

	return &Registry{
		meter:          meter,
		cache:          cache,
		counters:       make(map[string]*FastCounter),
		hists:          make(map[string]*FastHistogram),
		gauges:         make(map[string]*FastGauge),
		epoch:          epoch,
		epochPtr:       epochPtr,
		maxInstruments: maxInstruments,
	}
}

func (r *Registry) AttrCache() *AttrCache {
	return r.cache
}

func (r *Registry) instrumentCount() int {
	return len(r.counters) + len(r.hists) + len(r.gauges)
}

// getOrCreate looks up under RLock, otherwise creates via create and stores under Lock.
func getOrCreate[T any](
	reg *Registry,
	lookup func() (T, bool),
	create func() (T, error),
	store func(T),
) (T, error) {
	reg.mu.RLock()
	if existing, ok := lookup(); ok {
		reg.mu.RUnlock()

		return existing, nil
	}
	reg.mu.RUnlock()

	created, err := create()
	if err != nil {
		var zero T

		return zero, err
	}

	reg.mu.Lock()
	if existing, ok := lookup(); ok {
		reg.mu.Unlock()

		return existing, nil
	}
	if reg.instrumentCount() >= reg.maxInstruments {
		reg.mu.Unlock()
		var zero T

		return zero, fmt.Errorf("tel: max instruments (%d) exceeded", reg.maxInstruments)
	}
	store(created)
	reg.mu.Unlock()

	return created, nil
}

func (r *Registry) Counter(name string, opts ...metric.Int64CounterOption) (*FastCounter, error) {
	return getOrCreate(
		r,
		func() (*FastCounter, bool) {
			counter, ok := r.counters[name]

			return counter, ok
		},
		func() (*FastCounter, error) {
			counter, err := r.meter.Int64Counter(name, opts...)
			if err != nil {
				return nil, err
			}

			return &FastCounter{
				counter:  counter,
				cache:    r.cache,
				epoch:    r.epoch,
				epochPtr: r.epochPtr,
			}, nil
		},
		func(fast *FastCounter) { r.counters[name] = fast },
	)
}

func (r *Registry) Histogram(name string, opts ...metric.Float64HistogramOption) (*FastHistogram, error) {
	return getOrCreate(
		r,
		func() (*FastHistogram, bool) {
			hist, ok := r.hists[name]

			return hist, ok
		},
		func() (*FastHistogram, error) {
			histogram, err := r.meter.Float64Histogram(name, opts...)
			if err != nil {
				return nil, err
			}

			return &FastHistogram{
				histogram: histogram,
				cache:     r.cache,
				epoch:     r.epoch,
				epochPtr:  r.epochPtr,
			}, nil
		},
		func(fast *FastHistogram) { r.hists[name] = fast },
	)
}

func (r *Registry) Gauge(name string, opts ...metric.Int64GaugeOption) (*FastGauge, error) {
	return getOrCreate(
		r,
		func() (*FastGauge, bool) {
			gauge, ok := r.gauges[name]

			return gauge, ok
		},
		func() (*FastGauge, error) {
			gauge, err := r.meter.Int64Gauge(name, opts...)
			if err != nil {
				return nil, err
			}

			return &FastGauge{
				gauge:    gauge,
				cache:    r.cache,
				epoch:    r.epoch,
				epochPtr: r.epochPtr,
			}, nil
		},
		func(fast *FastGauge) { r.gauges[name] = fast },
	)
}

func instrumentLive(epochPtr *atomic.Uint64, epoch uint64) bool {
	return epochPtr == nil || epochPtr.Load() == epoch
}

func (c *FastCounter) live() bool {
	return instrumentLive(c.epochPtr, c.epoch)
}

func (h *FastHistogram) live() bool {
	return instrumentLive(h.epochPtr, h.epoch)
}

func (g *FastGauge) live() bool {
	return instrumentLive(g.epochPtr, g.epoch)
}

func (c *FastCounter) WithAttrs(attrs attribute.Set) *FastCounter {
	return &FastCounter{
		counter:  c.counter,
		opts:     attrsToAddOpts(attrs),
		cache:    c.cache,
		epoch:    c.epoch,
		epochPtr: c.epochPtr,
	}
}

func (c *FastCounter) Add(ctx context.Context, n int64) {
	if !c.live() {
		return
	}

	if len(c.opts) > 0 {
		c.counter.Add(ctx, n, c.opts...)

		return
	}

	c.counter.Add(ctx, n)
}

func (c *FastCounter) AddWith(ctx context.Context, n int64, subject string) {
	if !c.live() {
		return
	}

	if bytesconv.IsEmpty(subject) {
		c.Add(ctx, n)

		return
	}

	opts := c.cache.SubjectOpts(subject)
	if len(c.opts) > 0 {
		combined := make([]metric.AddOption, 0, len(c.opts)+len(opts))
		combined = append(combined, c.opts...)
		combined = append(combined, opts...)
		c.counter.Add(ctx, n, combined...)

		return
	}

	c.counter.Add(ctx, n, opts...)
}

func (h *FastHistogram) WithAttrs(attrs attribute.Set) *FastHistogram {
	return &FastHistogram{
		histogram:  h.histogram,
		recordOpts: attrsToRecordOpts(attrs),
		cache:      h.cache,
		epoch:      h.epoch,
		epochPtr:   h.epochPtr,
	}
}

func (h *FastHistogram) Record(ctx context.Context, value float64) {
	if !h.live() {
		return
	}

	if len(h.recordOpts) > 0 {
		h.histogram.Record(ctx, value, h.recordOpts...)

		return
	}

	h.histogram.Record(ctx, value)
}

func (h *FastHistogram) RecordWith(ctx context.Context, value float64, subject string) {
	if !h.live() {
		return
	}

	if bytesconv.IsEmpty(subject) {
		h.Record(ctx, value)

		return
	}

	opts := h.cache.SubjectRecordOpts(subject)
	if len(h.recordOpts) > 0 {
		combined := make([]metric.RecordOption, 0, len(h.recordOpts)+len(opts))
		combined = append(combined, h.recordOpts...)
		combined = append(combined, opts...)
		h.histogram.Record(ctx, value, combined...)

		return
	}

	h.histogram.Record(ctx, value, opts...)
}

func (g *FastGauge) WithAttrs(attrs attribute.Set) *FastGauge {
	return &FastGauge{
		gauge:      g.gauge,
		recordOpts: attrsToRecordOpts(attrs),
		cache:      g.cache,
		epoch:      g.epoch,
		epochPtr:   g.epochPtr,
	}
}

func (g *FastGauge) Record(ctx context.Context, value int64) {
	if !g.live() {
		return
	}

	if len(g.recordOpts) > 0 {
		g.gauge.Record(ctx, value, g.recordOpts...)

		return
	}

	g.gauge.Record(ctx, value)
}

func (g *FastGauge) RecordWith(ctx context.Context, value int64, subject string) {
	if !g.live() {
		return
	}

	if bytesconv.IsEmpty(subject) {
		g.Record(ctx, value)

		return
	}

	opts := g.cache.SubjectRecordOpts(subject)
	if len(g.recordOpts) > 0 {
		combined := make([]metric.RecordOption, 0, len(g.recordOpts)+len(opts))
		combined = append(combined, g.recordOpts...)
		combined = append(combined, opts...)
		g.gauge.Record(ctx, value, combined...)

		return
	}

	g.gauge.Record(ctx, value, opts...)
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
