# Changelog

## Unreleased

## v0.3.0

- Env-first config: `GetConfigFromEnv` / `Init` / `InitWithConfig` via `github.com/gopherust-io/env` v0.5.0 (nested Metrics/Traces/TLS filled in `applyNestedEnv`).
- `GetConfigFromEnv` / `Init` load `.env` (optional `TEL_DOTENV` path); missing file is ignored. `InitWithConfig` does not load dotenv.
- `Init` returns `(*Telemetry, shutdown)` and Fatals on failure (no error return); `InitWithConfig` returns the same shutdown func.
- `LOG_ENCODE` defaults to `console` (local-friendly); set `json` / `pretty` for structured logs.
- `LOG_ENCODE=pretty` (also `json_pretty`) indents JSON log lines for local readability.
- `ConfigureLogger` / OTel resource include `pod` (`POD_NAME` / hostname) plus `service`, `namespace`, `environment`, `version`.
- Context log correlation: `Ctx` / `InfoCtx` (and Debug/Warn/Error) emit `trace_id` / `span_id` from the active span; `StartSpan` stores an enriched logger on the returned context.
- Log helpers: `Func`, `Duration` (`duration_ms` under 1s, else `duration_s`), and `TraceFunc` (span + timed log line).
- Trace sampler config: `TEL_TRACES_SAMPLER` (`parentbased_statustraceidratio:0.1` default; `StatusTraceIDRatioBased` force-samples on start `error` attr). Debug config uses `always`.
- Log rate limiting via `LOGS_MAX_MESSAGES_PER_SECOND` / `LOGS_MAX_LEVEL_MESSAGES_PER_SECOND` (`RateSampler`, off by default).
- Immutable `WithFields` context bag (`StrField` / `IntField` / `BoolField`); merged in `Ctx` / `*Ctx`.
- `tel/middleware/fasthttp`: native Server middleware (extract + span + request log); `tel.Inject` / `tel.Extract` TextMapCarrier helpers.
- goalign **v1.3.0**; `ARCHITECTURE.md`.

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
