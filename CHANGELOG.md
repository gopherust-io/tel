# Changelog

## v0.1.1

- Optimize tracing inject/extract (`headerCarrier.Set` reuses single-value slices).
- Cache `trace.Tracer` on `Telemetry` for `StartSpan`.
- Cache constant messaging attributes (`MessagingSystem` / operation helpers).
- Lock-free `Global()` via `atomic.Pointer`.
- AttrCache: allocation-free FNV shard hash; cardinality detector observes misses only.

## v0.1.0

- Initial release: split from `github.com/gopherust-io/libs/telemetry` as module `github.com/gopherust-io/tel` (`package tel`).
