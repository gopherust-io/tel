package tel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCardinalityDetectorObserveAndReport(t *testing.T) {
	cache := newAttrCache(2)
	detector := newCardinalityDetector(cardinalitySettings{
		MaxCardinality:     2,
		MaxInstruments:     10,
		DiagnosticInterval: 20 * time.Millisecond,
		WarnUtilizationPct: 50,
		Enable:             true,
	}, cache)

	detector.Start()
	defer detector.Stop()

	detector.Observe("a")
	detector.Observe("b")
	detector.Observe("c")

	records, overflows := detector.Stats()
	assert.Equal(t, int64(3), records)
	assert.GreaterOrEqual(t, overflows, int64(0))
}

func TestCardinalityDetectorDisabled(t *testing.T) {
	cache := newAttrCache(10)
	detector := newCardinalityDetector(cardinalitySettings{Enable: false}, cache)
	detector.Start()
	detector.Stop()

	detector.Observe("subject")
	records, overflows := detector.Stats()
	assert.Equal(t, int64(0), records)
	assert.Equal(t, int64(0), overflows)
}

func TestCardinalityDetectorDefaults(t *testing.T) {
	detector := newCardinalityDetector(cardinalitySettings{Enable: true}, newAttrCache(5))
	assert.Equal(t, 100, detector.cfg.MaxCardinality)
	assert.Equal(t, 500, detector.cfg.MaxInstruments)
	assert.Equal(t, 10*time.Minute, detector.cfg.DiagnosticInterval)
}

func TestCardinalitySnapshot(t *testing.T) {
	cache := newAttrCache(10)
	cache.Subject("orders.created")
	detector := newCardinalityDetector(cardinalitySettings{
		Enable:             true,
		MaxCardinality:     10,
		MaxInstruments:     50,
		WarnUtilizationPct: 80,
	}, cache)
	cache.SetDetector(detector)

	snap := detector.Snapshot(3, 50)
	assert.Equal(t, 1, snap.CacheEntries)
	assert.Equal(t, 10, snap.MaxCardinality)
	assert.Equal(t, 10, snap.UtilizationPct)
	assert.Equal(t, 3, snap.Instruments)
	assert.Equal(t, 50, snap.MaxInstruments)
	assert.Contains(t, snap.Subjects, "orders.created")
}
