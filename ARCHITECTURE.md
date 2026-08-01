# tel — Architecture

Allocation-sensitive OTLP metrics and traces (plus zerolog logging) for Go services.

## Overview

`github.com/gopherust-io/tel` separates the hot **record** path from batched **export** over OTLP/gRPC. Config is env-driven via **env** codegen. Subject-keyed helpers use an AttrCache with cardinality guards. Used by **nats** for messaging telemetry; FastHTTP middleware provides server spans without a `net/http` adapter.

Ecosystem: [gopherust-io](https://github.com/gopherust-io/gopherust-io/blob/main/ARCHITECTURE.md) · Config: [env](https://github.com/gopherust-io/env/blob/main/ARCHITECTURE.md) · Messaging: [nats](https://github.com/gopherust-io/nats/blob/main/ARCHITECTURE.md)

## Layer / package overview

```
┌─────────────────────────────────────────────────────────────┐
│  Application / middleware                                   │
│  tel.Init → Registry instruments / spans / zlog             │
│  middleware/fasthttp                                        │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  Record path (hot)                                          │
│  counters/histograms/gauges, AttrCache, samplers, COW logs  │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  Providers + export (cold / batched)                        │
│  OTel Meter/Tracer → OTLP/gRPC exporters                    │
└─────────────────────────────────────────────────────────────┘

Config: model.go (+ envgen) → config_env_gen.go → GetConfigFromEnv
```

## Packages

| Path | Responsibility |
|------|----------------|
| `tel` (root) | Lifecycle (`Init`/`Start`/`Shutdown`), `Telemetry`, `Registry`, metrics, traces, logging, config |
| `middleware/fasthttp` | Native FastHTTP server spans (`Server`, skip prefixes) |
| `internal/bytesconv` | Zero-copy string/bytes helpers |
| `examples/basic` | Minimal Init + instrument sample |

Root areas (by concern): `tel.go` / `provider.go` (lifecycle), `instruments.go` / `metric.go` (registry), `trace*.go` / `sampler.go` (spans), `zlog*.go` / `ctx_fields.go` (logging), `config_env*.go` / `model.go` (env config), `monitor.go` (health/stats), `cardinality.go` / `compression.go` / `otlp_dial.go` (export knobs).

## Key design rules

- **Record vs export:** recording must stay allocation-sensitive; export is batched and may allocate—do not merge those worlds.
- **Noop until Start:** instruments and exporters are safe before `Start`; enable export explicitly via lifecycle.
- **Env-first config:** prefer `tel.Init` / `GetConfigFromEnv` over hand-built structs in services; use `DefaultDebugConfig` for local/tests without a collector.
- **Cardinality guards:** AttrCache and subject helpers must not unbounded-label metrics; use the built-in limits.
- **Restart-safe Shutdown:** `Shutdown` flushes and allows re-Init in the same process when tests need it.
- **Optional monitor:** `/healthz` and `/stats` are opt-in via monitor config, not required for core telemetry.

## Core APIs / interfaces

```go
func Init(ctx context.Context) (*Telemetry, func(context.Context) error)
func InitWithConfig(ctx context.Context, cfg Config) (*Telemetry, func(context.Context) error, error)

func (t *Telemetry) Start(ctx context.Context) error
func (t *Telemetry) Shutdown(ctx context.Context) error
func (t *Telemetry) Registry() *Registry

func (r *Registry) Counter(name string) (Counter, error)
// Counter.AddWith(ctx, delta, subjectKey) — AttrCache path

func WrapContext(ctx context.Context, t *Telemetry) context.Context
func FromCtx(ctx context.Context) *Telemetry
```

## Request / call flow

Example: counter on a request path

1. `tel.Init(ctx)` → loads `.env` if present, config from env, logger configured, global set, providers started (Fatals on failure).
2. `ctx = tel.WrapContext(ctx, t)`.
3. `c, _ := t.Registry().Counter("orders.processed")`.
4. `c.AddWith(ctx, 1, "orders.created")` records on the hot path (cached attributes).
5. Export batch ships via OTLP/gRPC according to metrics/traces config.
6. `defer shutdown(ctx)` flushes and tears down providers.

## Bootstrap / lifecycle

1. **Init** — `GetConfigFromEnv` (or explicit `Config`) → `ConfigureLogger` → `NewWithConfig` → `SetGlobal` → `Start`.
2. **Use** — `Registry()`, spans, `Logger` / context fields; middleware attaches server spans.
3. **Shutdown** — flush exporters, clear global as configured; safe for tests that re-init.

Local without collector: `InitWithConfig(ctx, tel.DefaultDebugConfig())`.

## Adding a feature

1. Keep new record-path APIs allocation-sensitive; put export/batch logic behind providers.
2. Extend `model.go` + regenerate with `envgen` when adding env knobs.
3. Add Registry helpers only when cardinality and naming conventions are clear.
4. Prefer `middleware/fasthttp` patterns for HTTP frameworks; avoid forcing `net/http` adapters into the core.
5. Document new env variables in the README table when config surface changes.

## Related docs

- [README](README.md)
- [Example](examples/basic/)
- [env architecture](https://github.com/gopherust-io/env/blob/main/ARCHITECTURE.md)
- [nats architecture](https://github.com/gopherust-io/nats/blob/main/ARCHITECTURE.md)
