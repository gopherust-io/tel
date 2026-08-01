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

// attrShard uses a mutex + in-place map so cold inserts do not CoW-copy the shard.
type attrShard struct {
	cache map[string]subjectEntry
	mu    sync.RWMutex
}

// AttrCache interns attribute sets and metric options for hot-path label reuse.
// Entries are sharded by subject hash; hits take a short shard RLock.
type AttrCache struct {
	shards       [attrCacheShards]attrShard
	detector     *cardinalityDetector
	overflow     subjectEntry
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
		c.shards[i].cache = make(map[string]subjectEntry, maxEntries/attrCacheShards+1)
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

	s.mu.RLock()
	if entry, ok := s.cache[subject]; ok {
		s.mu.RUnlock()

		return entry
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.cache[subject]; ok {
		return entry
	}

	for {
		sz := c.size.Load()
		if int(sz) >= c.maxEntries {
			if c.detector != nil {
				c.detector.ObserveMiss(subject, true)
			}

			return c.overflowEntry()
		}
		if c.size.CompareAndSwap(sz, sz+1) {
			break
		}
	}

	attrs := attribute.NewSet(attribute.String("subject", subject))
	entry := newSubjectEntry(attrs)
	s.cache[subject] = entry
	if c.detector != nil {
		c.detector.ObserveMiss(subject, false)
	}

	return entry
}

func (c *AttrCache) Len() int {
	return int(c.size.Load())
}

func (c *AttrCache) overflowEntry() subjectEntry {
	c.overflowOnce.Do(func() {
		attrs := attribute.NewSet(attribute.String("subject", overflowSubject))
		c.overflow = newSubjectEntry(attrs)
	})

	return c.overflow
}

func newSubjectEntry(attrs attribute.Set) subjectEntry {
	return subjectEntry{
		attrs:      attrs,
		addOpts:    []metric.AddOption{metric.WithAttributeSet(attrs)},
		recordOpts: []metric.RecordOption{metric.WithAttributeSet(attrs)},
	}
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
