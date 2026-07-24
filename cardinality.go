package tel

import (
	"log/slog"
	"sync/atomic"
	"time"
)

type cardinalitySettings struct {
	MaxCardinality     int
	MaxInstruments     int
	DiagnosticInterval time.Duration
	Enable             bool
}

type cardinalityDetector struct {
	cache     *AttrCache
	stopCh    chan struct{}
	doneCh    chan struct{}
	cfg       cardinalitySettings
	records   atomic.Int64
	overflows atomic.Int64
}

func newCardinalityDetector(cfg cardinalitySettings, cache *AttrCache) *cardinalityDetector {
	interval := cfg.DiagnosticInterval
	if interval <= 0 {
		interval = defaultDiagnosticInterval
	}

	maxCardinality := cfg.MaxCardinality
	if maxCardinality <= 0 {
		maxCardinality = defaultMaxCardinality
	}

	maxInstruments := cfg.MaxInstruments
	if maxInstruments <= 0 {
		maxInstruments = defaultMaxInstruments
	}

	return &cardinalityDetector{
		cfg: cardinalitySettings{
			MaxCardinality:     maxCardinality,
			MaxInstruments:     maxInstruments,
			DiagnosticInterval: interval,
			Enable:             cfg.Enable,
		},
		cache:  cache,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (d *cardinalityDetector) Start() {
	if d == nil || !d.cfg.Enable {
		return
	}

	go d.run()
}

func (d *cardinalityDetector) Stop() {
	if d == nil || !d.cfg.Enable {
		return
	}

	close(d.stopCh)
	<-d.doneCh
}

func (d *cardinalityDetector) Observe(subject string) {
	if d == nil || !d.cfg.Enable {
		return
	}

	d.records.Add(1)

	if d.cache.Len() >= d.cfg.MaxCardinality {
		d.overflows.Add(1)

		_ = subject
	}
}

func (d *cardinalityDetector) run() {
	defer close(d.doneCh)

	ticker := time.NewTicker(d.cfg.DiagnosticInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.report()
		}
	}
}

func (d *cardinalityDetector) report() {
	cacheLen := d.cache.Len()
	if cacheLen >= d.cfg.MaxCardinality {
		slog.Warn("telemetry cardinality limit reached",
			slog.Int("cache_entries", cacheLen),
			slog.Int("max_cardinality", d.cfg.MaxCardinality),
			slog.Int64("records", d.records.Load()),
			slog.Int64("overflow_events", d.overflows.Load()),
		)
	}
}

func (d *cardinalityDetector) Stats() (records, overflows int64) {
	return d.records.Load(), d.overflows.Load()
}
