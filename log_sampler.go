package tel

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

const (
	logSamplerTick      = time.Second
	logSamplerNumLevels = int(zerolog.FatalLevel) - int(zerolog.DebugLevel) + 1
	logSamplerMinLevel  = zerolog.DebugLevel
)

// RateSampler caps log volume per second (global and optional per-level).
// Sample is allocation-free: atomics + fixed per-level counters.
type RateSampler struct {
	globalLimit uint64
	levelLimit  [logSamplerNumLevels]uint64
	global      rateCounter
	levels      [logSamplerNumLevels]rateCounter
}

type rateCounter struct {
	resetAt atomic.Int64
	count   atomic.Uint64
}

// NewRateSampler builds a sampler. globalLimit 0 disables global capping (per-level
// limits may still apply). levelLimits maps zerolog level → max/sec; unset levels are unlimited.
func NewRateSampler(globalLimit int, levelLimits map[zerolog.Level]uint64) *RateSampler {
	sampler := &RateSampler{}
	if globalLimit > 0 {
		sampler.globalLimit = uint64(globalLimit)
	}
	for lvl, n := range levelLimits {
		idx := logSamplerIndex(lvl)
		if idx < 0 {
			continue
		}
		sampler.levelLimit[idx] = n
	}

	return sampler
}

func logSamplerIndex(lvl zerolog.Level) int {
	if lvl < logSamplerMinLevel || lvl > zerolog.FatalLevel {
		return -1
	}

	return int(lvl - logSamplerMinLevel)
}

// Sample implements zerolog.Sampler.
func (s *RateSampler) Sample(lvl zerolog.Level) bool {
	if s == nil {
		return true
	}
	if lvl >= zerolog.FatalLevel {
		return true
	}
	idx := logSamplerIndex(lvl)
	if idx < 0 {
		return true
	}

	limit := s.levelLimit[idx]
	if limit > 0 {
		if s.levels[idx].inc(logSamplerTick) > limit {
			return false
		}
	}
	if s.globalLimit > 0 {
		if s.global.inc(logSamplerTick) > s.globalLimit {
			return false
		}
	}

	return true
}

func (c *rateCounter) inc(tick time.Duration) uint64 {
	now := time.Now().UnixNano()
	resetAfter := c.resetAt.Load()
	if resetAfter > now {
		return c.count.Add(1)
	}
	c.count.Store(1)
	newReset := now + tick.Nanoseconds()
	if !c.resetAt.CompareAndSwap(resetAfter, newReset) {
		return c.count.Add(1)
	}

	return 1
}

// parseLevelMessageLimits parses "debug=50,info=200" into per-level caps.
func parseLevelMessageLimits(spec string) map[zerolog.Level]uint64 {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	out := make(map[zerolog.Level]uint64, 4)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		k, limitStr, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		lvl, err := zerolog.ParseLevel(strings.TrimSpace(k))
		if err != nil {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(limitStr), 10, 64)
		if err != nil {
			continue
		}
		out[lvl] = n
	}
	if len(out) == 0 {
		return nil
	}

	return out
}
