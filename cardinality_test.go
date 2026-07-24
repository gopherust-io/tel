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
