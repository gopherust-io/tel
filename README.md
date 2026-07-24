# tel

OpenTelemetry metrics and traces for Go services: OTLP export, low-allocation instruments, and a small HTTP monitor.

**Import:** `github.com/gopherust-io/tel`

Companion JetStream client: [`github.com/gopherust-io/nats`](https://github.com/gopherust-io/nats).

## Quick start

```bash
go get github.com/gopherust-io/tel@latest
```

```go
package main

import (
	"context"
	"log"

	"github.com/gopherust-io/tel"
)

func main() {
	cfg := tel.DefaultConfig()
	cfg.Service = "orders-api"
	cfg.TelConfig.Enable = true
	cfg.TelConfig.Address = "127.0.0.1:4317"
	cfg.MonitorConfig.Enable = true
	cfg.MonitorConfig.MonitorAddr = "0.0.0.0:8011"

	t := tel.NewWithConfig(cfg)
	tel.SetGlobal(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := t.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer t.Shutdown(ctx)

	ctx = tel.WrapContext(ctx, t)
	_ = ctx
}
```

When `TelConfig.Enable` is `false`, telemetry uses noop meter and tracer providers. `DefaultDebugConfig()` disables OTLP and the monitor for local development.

## Recording metrics (low-allocation)

```go
registry := tel.FromCtx(ctx).Registry()

events, _ := registry.Counter("orders.processed")
latency, _ := registry.Histogram("orders.latency_seconds")

events.AddWith(ctx, 1, "orders.created")

timer := tel.NewTimer(latency)
timer.Start()
// ... work ...
timer.StopWith(ctx, "orders.created")
```

## Distributed tracing helpers

```go
headers := tel.InjectContext(ctx, nil)
ctx = tel.ExtractContext(ctx, inboundHeaders)
```

Messaging attribute helpers (`MessagingSystem`, `MessagingSubject`, …) are available for publishers/consumers (including NATS).

## Monitor endpoints

- `GET /healthz` — liveness
- `GET /stats` — runtime stats

## Development

```bash
make test
make test-race
make lint
make demo    # examples/basic
```
