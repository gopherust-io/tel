# Changelog

## v0.2.1

- Console logger level colors: debug=blue, info=green, warn=yellow, error=red, fatal/panic=magenta (purple).
- Err() stack frames use a typed `stackFrame` struct (no per-frame map allocs); console level colors applied once via `sync.Once`.
- goalign v1.2.0; extra logger/bytesconv tests and benches.

## v0.2.0

- Replace slog / stdlib log with process-global zerolog (`InitLogger` / `ConfigureLogger`, `Info()` / `Ctx(ctx)`).
- Logger caller as `funcName:line`; `Err(err)` emits a `stack` field (func/file/line frames).
- Restart-safe `Shutdown` (Start→Shutdown→Start→Shutdown).
- Synchronize provider/registry/tracer access across Start/Shutdown/hot paths.
- AttrCache: reserve cardinality slots before insert (`Len() <= max` under race).
- Pre-Start instruments no-op after `Start` (epoch); re-fetch after Start.
- Enforce `MaxInstruments` on registry create; always set TraceContext+Baggage propagator on Start.
- `EndSpan(nil)` is safe; monitor bind failures fail `Start`; stats encode-then-write once.

## v0.1.1

- Optimize tracing inject/extract (`headerCarrier.Set` reuses single-value slices).
- Cache `trace.Tracer` on `Telemetry` for `StartSpan`.
- Cache constant messaging attributes (`MessagingSystem` / operation helpers).
- Lock-free `Global()` via `atomic.Pointer`.
- AttrCache: allocation-free FNV shard hash; cardinality detector observes misses only.

## v0.1.0

- Initial release: split from `github.com/gopherust-io/libs/telemetry` as module `github.com/gopherust-io/tel` (`package tel`).
