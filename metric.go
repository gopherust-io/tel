package tel

import (
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	overflowSubject = "_overflow"
	attrCacheShards = 16
	fnvOffset32     = 2166136261
	fnvPrime32      = 16777619
)

type subjectEntry struct {
	attrs      attribute.Set
	addOpts    []metric.AddOption
	recordOpts []metric.RecordOption
}

// attrShard holds an immutable map behind atomic.Pointer for lock-free reads.
// Inserts copy-on-write and CAS the pointer; steady-state hits never take a mutex.
type attrShard struct {
	cache atomic.Pointer[map[string]subjectEntry]
}

// AttrCache interns attribute sets and metric options for hot-path label reuse.
// Entries are sharded by subject hash; each shard is a lock-free CoW map.
type AttrCache struct {
	detector     *cardinalityDetector
	overflow     subjectEntry
	shards       [attrCacheShards]attrShard
	maxEntries   int
	size         atomic.Int64
	overflowOnce sync.Once
}

func newAttrCache(maxEntries int) *AttrCache {
	if maxEntries <= 0 {
		maxEntries = defaultMaxCardinality
	}

	c := &AttrCache{maxEntries: maxEntries}
	for i := range c.shards {
		m := make(map[string]subjectEntry, maxEntries/attrCacheShards+1)
		c.shards[i].cache.Store(&m)
	}

	return c
}

// SetDetector wires optional cardinality observation into the hot path.
func (c *AttrCache) SetDetector(d *cardinalityDetector) {
	if c == nil {
		return
	}
	c.detector = d
}

// fnv32aString hashes without allocating (no []byte conversion).
func fnv32aString(s string) uint32 {
	h := uint32(fnvOffset32)
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= fnvPrime32
	}

	return h
}

func (c *AttrCache) shardIndex(subject string) uint32 {
	return fnv32aString(subject) % attrCacheShards
}

func (c *AttrCache) Subject(subject string) attribute.Set {
	return c.entry(subject).attrs
}

func (c *AttrCache) SubjectOpts(subject string) []metric.AddOption {
	return c.entry(subject).addOpts
}

func (c *AttrCache) SubjectRecordOpts(subject string) []metric.RecordOption {
	return c.entry(subject).recordOpts
}

func (c *AttrCache) entry(subject string) subjectEntry {
	idx := c.shardIndex(subject)
	s := &c.shards[idx]

	for {
		cur := s.cache.Load()
		if entry, ok := (*cur)[subject]; ok {
			return entry
		}

		if int(c.size.Load()) >= c.maxEntries {
			if c.detector != nil {
				c.detector.ObserveMiss(subject, true)
			}

			return c.overflowEntry()
		}

		if c.detector != nil {
			c.detector.ObserveMiss(subject, false)
		}

		attrs := attribute.NewSet(attribute.String("subject", subject))
		entry := subjectEntry{
			attrs:      attrs,
			addOpts:    []metric.AddOption{metric.WithAttributeSet(attrs)},
			recordOpts: []metric.RecordOption{metric.WithAttributeSet(attrs)},
		}

		next := make(map[string]subjectEntry, len(*cur)+1)
		for k, v := range *cur {
			next[k] = v
		}
		next[subject] = entry

		if s.cache.CompareAndSwap(cur, &next) {
			c.size.Add(1)

			return entry
		}
		// Lost the race: retry — either the subject is present or we insert again.
	}
}

func (c *AttrCache) Len() int {
	return int(c.size.Load())
}

func (c *AttrCache) overflowEntry() subjectEntry {
	c.overflowOnce.Do(func() {
		attrs := attribute.NewSet(attribute.String("subject", overflowSubject))
		c.overflow = subjectEntry{
			attrs:      attrs,
			addOpts:    []metric.AddOption{metric.WithAttributeSet(attrs)},
			recordOpts: []metric.RecordOption{metric.WithAttributeSet(attrs)},
		}
	})

	return c.overflow
}

func attrsToAddOpts(attrs attribute.Set) []metric.AddOption {
	if attrs.Len() == 0 {
		return nil
	}

	return []metric.AddOption{metric.WithAttributeSet(attrs)}
}

func attrsToRecordOpts(attrs attribute.Set) []metric.RecordOption {
	if attrs.Len() == 0 {
		return nil
	}

	return []metric.RecordOption{metric.WithAttributeSet(attrs)}
}
