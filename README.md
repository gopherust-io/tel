# tel

OTLP metrics and traces for Go. The **record path** is allocation-sensitive; **export** is batched OTLP/gRPC—keep those worlds separate.

Module: [`github.com/gopherust-io/tel`](https://github.com/gopherust-io/tel) · JetStream: [nats](https://github.com/gopherust-io/nats)

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/gopherust-io/tel/badge)](https://scorecard.dev/viewer/?uri=github.com/gopherust-io/tel)

```bash
go get github.com/gopherust-io/tel@latest
```

## Example

```go
package main

import (
	"context"
	"log"

	"github.com/gopherust-io/tel"
)

func main() {
	// Local/tests: tel.DefaultDebugConfig() (OTLP + monitor off).
	cfg := tel.DefaultConfig()
	cfg.Service = "orders-api"
	cfg.TelConfig.Address = "127.0.0.1:4317"

	t := tel.NewWithConfig(cfg)
	tel.SetGlobal(t)

	ctx := context.Background()
	if err := t.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer t.Shutdown(ctx)

	ctx = tel.WrapContext(ctx, t)

	processed, err := t.Registry().Counter("orders.processed")
	if err != nil {
		log.Fatal(err)
	}
	processed.AddWith(ctx, 1, "orders.created")
}
```

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

## Knobs

| Concern | Knob |
|---------|------|
| Collector | `TelConfig.Address` / `TEL_COLLECTOR_GRPC_ADDR` |
| Export on/off | `TelConfig.Enable` / `TEL_ENABLE` |
| Quiet local | `DefaultDebugConfig()` |
| Compression | On by default; gzip BestSpeed on **export only** (`TEL_ENABLE_COMPRESSION`) |
| Monitor | `MonitorConfig` → `GET /healthz`, `GET /stats` |

Compression sets the process-wide gRPC `gzip` level. Default export is insecure—fine for a local collector; use `TelConfig.Raw` PEM for TLS/mTLS.

## Do not

1. Put network I/O, locks, or attribute allocation on the record path.
2. Pass unbounded strings (user IDs, raw URLs) as `*With` subjects.
3. Skip `Start` on a production `DefaultConfig()` and assume metrics still export.

## Development

```bash
make test
make demo
```

[CONTRIBUTING.md](CONTRIBUTING.md)
