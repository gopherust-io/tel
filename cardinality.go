package tel

import (
	"sync/atomic"
	"time"
)

// goalign:ignore
type cardinalitySettings struct {
	MaxCardinality     int
	MaxInstruments     int
	DiagnosticInterval time.Duration
	WarnUtilizationPct int
	Enable             bool
	DenyUnknown        bool
}

type cardinalityDetector struct {
	cache     *AttrCache
	stopCh    chan struct{}
	doneCh    chan struct{}
	cfg       cardinalitySettings
	records   atomic.Int64
	overflows atomic.Int64
	denied    atomic.Int64
	warned    atomic.Bool
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

	warnPct := cfg.WarnUtilizationPct
	if warnPct < 0 {
		warnPct = 0
	}
	if warnPct > 100 {
		warnPct = 100
	}

	return &cardinalityDetector{
		cfg: cardinalitySettings{
			MaxCardinality:     maxCardinality,
			MaxInstruments:     maxInstruments,
			DiagnosticInterval: interval,
			WarnUtilizationPct: warnPct,
			Enable:             cfg.Enable,
			DenyUnknown:        cfg.DenyUnknown,
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

// ObserveMiss records a cache miss (insert or overflow). Hits skip this path.
func (d *cardinalityDetector) ObserveMiss(subject string, overflow bool) {
	if d == nil || !d.cfg.Enable {
		return
	}

	d.records.Add(1)
	if overflow {
		d.overflows.Add(1)
	}

	_ = subject
}

// ObserveDenied records a DenyUnknown rejection.
func (d *cardinalityDetector) ObserveDenied(subject string) {
	if d == nil || !d.cfg.Enable {
		return
	}

	d.denied.Add(1)
	_ = subject
}

// Observe records a label sighting (tests / diagnostics). Prefer ObserveMiss on cache inserts.
func (d *cardinalityDetector) Observe(subject string) {
	overflow := d != nil && d.cache != nil && d.cache.Len() >= d.cfg.MaxCardinality
	d.ObserveMiss(subject, overflow)
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
	maxCard := d.cfg.MaxCardinality

	if d.cfg.WarnUtilizationPct > 0 && maxCard > 0 {
		utilPct := (cacheLen * 100) / maxCard
		if utilPct >= d.cfg.WarnUtilizationPct {
			Warn().
				Int("cache_entries", cacheLen).
				Int("max_cardinality", maxCard).
				Int("utilization_pct", utilPct).
				Int("warn_utilization_pct", d.cfg.WarnUtilizationPct).
				Int64("miss_events", d.records.Load()).
				Int64("overflow_events", d.overflows.Load()).
				Msg("telemetry cardinality utilization high")
			d.warned.Store(true)
		}
	}

	if cacheLen >= maxCard {
		Warn().
			Int("cache_entries", cacheLen).
			Int("max_cardinality", maxCard).
			Int64("records", d.records.Load()).
			Int64("overflow_events", d.overflows.Load()).
			Msg("telemetry cardinality limit reached")
	}
}

func (d *cardinalityDetector) Stats() (records, overflows int64) {
	return d.records.Load(), d.overflows.Load()
}

func (d *cardinalityDetector) Denied() int64 {
	if d == nil {
		return 0
	}

	return d.denied.Load()
}

// CardinalitySnapshot is the monitor /stats cardinality cockpit payload.
//
// goalign:ignore // JSON DTO; trailing bool padding is unavoidable
type CardinalitySnapshot struct {
	MissEvents         int64    `json:"miss_events"`
	OverflowEvents     int64    `json:"overflow_events"`
	DeniedEvents       int64    `json:"denied_events"`
	Subjects           []string `json:"subjects,omitempty"`
	CacheEntries       int      `json:"cache_entries"`
	MaxCardinality     int      `json:"max_cardinality"`
	UtilizationPct     int      `json:"utilization_pct"`
	WarnUtilizationPct int      `json:"warn_utilization_pct"`
	Instruments        int      `json:"instruments"`
	MaxInstruments     int      `json:"max_instruments"`
	DenyUnknown        bool     `json:"deny_unknown"`
}

func (d *cardinalityDetector) Snapshot(instruments, maxInstruments int) CardinalitySnapshot {
	snap := CardinalitySnapshot{
		MaxInstruments: maxInstruments,
		Instruments:    instruments,
	}
	if d == nil {
		return snap
	}

	cacheLen := 0
	if d.cache != nil {
		cacheLen = d.cache.Len()
		snap.Subjects = d.cache.Subjects()
		snap.DenyUnknown = d.cache.DenyUnknown()
	}
	maxCard := d.cfg.MaxCardinality
	util := 0
	if maxCard > 0 {
		util = (cacheLen * 100) / maxCard
	}

	snap.CacheEntries = cacheLen
	snap.MaxCardinality = maxCard
	snap.UtilizationPct = util
	snap.WarnUtilizationPct = d.cfg.WarnUtilizationPct
	snap.MissEvents = d.records.Load()
	snap.OverflowEvents = d.overflows.Load()
	snap.DeniedEvents = d.denied.Load()
	if d.cfg.DenyUnknown {
		snap.DenyUnknown = true
	}

	return snap
}
