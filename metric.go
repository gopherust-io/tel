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

type duoKey struct{ a, b string }

type trioKey struct{ a, b, c string }

type subjectEntry struct {
	attrs      attribute.Set
	addOpts    []metric.AddOption
	recordOpts []metric.RecordOption
}

// attrShard uses a mutex + in-place map so cold inserts do not CoW-copy the shard.
type attrShard struct {
	cache  map[string]subjectEntry
	cache2 map[duoKey]subjectEntry
	cache3 map[trioKey]subjectEntry
	mu     sync.RWMutex
}

// AttrCache interns attribute sets and metric options for hot-path label reuse.
// Entries are sharded by subject hash; hits take a short shard RLock.
type AttrCache struct {
	detector     *cardinalityDetector
	allowed      atomic.Pointer[map[string]struct{}]
	denyOnce     sync.Map
	overflow     subjectEntry
	shards       [attrCacheShards]attrShard
	maxEntries   int
	size         atomic.Int64
	overflowOnce sync.Once
	denyUnknown  atomic.Bool
}

func newAttrCache(maxEntries int) *AttrCache {
	if maxEntries <= 0 {
		maxEntries = defaultMaxCardinality
	}

	c := &AttrCache{maxEntries: maxEntries}
	for i := range c.shards {
		c.shards[i].cache = make(map[string]subjectEntry, maxEntries/attrCacheShards+1)
		c.shards[i].cache2 = make(map[duoKey]subjectEntry, maxEntries/attrCacheShards+1)
		c.shards[i].cache3 = make(map[trioKey]subjectEntry, maxEntries/attrCacheShards+1)
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

// SetDenyUnknown drops labels not registered via Allow when true.
func (c *AttrCache) SetDenyUnknown(deny bool) {
	if c == nil {
		return
	}
	c.denyUnknown.Store(deny)
}

// DenyUnknown reports whether unknown subjects are dropped.
func (c *AttrCache) DenyUnknown() bool {
	return c != nil && c.denyUnknown.Load()
}

// Allow registers label values permitted when DenyUnknown is set.
// Applies to every dimension of AddWith / AddWith2 / AddWith3.
func (c *AttrCache) Allow(labels ...string) {
	if c == nil || len(labels) == 0 {
		return
	}

	prev := c.allowed.Load()
	capHint := len(labels)
	if prev != nil {
		capHint += len(*prev)
	}
	next := make(map[string]struct{}, capHint)
	if prev != nil {
		for k := range *prev {
			next[k] = struct{}{}
		}
	}
	for _, l := range labels {
		if l == "" {
			continue
		}
		next[l] = struct{}{}
	}
	c.allowed.Store(&next)
}

func (c *AttrCache) isAllowed(labels ...string) bool {
	if !c.denyUnknown.Load() {
		return true
	}
	allowed := c.allowed.Load()
	if allowed == nil || len(*allowed) == 0 {
		return false
	}
	for _, l := range labels {
		if l == "" {
			continue
		}
		if _, ok := (*allowed)[l]; !ok {
			return false
		}
	}

	return true
}

func (c *AttrCache) noteDenied(label string) {
	if _, loaded := c.denyOnce.LoadOrStore(label, struct{}{}); loaded {
		return
	}
	Warn().
		Str("subject", label).
		Msg("telemetry denied unknown subject (METRICS_CARDINALITY_DENY_UNKNOWN)")
	if c.detector != nil {
		c.detector.ObserveDenied(label)
	}
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

func (c *AttrCache) shardIndex2(a, b string) uint32 {
	return (fnv32aString(a) ^ fnv32aString(b)) % attrCacheShards
}

func (c *AttrCache) shardIndex3(a, b, c3 string) uint32 {
	return (fnv32aString(a) ^ fnv32aString(b) ^ fnv32aString(c3)) % attrCacheShards
}

func (c *AttrCache) Subject(subject string) attribute.Set {
	e, ok := c.lookup(subject)
	if !ok {
		return attribute.Set{}
	}

	return e.attrs
}

func (c *AttrCache) SubjectOpts(subject string) []metric.AddOption {
	e, ok := c.lookup(subject)
	if !ok {
		return nil
	}

	return e.addOpts
}

func (c *AttrCache) SubjectRecordOpts(subject string) []metric.RecordOption {
	e, ok := c.lookup(subject)
	if !ok {
		return nil
	}

	return e.recordOpts
}

func (c *AttrCache) Subject2Opts(a, b string) []metric.AddOption {
	e, ok := c.lookup2(a, b)
	if !ok {
		return nil
	}

	return e.addOpts
}

func (c *AttrCache) Subject2RecordOpts(a, b string) []metric.RecordOption {
	e, ok := c.lookup2(a, b)
	if !ok {
		return nil
	}

	return e.recordOpts
}

func (c *AttrCache) Subject3Opts(a, b, c3 string) []metric.AddOption {
	e, ok := c.lookup3(a, b, c3)
	if !ok {
		return nil
	}

	return e.addOpts
}

func (c *AttrCache) Subject3RecordOpts(a, b, c3 string) []metric.RecordOption {
	e, ok := c.lookup3(a, b, c3)
	if !ok {
		return nil
	}

	return e.recordOpts
}

// lookup returns false when DenyUnknown rejects the subject.
func (c *AttrCache) lookup(subject string) (subjectEntry, bool) {
	if !c.isAllowed(subject) {
		c.noteDenied(subject)

		return subjectEntry{}, false
	}

	return c.entry(subject), true
}

func (c *AttrCache) lookup2(a, b string) (subjectEntry, bool) {
	if !c.isAllowed(a, b) {
		c.noteDenied(a + "|" + b)

		return subjectEntry{}, false
	}

	return c.entry2(a, b), true
}

func (c *AttrCache) lookup3(a, b, c3 string) (subjectEntry, bool) {
	if !c.isAllowed(a, b, c3) {
		c.noteDenied(a + "|" + b + "|" + c3)

		return subjectEntry{}, false
	}

	return c.entry3(a, b, c3), true
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

	if !c.reserveSlot(subject) {
		return c.overflowEntry()
	}

	attrs := attribute.NewSet(attribute.String(attrKeySubject, subject))
	entry := newSubjectEntry(attrs)
	s.cache[subject] = entry

	return entry
}

func (c *AttrCache) entry2(a, b string) subjectEntry {
	key := duoKey{a: a, b: b}
	idx := c.shardIndex2(a, b)
	s := &c.shards[idx]

	s.mu.RLock()
	if entry, ok := s.cache2[key]; ok {
		s.mu.RUnlock()

		return entry
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.cache2[key]; ok {
		return entry
	}

	if !c.reserveSlot(a + "|" + b) {
		return c.overflowEntry()
	}

	attrs := attribute.NewSet(
		attribute.String(attrKeySubject, a),
		attribute.String(attrKeyStatus, b),
	)
	entry := newSubjectEntry(attrs)
	s.cache2[key] = entry

	return entry
}

func (c *AttrCache) entry3(a, b, c3 string) subjectEntry {
	key := trioKey{a: a, b: b, c: c3}
	idx := c.shardIndex3(a, b, c3)
	s := &c.shards[idx]

	s.mu.RLock()
	if entry, ok := s.cache3[key]; ok {
		s.mu.RUnlock()

		return entry
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.cache3[key]; ok {
		return entry
	}

	if !c.reserveSlot(a + "|" + b + "|" + c3) {
		return c.overflowEntry()
	}

	attrs := attribute.NewSet(
		attribute.String(attrKeySubject, a),
		attribute.String(attrKeyStatus, b),
		attribute.String(attrKeyConsumer, c3),
	)
	entry := newSubjectEntry(attrs)
	s.cache3[key] = entry

	return entry
}

func (c *AttrCache) reserveSlot(missKey string) bool {
	for {
		sz := c.size.Load()
		if int(sz) >= c.maxEntries {
			if c.detector != nil {
				c.detector.ObserveMiss(missKey, true)
			}

			return false
		}
		if c.size.CompareAndSwap(sz, sz+1) {
			if c.detector != nil {
				c.detector.ObserveMiss(missKey, false)
			}

			return true
		}
	}
}

func (c *AttrCache) Len() int {
	return int(c.size.Load())
}

func (c *AttrCache) MaxEntries() int {
	if c == nil {
		return 0
	}

	return c.maxEntries
}

// Subjects returns a snapshot of cached arity-1 subject keys (for /stats).
func (c *AttrCache) Subjects() []string {
	if c == nil {
		return nil
	}

	out := make([]string, 0, c.Len())
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.RLock()
		for k := range s.cache {
			out = append(out, k)
		}
		for k := range s.cache2 {
			out = append(out, k.a+"|"+k.b)
		}
		for k := range s.cache3 {
			out = append(out, k.a+"|"+k.b+"|"+k.c)
		}
		s.mu.RUnlock()
	}

	return out
}

func (c *AttrCache) overflowEntry() subjectEntry {
	c.overflowOnce.Do(func() {
		attrs := attribute.NewSet(attribute.String(attrKeySubject, overflowSubject))
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
