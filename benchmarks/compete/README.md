# Competitive record-path benchmarks

Compares tel’s hot path to stock OpenTelemetry SDK attribute usage and plain zerolog.

## Methodology

- Same Go version / machine; `-benchmem -count=10` (or `make bench-compete`)
- Metrics: `sdkmetric.NewManualReader()` — **record path only**, no OTLP export
- Traces: in-memory `SpanRecorder`
- Logger: `io.Discard`
- Counter/Histogram: warm AttrCache subject `orders.created` before timing
- Report **ns/op**, **B/op**, **allocs/op**

## Scenarios

| Scenario | tel | Competitor |
|----------|-----|------------|
| Counter | `AddWith` (cached subject) | OTel prebuilt `attribute.Set` / `NewSet` each call |
| Histogram | `RecordWith` (cached) | Same twin patterns |
| Span | `StartSpan`+`End` (also enriches logger on ctx) | `tracer.Start`+`End` |
| Logger | `Info` / `InfoCtx` with active span | Plain zerolog `Info` |

## Run

```bash
make bench-compete
# or
go test -bench=BenchmarkCompete -benchmem -count=10 ./benchmarks/compete/ -run '^$'
```

Sample output: [results.sample.txt](results.sample.txt).
