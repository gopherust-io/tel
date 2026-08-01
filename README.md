# tel

OTLP metrics and traces for Go. The **record path** is allocation-sensitive; **export** is batched OTLP/gRPC—keep those worlds separate.

Module: [`github.com/gopherust-io/tel`](https://github.com/gopherust-io/tel) · [Architecture](ARCHITECTURE.md) · JetStream: [nats](https://github.com/gopherust-io/nats)

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/gopherust-io/tel/badge)](https://scorecard.dev/viewer/?uri=github.com/gopherust-io/tel)

```bash
go get github.com/gopherust-io/tel@latest
```

## Example

Env-first (recommended): put knobs in the process environment or a `.env` file. `tel.Init` loads `.env` (if present), parses config, configures the logger, and starts exporters:

```go
package main

import (
	"context"
	"log"

	"github.com/gopherust-io/tel"
)

func main() {
	ctx := context.Background()
	t, shutdown := tel.Init(ctx) // .env + GetConfigFromEnv + ConfigureLogger + Start + SetGlobal
	defer shutdown(ctx)

	ctx = tel.WrapContext(ctx, t)
	processed, err := t.Registry().Counter("orders.processed")
	if err != nil {
		log.Fatal(err)
	}
	processed.AddWith(ctx, 1, "orders.created")
}
```

Local/tests without a collector: `tel.InitWithConfig(ctx, tel.DefaultDebugConfig())` (skips `.env`).

### Environment knobs

| Variable | Default | Notes |
|----------|---------|--------|
| `TEL_DOTENV` | `.env` | path loaded by `Init` / `GetConfigFromEnv`; missing file is OK |
| `TEL_SERVICE_NAME` | hostname | service field + OTel `service.name` |
| `POD_NAME` | hostname | log `pod` + instance id |
| `NAMESPACE` | `default` | |
| `DEPLOY_ENVIRONMENT` | `dev` | |
| `VERSION` | `dev` | |
| `LOG_LEVEL` | `info` | |
| `LOG_ENCODE` | `console` | `json` \| `pretty` \| `console` |
| `TEL_ENABLE` | `true` | OTLP export on/off |
| `TEL_COLLECTOR_GRPC_ADDR` | `127.0.0.1:4317` | |
| `TEL_TRACES_ENABLE` | `true` | |
| `TEL_TRACES_SAMPLER` | `parentbased_statustraceidratio:0.1` | |
| `MONITOR_ENABLE` | `true` | `/healthz`, `/stats` |
| `MONITOR_ADDR` | `127.0.0.1:8011` | |

## Record

Create instruments once. Use subject-keyed `*With` helpers—they hit `AttrCache`. Subjects must be a **bounded** set or you will blow cardinality.

```go
r := tel.FromCtx(ctx).Registry()

count, err := r.Counter("orders.processed")
if err != nil {
	return err
}
latency, err := r.Histogram("orders.latency_seconds")
if err != nil {
	return err
}

count.AddWith(ctx, 1, "orders.created")

timer := tel.NewTimer(latency)
timer.Start()
// work
timer.StopWith(ctx, "orders.created")
```

## Propagate

```go
headers := tel.InjectContext(ctx, nil)
ctx = tel.ExtractContext(ctx, inboundHeaders)
```

Prefer `MessagingSystem` / `MessagingSubject` (and friends) over hand-rolled attribute maps.

## Logging

Process-global zerolog via `tel.InitLogger` / `tel.ConfigureLogger` and `tel.Info()` / `tel.Ctx(ctx)`.

`LOG_ENCODE`: `console` / `text` (zerolog ConsoleWriter, default) | `json` (compact) | `pretty` / `json_pretty` (indented JSON).

`ConfigureLogger` attaches resource fields when set: `service`, `pod`, `namespace`, `environment`, `version`.

`pod` comes from `Config.Pod` / `POD_NAME`, else `HOSTNAME`, else `os.Hostname()`. Set `TEL_SERVICE_NAME` to the app name and `POD_NAME` (Downward API) for the instance.

Every line includes `caller` as `funcName:line` (e.g. `main.main:42`). `Err(err)` adds a `stack` field (func/file/line frames) for the call path to the log site.

### Trace correlation

Use context-aware helpers so log lines carry `trace_id` / `span_id` from the active span. `StartSpan` also stores an enriched logger on the returned context.

```go
ctx, span := tel.FromCtx(ctx).StartSpan(ctx, "orders.create")
defer tel.EndSpan(span, err)

tel.InfoCtx(ctx).Msg("creating order")
```

Or wrap work with `TraceFunc` (span + one log line with `function` and duration):

```go
err := tel.TraceFunc(ctx, "orders.create", func(ctx context.Context) error {
	return createOrder(ctx)
})
```

### Operation metadata

| Field | When |
|-------|------|
| `trace_id`, `span_id` | Valid span on `ctx` (`Ctx` / `*Ctx` / after `StartSpan`) |
| `function` | `tel.Func(e, name)` or `TraceFunc` |
| `duration_ms` | `tel.Duration(e, d)` when `d < 1s` |
| `duration_s` | `tel.Duration(e, d)` when `d >= 1s` |
| `service`, `pod`, `namespace`, `environment`, `version` | From `ConfigureLogger` (`pod` via `POD_NAME` / hostname fallback) |

```go
start := time.Now()
// ...
tel.Duration(tel.Func(tel.InfoCtx(ctx), "HandlePay"), time.Since(start)).Msg("done")
```

### Context fields

Immutable bag (copy-on-write). Nested `WithFields` appends; last key wins when logged.

```go
ctx = tel.WithFields(ctx, tel.StrField("component", "api"), tel.IntField("user_shard", 3))
tel.InfoCtx(ctx).Msg("handling")
```

### Log rate limits

Off by default (`LOGS_MAX_MESSAGES_PER_SECOND=0`). When set, `ConfigureLogger` installs an allocation-free `RateSampler` (atomics; never drops fatal/panic). Optional per-level caps: `LOGS_MAX_LEVEL_MESSAGES_PER_SECOND=debug=50,info=200`.

### Trace sampling

`TEL_TRACES_SAMPLER` (default `parentbased_statustraceidratio:0.1`): `always` | `never` | `traceidratio:N` | `statustraceidratio:N` | `parentbased_*`. Status sampler force-records when start attrs/links include `error` (attrs at **StartSpan** only). `DefaultDebugConfig()` uses `always`.

## fasthttp middleware

Native middleware (no `net/http` adaptor). Default span name is the HTTP method (low cardinality).

```go
import telfasthttp "github.com/gopherust-io/tel/middleware/fasthttp"

h := telfasthttp.Server(next,
    telfasthttp.WithSkipPrefixes("/health", "/metrics"),
)
```

## Knobs

| Concern | Knob |
|---------|------|
| Collector | `TelConfig.Address` / `TEL_COLLECTOR_GRPC_ADDR` |
| Export on/off | `TelConfig.Enable` / `TEL_ENABLE` |
| Trace sampler | `TEL_TRACES_SAMPLER` |
| Log rate limit | `LOGS_MAX_MESSAGES_PER_SECOND`, `LOGS_MAX_LEVEL_MESSAGES_PER_SECOND` |
| Quiet local | `DefaultDebugConfig()` |
| Compression | On by default; gzip BestSpeed on **export only** (`TEL_ENABLE_COMPRESSION`) |
| Monitor | `MonitorConfig` → `GET /healthz`, `GET /stats` |

Compression sets the process-wide gRPC `gzip` level. Default export is insecure—fine for a local collector; use `TelConfig.Raw` PEM for TLS/mTLS.

## Lifecycle

Call `Start` before recording. Instruments obtained **before** `Start` are invalidated when `Start` runs—re-fetch via `Registry()` afterward. `Shutdown` is restart-safe (`Start` → `Shutdown` → `Start` → `Shutdown`).

## Do not

1. Put network I/O, locks, or attribute allocation on the record path.
2. Pass unbounded strings (user IDs, raw URLs) as `*With` subjects.
3. Skip `Start` on a production `DefaultConfig()` and assume metrics still export.
4. Keep using Counter/Histogram handles created before `Start`.

## Development

```bash
make test
make demo
```

[CONTRIBUTING.md](CONTRIBUTING.md)

## License

Apache License 2.0 — see [LICENSE](LICENSE).
